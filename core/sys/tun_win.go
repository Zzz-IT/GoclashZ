//go:build windows

package sys

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"goclashz/core/utils"

	"golang.org/x/sys/windows"
)

func GetWintunPath() string {
	return filepath.Join(utils.GetCoreBinDir(), "wintun.dll")
}

func IsWintunInstalled() bool {
	path := GetWintunPath()
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}

	// 校验1：大小在合理范围内 (32KB ~ 5MB)，防止 0 字节损坏文件
	if info.Size() < 32*1024 || info.Size() > 5*1024*1024 {
		return false
	}

	// 校验2：验证 PE 文件的特征码 (MZ 标识)
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	header := make([]byte, 2)
	if _, err := io.ReadFull(f, header); err != nil {
		return false
	}
	return header[0] == 'M' && header[1] == 'Z'
}

// ================= TUN 实际状态探测（只读） =================
//
// 这里的所有逻辑只「读」Windows 网络栈状态（适配器列表 + 路由表）：
//   - 绝不创建/删除/重启适配器
//   - 绝不修改系统路由、DNS 或防火墙
//
// TUN 资源的创建与回收都必须由 helper 服务负责（服务权限 + 独立生命周期），
// 主进程只负责如实观测：ActualTun 只能来自 ProbeTunAdapter 的成功结果，
// 不允许由 desired.Tun 推导。
//
// 适配器识别依据（真实环境）：
//   - 设备名来自 TUN 设置中的 device 字段（默认 "GOCLASHZ"，实际适配器可能带后缀，如 "GOCLASHZT"）
//   - 适配器描述由 mihomo/sing-tun 通过 Wintun 创建，为 "Meta Tunnel"
//
// 因此匹配顺序为：设备名精确匹配 > 设备名前缀匹配 > 描述关键字匹配。

var (
	// ErrTunAdapterNotFound 表示当前系统中不存在目标 TUN 适配器
	ErrTunAdapterNotFound = errors.New("tun_adapter_not_found")
	// ErrTunAdapterNotReady 表示适配器存在但不是 Up（驱动仍在初始化或已进入拆除流程）
	ErrTunAdapterNotReady = errors.New("tun_adapter_not_ready")
)

// tunAdapterDescriptionHints 是已知 TUN 适配器描述关键字（小写匹配）。
var tunAdapterDescriptionHints = []string{"meta tunnel", "wintun"}

// TunRouteInfo 是落在 TUN 适配器上的一条路由（只读快照，禁止据此清理系统路由）
type TunRouteInfo struct {
	Family      string `json:"family"`      // ipv4 / ipv6
	Destination string `json:"destination"` // CIDR
	NextHop     string `json:"nextHop,omitempty"`
	Metric      uint32 `json:"metric"`
}

// TunAdapterInfo 是 TUN 适配器的只读快照
type TunAdapterInfo struct {
	DeviceName  string         `json:"deviceName"`  // 适配器友好名（Alias）
	Description string         `json:"description"` // 适配器描述（Wintun 为 "Meta Tunnel"）
	IfIndex     uint32         `json:"ifIndex"`
	Luid        uint64         `json:"luid"`
	OperStatus  uint32         `json:"operStatus"`
	Up          bool           `json:"up"`
	Routes      []TunRouteInfo `json:"routes,omitempty"`
}

// Summary 返回用于日志/诊断的单行描述
func (i TunAdapterInfo) Summary() string {
	return fmt.Sprintf("device=%q desc=%q ifIndex=%d luid=%#x up=%v routes=%d",
		i.DeviceName, i.Description, i.IfIndex, i.Luid, i.Up, len(i.Routes))
}

