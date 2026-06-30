//go:build windows

package sys

import (
	"syscall"
	"unsafe"
	"golang.org/x/sys/windows"
)

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	procCreateEventW = kernel32.NewProc("CreateEventW")
	procSetEvent = kernel32.NewProc("SetEvent")
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
func ListenForShutdownEvent() {
	eventName, _ := syscall.UTF16PtrFromString("Global\\GoclashZ_Shutdown_Event")
	handle, _, _ := procCreateEventW.Call(
		0, 
		1, // ManualReset = TRUE
		0, // InitialState = FALSE
		uintptr(unsafe.Pointer(eventName)),
	)
	if handle != 0 {
		defer windows.CloseHandle(windows.Handle(handle))
		windows.WaitForSingleObject(windows.Handle(handle), windows.INFINITE)
	}
}
