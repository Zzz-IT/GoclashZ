//go:build windows
// +build windows

package sys

import (
	"fmt"
	"log"
	"syscall"
	"unsafe"
)

var (
	wininet               = syscall.NewLazyDLL("wininet.dll")
	procInternetSetOption = wininet.NewProc("InternetSetOptionW")

	rasapi32           = syscall.NewLazyDLL("rasapi32.dll")
	procRasEnumEntries = rasapi32.NewProc("RasEnumEntriesW")
)

const (
	INTERNET_OPTION_PER_CONNECTION_OPTION = 75
	INTERNET_OPTION_SETTINGS_CHANGED      = 39
	INTERNET_OPTION_REFRESH               = 37

	PROXY_TYPE_DIRECT = 1
	PROXY_TYPE_PROXY  = 2

	INTERNET_PER_CONN_FLAGS        = 1
	INTERNET_PER_CONN_PROXY_SERVER = 2
	INTERNET_PER_CONN_PROXY_BYPASS = 3

	ERROR_SUCCESS          = 0
	ERROR_BUFFER_TOO_SMALL = 603
	RAS_MaxEntryName       = 256
	MAX_PATH               = 260
)

// INTERNET_PER_CONN_OPTION 对应 WinINet 的结构体
type INTERNET_PER_CONN_OPTION struct {
	dwOption uint32
	Value    uint64 // 使用 uint64 兼容 32位和64位指针/数值
}

type INTERNET_PER_CONN_OPTION_LIST struct {
	dwSize        uint32
	pszConnection *uint16 // NULL 表示默认 LAN 连接
	dwOptionCount uint32
	dwOptionError uint32
	pOptions      *INTERNET_PER_CONN_OPTION
}

// RasEntryName 对应 Windows 的 RASENTRYNAMEW
type RasEntryName struct {
	dwSize      uint32
	szEntryName [RAS_MaxEntryName + 1]uint16
	dwFlags     uint32
	szPhonebook [MAX_PATH + 1]uint16
}

// EnableSystemProxy 开启系统代理 (带 RAS 穿透和瞬间广播)
func EnableSystemProxy(host string, port int, bypassDomains string) error {
	serverStr := fmt.Sprintf("%s:%d", host, port)

	serverPtr, _ := syscall.UTF16PtrFromString(serverStr)
	bypassPtr, _ := syscall.UTF16PtrFromString(bypassDomains)

	// 配置三个选项：开启代理、设置地址、设置绕过列表
	options := []INTERNET_PER_CONN_OPTION{
		{dwOption: INTERNET_PER_CONN_FLAGS, Value: PROXY_TYPE_DIRECT | PROXY_TYPE_PROXY},
		{dwOption: INTERNET_PER_CONN_PROXY_SERVER, Value: uint64(uintptr(unsafe.Pointer(serverPtr)))},
		{dwOption: INTERNET_PER_CONN_PROXY_BYPASS, Value: uint64(uintptr(unsafe.Pointer(bypassPtr)))},
	}

	list := INTERNET_PER_CONN_OPTION_LIST{
		dwSize:        uint32(unsafe.Sizeof(INTERNET_PER_CONN_OPTION_LIST{})),
		pszConnection: nil, // 先设置默认局域网
		dwOptionCount: uint32(len(options)),
		pOptions:      &options[0],
	}

	// 1. 设置局域网代理
	ret, _, err := procInternetSetOption.Call(
		0,
		uintptr(INTERNET_OPTION_PER_CONNECTION_OPTION),
		uintptr(unsafe.Pointer(&list)),
		uintptr(list.dwSize),
	)
	if ret == 0 {
		return fmt.Errorf("设置默认连接代理失败: %v", err)
	}

	// 2. 遍历并设置所有的拨号/VPN (RAS) 连接
	setRasProxy(&list)

	// 3. 发送广播，强制系统和浏览器立刻应用新设置！
	procInternetSetOption.Call(0, uintptr(INTERNET_OPTION_SETTINGS_CHANGED), 0, 0)
	procInternetSetOption.Call(0, uintptr(INTERNET_OPTION_REFRESH), 0, 0)

	log.Printf("系统代理设置成功: %s", serverStr)
	return nil
}

// DisableSystemProxy 关闭系统代理并恢复直连
func DisableSystemProxy() error {
	options := []INTERNET_PER_CONN_OPTION{
		{dwOption: INTERNET_PER_CONN_FLAGS, Value: PROXY_TYPE_DIRECT},
	}

	list := INTERNET_PER_CONN_OPTION_LIST{
		dwSize:        uint32(unsafe.Sizeof(INTERNET_PER_CONN_OPTION_LIST{})),
		pszConnection: nil,
		dwOptionCount: uint32(len(options)),
		pOptions:      &options[0],
	}

	// 1. 关闭局域网代理
	ret, _, err := procInternetSetOption.Call(
		0,
		uintptr(INTERNET_OPTION_PER_CONNECTION_OPTION),
		uintptr(unsafe.Pointer(&list)),
		uintptr(list.dwSize),
	)
	if ret == 0 {
		return fmt.Errorf("关闭代理选项失败: %v", err)
	}

	// 2. 关闭所有拨号/VPN (RAS) 的代理
	setRasProxy(&list)

	// 3. 广播刷新
	procInternetSetOption.Call(0, uintptr(INTERNET_OPTION_SETTINGS_CHANGED), 0, 0)
	procInternetSetOption.Call(0, uintptr(INTERNET_OPTION_REFRESH), 0, 0)

	log.Println("系统代理已彻底禁用")
	return nil
}

// setRasProxy 同步配置给所有拨号/VPN连接 (复刻 Stelliberty 的精髓)
func setRasProxy(list *INTERNET_PER_CONN_OPTION_LIST) {
	var cb uint32 = uint32(unsafe.Sizeof(RasEntryName{}))
	var cEntries uint32 = 0

	entry := RasEntryName{dwSize: cb}

	// 第一次调用，获取需要的内存大小和条目数
	ret, _, _ := procRasEnumEntries.Call(
		0, 0,
		uintptr(unsafe.Pointer(&entry)),
		uintptr(unsafe.Pointer(&cb)),
		uintptr(unsafe.Pointer(&cEntries)),
	)

	// 如果有条目且缓冲区不足，重新分配切片
	if ret == ERROR_BUFFER_TOO_SMALL && cEntries > 0 {
		entries := make([]RasEntryName, cEntries)
		entries[0].dwSize = uint32(unsafe.Sizeof(RasEntryName{}))

		ret, _, _ = procRasEnumEntries.Call(
			0, 0,
			uintptr(unsafe.Pointer(&entries[0])),
			uintptr(unsafe.Pointer(&cb)),
			uintptr(unsafe.Pointer(&cEntries)),
		)

		if ret == ERROR_SUCCESS {
			for i := uint32(0); i < cEntries; i++ {
				// 将当前遍历到的连接名称赋给配置列表
				list.pszConnection = &entries[i].szEntryName[0]

				// 为该连接应用代理设置
				procInternetSetOption.Call(
					0,
					uintptr(INTERNET_OPTION_PER_CONNECTION_OPTION),
					uintptr(unsafe.Pointer(list)),
					uintptr(list.dwSize),
				)
			}
		}
	}
}
