package sys

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

const (
	operUp   = uint32(windows.IfOperStatusUp)
	operDown = uint32(windows.IfOperStatusDown)
)

func mkAdapter(name, desc string, status uint32) adapterEntry {
	return adapterEntry{friendlyName: name, description: desc, ifIndex: 21, luid: 0x35008000000000, operStatus: status}
}

func TestRankTunAdapter(t *testing.T) {
	cases := []struct {
		name     string
		friendly string
		desc     string
		device   string
		want     int
	}{
		{"精确匹配", "GOCLASHZ", "Meta Tunnel", "GOCLASHZ", rankNameExact},
		{"大小写与空格归一", "  goclashz  ", "meta tunnel", "GOCLASHZ", rankNameExact},
		{"带后缀前缀匹配", "GOCLASHZT", "Meta Tunnel", "GOCLASHZ", rankNamePrefix},
		{"仅描述匹配", "以太网 3", "Meta Tunnel", "GOCLASHZ", rankDescHint},
		{"仅 wintun 描述匹配", "本地连接", "Wintun Userspace Tunnel", "GOCLASHZ", rankDescHint},
		{"完全不匹配", "WLAN", "Intel(R) Wi-Fi 6 AX201", "GOCLASHZ", rankNoMatch},
		{"device 为空时只看描述", "以太网", "Meta Tunnel", "", rankDescHint},
		{"device 为空且无描述", "以太网", "Realtek PCIe GbE", "", rankNoMatch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rankTunAdapter(c.friendly, c.desc, c.device); got != c.want {
				t.Fatalf("rankTunAdapter(%q,%q,%q)=%d, want %d", c.friendly, c.desc, c.device, got, c.want)
			}
		})
	}
}

func TestBetterTunCandidate(t *testing.T) {
	up := TunAdapterInfo{DeviceName: "GOCLASHZT", Up: true}
	down := TunAdapterInfo{DeviceName: "GOCLASHZ", Up: false}

	// 关键回归：名字精确但 Down 的残留适配器，不得压过名字带后缀但 Up 的真实适配器。
	if betterTunCandidate(tunCandidate{info: down, rank: rankNameExact}, tunCandidate{info: up, rank: rankNamePrefix}) {
		t.Fatal("名字强度比较错误：Down 的精确匹配不应优于 Up 的前缀匹配")
	}

	// 同强度时 Up 优先。
	if !betterTunCandidate(tunCandidate{info: up, rank: rankNamePrefix}, tunCandidate{info: down, rank: rankNamePrefix}) {
		t.Fatal("同强度应优先 Up")
	}

	// 同为 Down 时保留先命中的候选（稳定输出）。
	if betterTunCandidate(tunCandidate{info: down, rank: rankNamePrefix}, tunCandidate{info: down, rank: rankNamePrefix}) {
		t.Fatal("同强度同 Up 状态应保留先出现的候选")
	}
}

// TestSelectTunAdapter 锁住 2026-11-18 事故的判定规则：
// 「设备不存在」与「设备存在但未 Up」必须是两种结论，且**别人的 Wintun 适配器永远不算我们的**。
func TestSelectTunAdapter(t *testing.T) {
	t.Run("自己的适配器已 Up", func(t *testing.T) {
		info, err := selectTunAdapter([]adapterEntry{
			mkAdapter("WLAN", "Intel(R) Wi-Fi 6 AX201", operUp),
			mkAdapter("GOCLASHZ", "Meta Tunnel", operUp),
		}, "GOCLASHZ")
		if err != nil {
			t.Fatalf("应当探测成功，实际: %v", err)
		}
		if info.DeviceName != "GOCLASHZ" || !info.Up {
			t.Fatalf("选错适配器: %+v", info)
		}
	})

	t.Run("自己的适配器存在但未Up_应判可等", func(t *testing.T) {
		_, err := selectTunAdapter([]adapterEntry{
			mkAdapter("GOCLASHZ", "Meta Tunnel", operDown),
		}, "GOCLASHZ")
		if !errors.Is(err, ErrTunAdapterNotReady) {
			t.Fatalf("Down 的自家适配器应返回 ErrTunAdapterNotReady，实际: %v", err)
		}
	})

	t.Run("事故复现_只有别人的Wintun且Down_必须判设备缺失", func(t *testing.T) {
		// 真实现场：device="本地连接" desc="Wintun Userspace Tunnel" up=false routes=0
		_, err := selectTunAdapter([]adapterEntry{
			mkAdapter("本地连接", "Wintun Userspace Tunnel", operDown),
		}, "GOCLASHZ")
		if !errors.Is(err, ErrTunAdapterNotFound) {
			t.Fatalf("别人的 Wintun 适配器不得被认作本程序 TUN（否则不会触发内核重启），实际: %v", err)
		}
	})

	t.Run("假阳性防护_别人的Wintun已Up也不能算就绪", func(t *testing.T) {
		_, err := selectTunAdapter([]adapterEntry{
			mkAdapter("本地连接", "Wintun Userspace Tunnel", operUp),
		}, "GOCLASHZ")
		if !errors.Is(err, ErrTunAdapterNotFound) {
			t.Fatalf("外来 Wintun 适配器即使 Up 也不能判 ActualTun=true，实际: %v", err)
		}
	})

	t.Run("被重命名但仍是我们_MetaTunnel描述可兜底", func(t *testing.T) {
		info, err := selectTunAdapter([]adapterEntry{
			mkAdapter("以太网 3", "Meta Tunnel", operUp),
		}, "GOCLASHZ")
		if err != nil {
			t.Fatalf("强描述（Meta Tunnel）应当仍能识别，实际: %v", err)
		}
		if info.DeviceName != "以太网 3" {
			t.Fatalf("选错适配器: %+v", info)
		}
	})

	t.Run("精确Down_前缀Up_应选Up的那个", func(t *testing.T) {
		info, err := selectTunAdapter([]adapterEntry{
			mkAdapter("GOCLASHZ", "Meta Tunnel", operDown),
			mkAdapter("GOCLASHZT", "Meta Tunnel", operUp),
		}, "GOCLASHZ")
		if err != nil {
			t.Fatalf("应当选中带后缀但 Up 的适配器，实际: %v", err)
		}
		if info.DeviceName != "GOCLASHZT" {
			t.Fatalf("选错适配器: %+v", info)
		}
	})

	t.Run("空列表_判设备缺失", func(t *testing.T) {
		if _, err := selectTunAdapter(nil, "GOCLASHZ"); !errors.Is(err, ErrTunAdapterNotFound) {
			t.Fatalf("空列表应返回 ErrTunAdapterNotFound，实际: %v", err)
		}
	})
}

func TestIsStrongTunDescription(t *testing.T) {
	cases := []struct {
		desc string
		want bool
	}{
		{"Meta Tunnel", true},
		{"meta tunnel", true},
		{"  Meta Tunnel  ", true},
		{"Wintun Userspace Tunnel", false},
		{"Wintun", false},
		{"Realtek PCIe GbE", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isStrongTunDescription(c.desc); got != c.want {
			t.Errorf("isStrongTunDescription(%q)=%v, want %v", c.desc, got, c.want)
		}
	}
}
