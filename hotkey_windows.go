//go:build windows

package main

import (
	"context"
	goRuntime "runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmHotkey      = 0x0312
	wmQuit        = 0x0012
	hotkeyIDQuit  = 1
	hotkeyModCtrl = 0x0002
	hotkeyModAlt  = 0x0001
	hotkeyKeyQ    = 0x51
)

func (a *App) startHotkeyWorker(ctx context.Context) {
	goRuntime.LockOSThread()
	defer goRuntime.UnlockOSThread()

	tid := windows.GetCurrentThreadId()
	a.hotkeyTID.Store(uint32(tid))

	user32 := windows.NewLazySystemDLL("user32.dll")
	regHotkey := user32.NewProc("RegisterHotKey")
	unregHotkey := user32.NewProc("UnregisterHotKey")
	getMsg := user32.NewProc("GetMessageW")

	ret, _, _ := regHotkey.Call(0, hotkeyIDQuit, hotkeyModCtrl|hotkeyModAlt, hotkeyKeyQ)
	if ret == 0 {
		a.hotkeyTID.Store(0)
		return
	}

	var msg struct {
		hWnd    uintptr
		message uint32
		wParam  uintptr
		lParam  uintptr
		time    uint32
		pt      struct{ x, y int32 }
	}

	for {
		ret, _, _ := getMsg.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if ret == 0 || ret == ^uintptr(0) {
			return
		}

		select {
		case <-ctx.Done():
			unregHotkey.Call(0, hotkeyIDQuit)
			return
		default:
		}

		if msg.message == wmHotkey && msg.wParam == hotkeyIDQuit {
			go a.safeQuit()
			return
		}
	}
}

func (a *App) stopHotkey() {
	tid := a.hotkeyTID.Swap(0)

	user32 := windows.NewLazySystemDLL("user32.dll")
	user32.NewProc("UnregisterHotKey").Call(0, hotkeyIDQuit)

	if tid != 0 {
		user32.NewProc("PostThreadMessageW").Call(uintptr(tid), wmQuit, 0, 0)
	}
}