// ProbeTunAdapter 只读探测目标 TUN 适配器是否真实存在且处于 Up 状态。
//
// deviceName 为空时只按描述关键字（Meta Tunnel / Wintun）匹配。
// 命中适配器时返回其 ifIndex / LUID 以及该接口上的路由只读快照。
// 成功返回 nil error 时才表示「TUN 实际处于就绪状态」。
func ProbeTunAdapter(deviceName string) (TunAdapterInfo, error) {
	addrs, err := listAdapters()
	if err != nil {
		return TunAdapterInfo{}, err
	}

	// 分阶段选优（关键）：名字匹配组永远优先于描述匹配组，组内再优先 Up。
	// 历史实现用 score*2+Up 做权重，导致「同名但 Down 的残留适配器」(8)
	// 压过「名字带后缀且 Up 的真实适配器」(7)，探测永久失败 → ActualTun 永远 false。
	var nameBest, descBest tunCandidate
	nameFound, descFound := false, false

	for _, a := range addrs {
		rank := rankTunAdapter(a.friendlyName, a.description, deviceName)
		if rank == rankNoMatch {
			continue
		}

		cur := tunCandidate{
			rank: rank,
			info: TunAdapterInfo{
				DeviceName:  a.friendlyName,
				Description: a.description,
				IfIndex:     a.ifIndex,
				Luid:        a.luid,
				OperStatus:  a.operStatus,
				Up:          a.operStatus == uint32(windows.IfOperStatusUp),
			},
		}

		if rank >= rankNamePrefix {
			if !nameFound || betterTunCandidate(cur, nameBest) {
				nameBest, nameFound = cur, true
			}
			continue
		}
		if !descFound || betterTunCandidate(cur, descBest) {
			descBest, descFound = cur, true
		}
	}

	var best TunAdapterInfo
	switch {
	case nameFound:
		best = nameBest.info
	case descFound:
		// 只有完全没有名字匹配时才退到描述匹配：
		// 否则可能把其它 TUN 软件（同为 "Meta Tunnel" 描述）的适配器认成自己的。
		best = descBest.info
	default:
		return TunAdapterInfo{}, fmt.Errorf("%w: device=%q", ErrTunAdapterNotFound, deviceName)
	}

	if !best.Up {
		return best, fmt.Errorf("%w: %s", ErrTunAdapterNotReady, best.Summary())
	}

	best.Routes = probeTunRoutes(best.Luid, best.IfIndex)
	return best, nil
}

// tunCandidate 是一次探测中命中的适配器候选，rank 越大匹配越强。
type tunCandidate struct {
	info TunAdapterInfo
	rank int
}

// betterTunCandidate 比较同一匹配组内的两个候选：先比 Up，再比匹配强度。
//
// Up 优先于匹配强度是刻意的：Down 的适配器永远不可能让探测成功，选它只会把
// 「网卡已就绪」误判成「未就绪」——正是残留同名适配器遮蔽真实适配器的场景
// （Windows 遇到重名会给新适配器加后缀，于是「Down 的 GOCLASHZ」压过「Up 的 GOCLASHZT」）。
// 跨组时名字匹配仍然永远压过描述匹配，由调用方的分组逻辑保证。
func betterTunCandidate(a, b tunCandidate) bool {
	if a.info.Up != b.info.Up {
		return a.info.Up
	}
	return a.rank > b.rank
}

type adapterEntry struct {
	friendlyName string
	description  string
	ifIndex      uint32
	luid         uint64
	operStatus   uint32
}

// listAdapters 通过 GetAdaptersAddresses 读取适配器列表（只读）
func listAdapters() ([]adapterEntry, error) {
	const flags = windows.GAA_FLAG_SKIP_ANYCAST |
		windows.GAA_FLAG_SKIP_MULTICAST |
		windows.GAA_FLAG_SKIP_DNS_SERVER

	size := uint32(16 * 1024)
	var buf []byte
	var first *windows.IpAdapterAddresses
	loaded := false

	for attempt := 0; attempt < 4; attempt++ {
		buf = make([]byte, size)
		first = (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))

		err := windows.GetAdaptersAddresses(uint32(windows.AF_UNSPEC), flags, 0, first, &size)
		if err == nil {
			loaded = true
			break
		}
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			continue // size 已被 API 更新为所需大小，重试
		}
		return nil, fmt.Errorf("读取网络适配器列表失败: %w", err)
	}
	if !loaded || first == nil {
		return nil, errors.New("读取网络适配器列表失败: 缓冲区不足")
	}

	entries := make([]adapterEntry, 0, 8)
	for a := first; a != nil && a.Length > 0; a = a.Next {
		entries = append(entries, adapterEntry{
			friendlyName: windows.UTF16PtrToString(a.FriendlyName),
			description:  windows.UTF16PtrToString(a.Description),
			ifIndex:      a.IfIndex,
			luid:         a.Luid,
			operStatus:   a.OperStatus,
		})
	}

	// 链表节点指向 buf 内部，显式保活直到数据拷贝完成
	runtime.KeepAlive(buf)
	return entries, nil
}

