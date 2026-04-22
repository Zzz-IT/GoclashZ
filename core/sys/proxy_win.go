//go:build windows
// +build windows

package sys

import (
	"fmt"
	"log"
	"runtime"
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

// ⚠️ 修复1：使用 uintptr 代替 uint64。
// 这样在 64 位系统下是 8 字节并自动产生 4 字节 Padding，
// 在 32 位系统下是 4 字节无 Padding，完美对齐 C++ 中的 union 结构
type INTERNET_PER_CONN_OPTION struct {
	dwOption uint32
	Value    uintptr
}

type INTERNET_PER_CONN_OPTION_LIST struct {
	dwSize        uint32
	pszConnection *uint16
	dwOptionCount uint32
	dwOptionError uint32
	pOptions      *INTERNET_PER_CONN_OPTION
}

type RasEntryName struct {
	dwSize      uint32
	szEntryName [RAS_MaxEntryName + 1]uint16
	dwFlags     uint32
	szPhonebook [MAX_PATH + 1]uint16
}

// EnableSystemProxy 开启系统代理
func EnableSystemProxy(host string, port int, bypassDomains string) error {
	serverStr := fmt.Sprintf("%s:%d", host, port)

	serverPtr, _ := syscall.UTF16PtrFromString(serverStr)
	bypassPtr, _ := syscall.UTF16PtrFromString(bypassDomains)

	options := []INTERNET_PER_CONN_OPTION{
		{dwOption: INTERNET_PER_CONN_FLAGS, Value: uintptr(PROXY_TYPE_DIRECT | PROXY_TYPE_PROXY)},
		{dwOption: INTERNET_PER_CONN_PROXY_SERVER, Value: uintptr(unsafe.Pointer(serverPtr))},
		{dwOption: INTERNET_PER_CONN_PROXY_BYPASS, Value: uintptr(unsafe.Pointer(bypassPtr))},
	}

	list := INTERNET_PER_CONN_OPTION_LIST{
		dwSize:        uint32(unsafe.Sizeof(INTERNET_PER_CONN_OPTION_LIST{})),
		pszConnection: nil, // LAN
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

	// 2. 设置 RAS 拨号/VPN 代理
	setRasProxy(&list)

	// 3. 全局广播，瞬间生效
	procInternetSetOption.Call(0, uintptr(INTERNET_OPTION_SETTINGS_CHANGED), 0, 0)
	procInternetSetOption.Call(0, uintptr(INTERNET_OPTION_REFRESH), 0, 0)

	log.Printf("系统代理设置成功: %s", serverStr)

	// ⚠️ 修复2：GC 护城河！必须保持这些变量在 Syscall 执行完毕前不被回收！
	runtime.KeepAlive(serverPtr)
	runtime.KeepAlive(bypassPtr)
	runtime.KeepAlive(options)
	runtime.KeepAlive(list)

	return nil
}

// DisableSystemProxy 关闭系统代理
func DisableSystemProxy() error {
	options := []INTERNET_PER_CONN_OPTION{
		{dwOption: INTERNET_PER_CONN_FLAGS, Value: uintptr(PROXY_TYPE_DIRECT)},
	}

	list := INTERNET_PER_CONN_OPTION_LIST{
		dwSize:        uint32(unsafe.Sizeof(INTERNET_PER_CONN_OPTION_LIST{})),
		pszConnection: nil,
		dwOptionCount: uint32(len(options)),
		pOptions:      &options[0],
	}

	ret, _, err := procInternetSetOption.Call(
		0,
		uintptr(INTERNET_OPTION_PER_CONNECTION_OPTION),
		uintptr(unsafe.Pointer(&list)),
		uintptr(list.dwSize),
	)
	if ret == 0 {
		return fmt.Errorf("关闭代理选项失败: %v", err)
	}

	setRasProxy(&list)

	procInternetSetOption.Call(0, uintptr(INTERNET_OPTION_SETTINGS_CHANGED), 0, 0)
	procInternetSetOption.Call(0, uintptr(INTERNET_OPTION_REFRESH), 0, 0)

	log.Println("系统代理已禁用")

	// 保护指针
	runtime.KeepAlive(options)
	runtime.KeepAlive(list)

	return nil
}

// ClearSystemProxy 是 DisableSystemProxy 的别名，用于启动清理
func ClearSystemProxy() error {
	return DisableSystemProxy()
}

func setRasProxy(list *INTERNET_PER_CONN_OPTION_LIST) {
	var cb uint32 = uint32(unsafe.Sizeof(RasEntryName{}))
	var cEntries uint32 = 0

	entry := RasEntryName{dwSize: cb}

	ret, _, _ := procRasEnumEntries.Call(
		0, 0,
		uintptr(unsafe.Pointer(&entry)),
		uintptr(unsafe.Pointer(&cb)),
		uintptr(unsafe.Pointer(&cEntries)),
	)

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
				list.pszConnection = &entries[i].szEntryName[0]

				procInternetSetOption.Call(
					0,
					uintptr(INTERNET_OPTION_PER_CONNECTION_OPTION),
					uintptr(unsafe.Pointer(list)),
					uintptr(list.dwSize),
				)
			}
		}
		// 保护 entries 切片不被回收
		runtime.KeepAlive(entries)
	}
}
