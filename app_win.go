//go:build windows

package main

import (
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"goclashz/core/sys"
)

// showMainWindowFromTrayAndFlash 给托盘使用：先让 Wails 显示窗口，再用 Win32 兜底抢焦点和闪烁。
func showMainWindowFromTrayAndFlash(a *App) {
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)

	// 🚀 保持与原有逻辑一致：如果有关联更新，则触发前端通知
	a.mu.RLock()
	ready := a.appUpdateReady
	ver := a.newAppVersion
	a.mu.RUnlock()
	if ready {
		runtime.EventsEmit(a.ctx, "app-update-ready", ver)
	}

	hwnd := sys.WaitMainWindowHandle()
	if hwnd == 0 {
		return
	}

	sys.FocusWindow(hwnd)
	sys.FlashWindowTwice(hwnd)
}

func hideMainWindowFromTrayNoFlash(a *App) {
	if hwnd := sys.FindMainWindow(); hwnd != 0 {
		sys.StopTaskbarFlash(hwnd)
	}
	
	runtime.WindowHide(a.ctx)
	// 短暂休眠确保窗口状态更新
	time.Sleep(50 * time.Millisecond)
	
	if hwnd := sys.FindMainWindow(); hwnd != 0 {
		sys.StopTaskbarFlash(hwnd)
	}
}

func toggleMainWindowFromTray(a *App) {
	// 只要窗口当前在屏幕上显示（无论是否是焦点），双击即隐藏
	if sys.IsMainWindowShowing() {
		hideMainWindowFromTrayNoFlash(a)
		return
	}

	showMainWindowFromTrayAndFlash(a)
}
