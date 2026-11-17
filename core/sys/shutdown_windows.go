//go:build windows

package sys

import (
	"goclashz/core/logger"
	"golang.org/x/sys/windows"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	procCreateEventW = kernel32.NewProc("CreateEventW")
	procSetEvent     = kernel32.NewProc("SetEvent")
)

// RequestExistingInstanceQuit triggers the shutdown event to ask the running instance to quit
func RequestExistingInstanceQuit() {
	eventName, _ := syscall.UTF16PtrFromString("Global\\GoclashZ_Shutdown_Event")
	handle, _, _ := procCreateEventW.Call(
		0,
		1, // ManualReset = TRUE
		0, // InitialState = FALSE
		uintptr(unsafe.Pointer(eventName)),
	)
	if handle != 0 {
		procSetEvent.Call(handle)
		windows.CloseHandle(windows.Handle(handle))
	}
}

// ListenForShutdownEvent blocks until the shutdown event is triggered, then returns
//
// F-A5-25：唯一的调用方（main.go 的守护 goroutine）把「本函数返回」直接当成
// 「安装器请求退出」→ ClearOwnedSystemProxy() + os.Exit(0)。因此创建事件失败时
// **绝不能返回**，否则应用会在启动后立刻静默退出。这里改为永久阻塞并落错误日志：
// 本次没有监听能力，但绝不会误触发退出。
func ListenForShutdownEvent() {
	eventName, _ := syscall.UTF16PtrFromString("Global\\GoclashZ_Shutdown_Event")
	handle, _, callErr := procCreateEventW.Call(
		0,
		1, // ManualReset = TRUE
		0, // InitialState = FALSE
		uintptr(unsafe.Pointer(eventName)),
	)
	if handle == 0 {
		logger.Errorf("创建安装器退出事件失败（将不再响应安装器退出请求，但不会误退出应用）: %v", callErr)
		blockForever()
	}

	defer windows.CloseHandle(windows.Handle(handle))
	if _, err := windows.WaitForSingleObject(windows.Handle(handle), windows.INFINITE); err != nil {
		logger.Errorf("等待安装器退出事件失败（将不再响应安装器退出请求）: %v", err)
		blockForever()
	}
}

// blockForever 让守护 goroutine 永久停住但不占 CPU。
// 用 Sleep 循环而不是 select{}：避免"所有 goroutine 都在等"时被运行时判定成死锁。
func blockForever() {
	for {
		time.Sleep(time.Hour)
	}
}
