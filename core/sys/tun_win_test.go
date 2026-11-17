package sys

import "testing"

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
