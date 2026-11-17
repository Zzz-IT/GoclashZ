//go:build windows

package main

// 全局热键（v3 原生化，第二轮 #10）
//
// 用 v3 的 application.GlobalShortcut（Windows 下即 RegisterHotKey + 主线程隐藏窗口的
// WM_HOTKEY 消息）替换原先 63 行自研 RegisterHotKey + 独立消息循环线程的实现。
// 对外签名保持不变（app.go 调用）：
//
//	(*App).startHotkeyWorker(ctx context.Context)
//	(*App).stopHotkey()
//
// 行为对照：热键仍为 Ctrl+Alt+Q 退出；被其它程序占用时静默失败（仅记录日志，不弹窗、不影响启动）。

import (
	"context"
	"fmt"
	"time"

	"goclashz/core/appcore"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// reportHotkeyFailure 让「热键没生效」不再只沉在内存日志里（F-A5-21）：
// 落一条 warn 日志（进文件日志）+ 发一条 app-notification（source=hotkey），
// 由前端按自己的分级决定是否提示（A/B 类不应弹模态）。
func (a *App) reportHotkeyFailure(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	a.logApp("warn", "[Hotkey] %s", msg)
	if a == nil || a.core == nil {
		return
	}
	events := a.core.GetEvents()
	if events == nil {
		return
	}
	events.Emit(appcore.EventNotification, appcore.Notification{
		Level:   "warn",
		Code:    "HOTKEY_REGISTER_FAILED",
		Message: msg,
		Source:  "hotkey",
	})
}

// hotkeyQuitAccelerator 是退出程序的全局热键（等价 v2 的 MOD_CONTROL|MOD_ALT + VK_Q）。
const hotkeyQuitAccelerator = "Ctrl+Alt+Q"

// startHotkeyWorker 注册全局退出热键。
//
// v3 的 GlobalShortcutManager 内部保证 RegisterHotKey 在主线程执行：
//   - App.Run 尚未完成时调用 -> 登记为 pending，启动完成后由 flushPending 绑定，
//     OS 层失败只会走 ErrorHandler（即「静默失败」）；
//   - 应用运行后调用 -> 直接绑定，失败会返回 error。
//
// 两种情况都只记录日志，不影响启动。
func (a *App) startHotkeyWorker(ctx context.Context) {
	wailsApp := application.Get()
	if wailsApp == nil || wailsApp.GlobalShortcut == nil {
		a.reportHotkeyFailure("Wails 应用尚未创建，跳过全局热键注册")
		return
	}

	if err := wailsApp.GlobalShortcut.Register(hotkeyQuitAccelerator, func() {
		// 回调在框架的独立 goroutine 中执行，保持与 v2 一致的异步退出。
		go a.safeQuit()
	}); err != nil {
		a.reportHotkeyFailure("全局热键 %s 注册失败（可能已被其它程序占用）: %v", hotkeyQuitAccelerator, err)
		return
	}

	a.logApp("info", "[Hotkey] 已注册全局热键 %s（退出程序）", hotkeyQuitAccelerator)

	if ctx == nil {
		return
	}

	// 启动前注册的快捷键会延迟绑定，这里在启动完成后回查一次，确认 OS 层真的绑定成功。
	go func() {
		timer := time.NewTimer(3 * time.Second)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		if !wailsApp.GlobalShortcut.IsRegistered(hotkeyQuitAccelerator) {
			a.reportHotkeyFailure("全局热键 %s 未生效（可能已被其它程序占用）", hotkeyQuitAccelerator)
		}
	}()
}

// stopHotkey 注销全局退出热键。
func (a *App) stopHotkey() {
	wailsApp := application.Get()
	if wailsApp == nil || wailsApp.GlobalShortcut == nil {
		return
	}

	if err := wailsApp.GlobalShortcut.Unregister(hotkeyQuitAccelerator); err != nil {
		a.logApp("debug", "[Hotkey] 注销全局热键 %s 失败: %v", hotkeyQuitAccelerator, err)
	}
}
