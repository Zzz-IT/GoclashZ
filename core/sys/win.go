//go:build windows

package sys

import (
	"time"
	"unsafe"

	syswin "golang.org/x/sys/windows"
)

var (
	user32                  = syswin.NewLazySystemDLL("user32.dll")
	procFindWindow          = user32.NewProc("FindWindowW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procBringWindowToTop    = user32.NewProc("BringWindowToTop")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procIsIconic            = user32.NewProc("IsIconic")
	procFlashWindowEx       = user32.NewProc("FlashWindowEx")
)

// FLASHWINFO 结构体，用于配置闪烁行为
type FLASHWINFO struct {
	CbSize    uint32
	Hwnd      syswin.Handle
	DwFlags   uint32
	UCount    uint32
	DwTimeout uint32
}

const (
	FLASHW_STOP      = 0x00000000
	FLASHW_CAPTION   = 0x00000001
	FLASHW_TRAY      = 0x00000002
	FLASHW_ALL       = 0x00000003
	FLASHW_TIMERNOFG = 0x0000000C

	SW_RESTORE = 9
)

type MainWindowState struct {
	Hwnd       uintptr
	Visible    bool
	Minimized  bool
	Foreground bool
}

// FindMainWindow 根据窗口标题查找主窗口句柄
func FindMainWindow() uintptr {
	windowName, _ := syswin.UTF16PtrFromString("GoclashZ")
	hwnd, _, _ := procFindWindow.Call(0, uintptr(unsafe.Pointer(windowName)))
	return hwnd
}

func GetMainWindowState() MainWindowState {
	hwnd := FindMainWindow()
	if hwnd == 0 {
		return MainWindowState{}
	}

	isVisible, _, _ := procIsWindowVisible.Call(hwnd)
	isMinimized, _, _ := procIsIconic.Call(hwnd)
	fgHwnd, _, _ := procGetForegroundWindow.Call()

	return MainWindowState{
		Hwnd:       hwnd,
		Visible:    isVisible != 0,
		Minimized:  isMinimized != 0,
		Foreground: hwnd == fgHwnd,
	}
}

// IsMainWindowShowing 检查主窗口是否正在显示（非隐藏且非最小化）
func IsMainWindowShowing() bool {
	s := GetMainWindowState()
	return s.Hwnd != 0 && s.Visible && !s.Minimized
}

// StopTaskbarFlash 强行重置任务栏闪烁状态
func StopTaskbarFlash(hwnd uintptr) {
	if hwnd == 0 {
		return
	}

	finfo := FLASHWINFO{
		CbSize:    uint32(unsafe.Sizeof(FLASHWINFO{})),
		Hwnd:      syswin.Handle(hwnd),
		DwFlags:   FLASHW_STOP,
		UCount:    0,
		DwTimeout: 0,
	}
	procFlashWindowEx.Call(uintptr(unsafe.Pointer(&finfo)))
}

// FocusWindow 将窗口暴力拉到最前台并获取焦点
func FocusWindow(hwnd uintptr) {
	if hwnd == 0 {
		return
	}

	// 先恢复窗口，覆盖隐藏、最小化等状态
	procShowWindow.Call(hwnd, SW_RESTORE)

	// 尽量置顶并转移焦点
	procBringWindowToTop.Call(hwnd)
	procSetForegroundWindow.Call(hwnd)
}

// FlashWindowTwice 执行标准化的“快速”闪烁并自动清理
func FlashWindowTwice(hwnd uintptr) {
	if hwnd == 0 {
		return
	}

	// 1. 先清一次旧状态
	StopTaskbarFlash(hwnd)

	// 2. 节奏加快 (150ms)，次数 2 次，针对 TRAY 闪烁
	finfo := FLASHWINFO{
		CbSize:    uint32(unsafe.Sizeof(FLASHWINFO{})),
		Hwnd:      syswin.Handle(hwnd),
		DwFlags:   FLASHW_TRAY,
		UCount:    2,
		DwTimeout: 150,
	}
	procFlashWindowEx.Call(uintptr(unsafe.Pointer(&finfo)))

	// 3. 异步延时等待闪烁动作完成，然后强制清空状态位
	go func() {
		time.Sleep(800 * time.Millisecond)
		StopTaskbarFlash(hwnd)
	}()
}

// WaitMainWindowHandle 等待主窗口句柄出现（带重试）
func WaitMainWindowHandle() uintptr {
	for i := 0; i < 10; i++ {
		if hwnd := FindMainWindow(); hwnd != 0 {
			return hwnd
		}
		time.Sleep(30 * time.Millisecond)
	}
	return 0
}

// FocusMainWindowAndFlashTwiceWin32Only 供单实例碰撞时使用的纯 Win32 唤醒入口
func FocusMainWindowAndFlashTwiceWin32Only() {
	hwnd := WaitMainWindowHandle()
	if hwnd == 0 {
		return
	}

	FocusWindow(hwnd)
	FlashWindowTwice(hwnd)
}