// 匹配层级：数值越大越强。名字匹配组（前缀/精确）永远优于描述匹配组。
const (
	rankNoMatch    = 0
	rankDescHint   = 1
	rankNamePrefix = 2
	rankNameExact  = 3
)

// rankTunAdapter 计算适配器与目标 TUN 的匹配层级，rankNoMatch 表示不匹配。
func rankTunAdapter(friendlyName, description, deviceName string) int {
	friendly := strings.ToLower(strings.TrimSpace(friendlyName))
	desc := strings.ToLower(strings.TrimSpace(description))
	device := strings.ToLower(strings.TrimSpace(deviceName))

	if device != "" {
		if friendly == device {
			return rankNameExact
		}
		// Wintun 适配器名可能带后缀（如配置 GOCLASHZ、实际 GOCLASHZT）
		if strings.HasPrefix(friendly, device) {
			return rankNamePrefix
		}
	}

	for _, hint := range tunAdapterDescriptionHints {
		if strings.Contains(desc, hint) {
			return rankDescHint
		}
	}

	return rankNoMatch
}

// probeTunRoutes 读取指定接口上的路由（只读快照）
func probeTunRoutes(luid uint64, ifIndex uint32) []TunRouteInfo {
	routes := make([]TunRouteInfo, 0, 8)

	for _, family := range []uint16{uint16(windows.AF_INET), uint16(windows.AF_INET6)} {
		var table *windows.MibIpForwardTable2
		if err := windows.GetIpForwardTable2(family, &table); err != nil || table == nil {
			continue
		}

		familyLabel := "ipv6"
		if family == uint16(windows.AF_INET) {
			familyLabel = "ipv4"
		}

		for _, row := range table.Rows() {
			if row.InterfaceLuid != luid && row.InterfaceIndex != ifIndex {
				continue
			}

			ip := sockaddrToIP(row.DestinationPrefix.Prefix)
			if ip == nil {
				continue
			}

			route := TunRouteInfo{
				Family:      familyLabel,
				Destination: fmt.Sprintf("%s/%d", ip.String(), row.DestinationPrefix.PrefixLength),
				Metric:      row.Metric,
			}
			if nextHop := sockaddrToIP(row.NextHop); nextHop != nil && !nextHop.IsUnspecified() {
				route.NextHop = nextHop.String()
			}
			routes = append(routes, route)
		}

		windows.FreeMibTable(unsafe.Pointer(table))
	}

	return routes
}

// sockaddrToIP 把 SOCKADDR_INET 联合体转换为 net.IP（只读，不做任何系统调用）
func sockaddrToIP(sa windows.RawSockaddrInet) net.IP {
	switch sa.Family {
	case uint16(windows.AF_INET):
		in4 := (*windows.RawSockaddrInet4)(unsafe.Pointer(&sa))
		ip := net.IPv4(in4.Addr[0], in4.Addr[1], in4.Addr[2], in4.Addr[3])
		return ip.To4()
	case uint16(windows.AF_INET6):
		in6 := (*windows.RawSockaddrInet6)(unsafe.Pointer(&sa))
		ip := make(net.IP, net.IPv6len)
		copy(ip, in6.Addr[:])
		return ip
	default:
		return nil
	}
}
