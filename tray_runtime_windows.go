//go:build windows

package main

import (
	"context"
	"fmt"
	"goclashz/core/appcore"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Win32 constants
const (
	WM_USER           = 0x0400
	WM_TRAYICON       = WM_USER + 1
	WM_TRAYRENDER     = WM_USER + 2
	WM_TRAYQUIT       = WM_USER + 3
	WM_TRAY_ADD_RETRY = WM_USER + 4
	WM_NULL           = 0x0000
	WM_COMMAND        = 0x0111
	WM_RBUTTONUP      = 0x0205
	WM_LBUTTONDBLCLK  = 0x0203
	WM_DESTROY        = 0x0002

	NIM_ADD     = 0x00000000
	NIM_MODIFY  = 0x00000001
	NIM_DELETE  = 0x00000002
	NIF_MESSAGE = 0x00000001
	NIF_ICON    = 0x00000002
	NIF_TIP     = 0x00000004

	MF_STRING    = 0x00000000
	MF_SEPARATOR = 0x00000800
	MF_CHECKED   = 0x00000008
	MF_POPUP     = 0x00000010

	TPM_RIGHTBUTTON = 0x0002
	TPM_RETURNCMD   = 0x0100

	IMAGE_ICON      = 1
	LR_DEFAULTSIZE  = 0x00000040
	LR_LOADFROMFILE = 0x00000010

	IDI_APPLICATION = 32512
)

// Menu command IDs
const (
	idShow       = 1001
	idSysProxy   = 1002
	idTun        = 1003
	idModeRule   = 1004
	idModeGlobal = 1005
	idModeDirect = 1006
	idRestart    = 1007
	idQuit       = 1008
)

// TrayCommand represents a command from the tray menu
type TrayCommand int

const (
	TrayCmdShowMainWindow TrayCommand = iota
	TrayCmdToggleSystemProxy
	TrayCmdToggleTun
	TrayCmdModeRule
	TrayCmdModeGlobal
	TrayCmdModeDirect
	TrayCmdRestartCore
	TrayCmdQuitApp
)

// Win32 structures
type NOTIFYICONDATA struct {
	CbSize           uint32
	HWnd             windows.HWND
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UTimeoutVersion  uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     uintptr
}

type POINT struct {
	X int32
	Y int32
}

type MSG struct {
	HWnd    windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

type WNDCLASSEX struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         uintptr
	HCursor       windows.Handle
	HbrBackground windows.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

// Win32 API functions
var (
	user32   = windows.NewLazyDLL("user32.dll")
	shell32  = windows.NewLazyDLL("shell32.dll")
	kernel32 = windows.NewLazyDLL("kernel32.dll")

	procRegisterClassExW       = user32.NewProc("RegisterClassExW")
	procCreateWindowExW        = user32.NewProc("CreateWindowExW")
	procDefWindowProcW         = user32.NewProc("DefWindowProcW")
	procGetMessageW            = user32.NewProc("GetMessageW")
	procTranslateMessage       = user32.NewProc("TranslateMessage")
	procDispatchMessageW       = user32.NewProc("DispatchMessageW")
	procPostMessageW           = user32.NewProc("PostMessageW")
	procPostQuitMessage        = user32.NewProc("PostQuitMessage")
	procSetForegroundWindow    = user32.NewProc("SetForegroundWindow")
	procGetCursorPos           = user32.NewProc("GetCursorPos")
	procLoadIconW              = user32.NewProc("LoadIconW")
	procLoadImageW             = user32.NewProc("LoadImageW")
	procDestroyIcon            = user32.NewProc("DestroyIcon")
	procCreatePopupMenu        = user32.NewProc("CreatePopupMenu")
	procDestroyMenu            = user32.NewProc("DestroyMenu")
	procAppendMenuW            = user32.NewProc("AppendMenuW")
	procTrackPopupMenu         = user32.NewProc("TrackPopupMenu")
	procGetModuleHandleW       = kernel32.NewProc("GetModuleHandleW")
	procRegisterWindowMessageW = user32.NewProc("RegisterWindowMessageW")
	procShell_NotifyIconW      = shell32.NewProc("Shell_NotifyIconW")
)

// TrayRuntime is the Win32-based system tray implementation
type TrayRuntime struct {
	app  *App
	logf func(level string, format string, args ...any)

	ctx    context.Context
	cancel context.CancelFunc

	cmdCh chan TrayCommand

	started atomic.Bool
	ready   atomic.Bool

	latestMu sync.Mutex
	latest   appcore.AppState

	// Win32 state
	hwnd              windows.HWND
	hIcon             uintptr
	ownsIcon          bool
	iconAdded         atomic.Bool
	taskbarCreatedMsg uint32

	// State coalescing
	renderQueued atomic.Bool

	retryAddCount atomic.Int32
}

// NewTrayRuntime creates a new Win32 tray runtime
func NewTrayRuntime(app *App) *TrayRuntime {
	return &TrayRuntime{
		app:   app,
		cmdCh: make(chan TrayCommand, 10),
		logf:  app.logApp,
	}
}

// Start initializes and starts the tray runtime
func (t *TrayRuntime) Start(ctx context.Context) {
	if !t.started.CompareAndSwap(false, true) {
		return
	}

	t.ctx, t.cancel = context.WithCancel(ctx)

	go t.runWin32TrayLoop()
	go t.commandLoop()
}

// Stop gracefully stops the tray runtime
func (t *TrayRuntime) Stop() {
	if t.cancel != nil {
		t.cancel()
	}

	if t.ready.Load() && t.hwnd != 0 {
		procPostMessageW.Call(uintptr(t.hwnd), WM_TRAYQUIT, 0, 0)
	}
}

// UpdateState posts a state update to the tray thread
func (t *TrayRuntime) UpdateState(state appcore.AppState) {
	t.latestMu.Lock()
	t.latest = state
	t.latestMu.Unlock()

	if !t.ready.Load() || t.hwnd == 0 {
		return
	}

	if !t.iconAdded.Load() {
		procPostMessageW.Call(uintptr(t.hwnd), WM_TRAY_ADD_RETRY, 0, 0)
		return
	}

	if t.renderQueued.CompareAndSwap(false, true) {
		r, _, _ := procPostMessageW.Call(uintptr(t.hwnd), WM_TRAYRENDER, 0, 0)
		if r == 0 {
			t.renderQueued.Store(false)
		}
	}
}

// PostCommand posts a command to be executed
func (t *TrayRuntime) PostCommand(cmd TrayCommand) {
	select {
	case t.cmdCh <- cmd:
	default:
	}
}

// snapshot returns the latest state
func (t *TrayRuntime) snapshot() appcore.AppState {
	t.latestMu.Lock()
	defer t.latestMu.Unlock()
	return t.latest
}

// runWin32TrayLoop runs the Win32 message loop on a locked OS thread
func (t *TrayRuntime) runWin32TrayLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	defer func() {
		t.ready.Store(false)
		t.started.Store(false)
		t.renderQueued.Store(false)
		t.iconAdded.Store(false)
		t.hwnd = 0
		t.hIcon = 0
		t.ownsIcon = false
	}()

	// Register TaskbarCreated message for Explorer restart recovery
	taskbarCreatedPtr, _ := windows.UTF16PtrFromString("TaskbarCreated")
	ret, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(taskbarCreatedPtr)))
	t.taskbarCreatedMsg = uint32(ret)

	// Create hidden window
	hwnd := t.createHiddenWindow()
	if hwnd == 0 {
		t.logf("error", "[Tray] Failed to create tray window")
		return
	}
	t.hwnd = hwnd

	// Load icon
	t.loadIcon()

	// Initial add tray icon attempt, if fails, schedule async retry
	if err := t.addTrayIcon(); err != nil {
		t.scheduleTrayAddRetry()
	}

	t.ready.Store(true)

	// Initial render with current state
	t.renderQueued.Store(false)
	t.renderOnTrayThread(t.snapshot())

	// Message loop
	var msg MSG
	for {
		ret, _, _ := procGetMessageW.Call(
			uintptr(unsafe.Pointer(&msg)),
			0,
			0,
			0,
		)
		if ret == 0 || ret == ^uintptr(0) {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	// Cleanup
	t.deleteTrayIcon()
	if t.hIcon != 0 && t.ownsIcon {
		procDestroyIcon.Call(t.hIcon)
	}
}

// createHiddenWindow creates a hidden window for receiving messages
func (t *TrayRuntime) createHiddenWindow() windows.HWND {
	className, _ := windows.UTF16PtrFromString("GoclashZTrayWindow")

	hInstance, _, _ := procGetModuleHandleW.Call(0)

	wndProc := windows.NewCallback(t.wndProc)

	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   wndProc,
		HInstance:     windows.Handle(hInstance),
		LpszClassName: className,
	}

	r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if r == 0 {
		if errno, ok := err.(syscall.Errno); ok && errno == windows.ERROR_CLASS_ALREADY_EXISTS {
			t.logf("debug", "[Tray] window class already exists")
		} else {
			t.logf("error", "[Tray] RegisterClassExW failed: %v", err)
			return 0
		}
	}

	hwnd, _, err := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(className)),
		0,
		0, 0, 0, 0,
		0,
		0,
		hInstance,
		0,
	)
	if hwnd == 0 {
		t.logf("error", "[Tray] CreateWindowExW failed: %v", err)
	}

	return windows.HWND(hwnd)
}

// loadIcon loads the application icon
func (t *TrayRuntime) loadIcon() {
	if len(iconData) == 0 {
		ret, _, _ := procLoadIconW.Call(0, IDI_APPLICATION)
		t.hIcon = ret
		t.ownsIcon = false
		return
	}

	iconPath := filepath.Join(os.TempDir(), fmt.Sprintf("goclashz_tray_%d.ico", os.Getpid()))
	if err := os.WriteFile(iconPath, iconData, 0644); err == nil {
		defer os.Remove(iconPath)

		pathPtr, _ := windows.UTF16PtrFromString(iconPath)
		ret, _, _ := procLoadImageW.Call(
			0,
			uintptr(unsafe.Pointer(pathPtr)),
			IMAGE_ICON,
			0, 0,
			LR_LOADFROMFILE|LR_DEFAULTSIZE,
		)
		if ret != 0 {
			t.hIcon = ret
			t.ownsIcon = true
			return
		}
	}

	// Fallback to default application icon (shared, do not destroy)
	ret, _, _ := procLoadIconW.Call(0, IDI_APPLICATION)
	t.hIcon = ret
	t.ownsIcon = false
}

// shellNotifyIcon wraps Shell_NotifyIconW with error checking
func shellNotifyIcon(op uintptr, nid *NOTIFYICONDATA) error {
	r, _, err := procShell_NotifyIconW.Call(op, uintptr(unsafe.Pointer(nid)))
	if r == 0 {
		return fmt.Errorf("Shell_NotifyIcon op=%d: %w", op, err)
	}
	return nil
}

// addTrayIcon adds the tray icon
func (t *TrayRuntime) addTrayIcon() error {
	if t.hwnd == 0 {
		return fmt.Errorf("hwnd is nil")
	}

	nid := NOTIFYICONDATA{
		CbSize:           uint32(unsafe.Sizeof(NOTIFYICONDATA{})),
		HWnd:             t.hwnd,
		UID:              1,
		UFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP,
		UCallbackMessage: WM_TRAYICON,
		HIcon:            t.hIcon,
	}

	state := t.snapshot()
	tip := "GoclashZ - 未选择配置"
	if state.ActiveConfigName != "" {
		tip = "GoclashZ - " + state.ActiveConfigName
	}
	copy(nid.SzTip[:], windows.StringToUTF16(tip))

	err := shellNotifyIcon(NIM_ADD, &nid)
	if err == nil {
		t.iconAdded.Store(true)
	}
	return err
}

// scheduleTrayAddRetry schedules a retry for adding the tray icon
func (t *TrayRuntime) scheduleTrayAddRetry() {
	count := t.retryAddCount.Add(1)
	if count > 3 {
		return
	}
	go func(retryNum int32) {
		time.Sleep(time.Duration(retryNum) * time.Second)
		procPostMessageW.Call(uintptr(t.hwnd), WM_TRAY_ADD_RETRY, 0, 0)
	}(count)
}

// deleteTrayIcon removes the tray icon
func (t *TrayRuntime) deleteTrayIcon() {
	if t.hwnd == 0 || !t.iconAdded.Load() {
		return
	}

	nid := NOTIFYICONDATA{
		CbSize: uint32(unsafe.Sizeof(NOTIFYICONDATA{})),
		HWnd:   t.hwnd,
		UID:    1,
	}

	if err := shellNotifyIcon(NIM_DELETE, &nid); err != nil {
		t.logf("error", "[Tray] deleteTrayIcon failed: %v", err)
	}
	t.iconAdded.Store(false)
}

// updateTrayTooltip updates the tray icon tooltip
func (t *TrayRuntime) updateTrayTooltip(tooltip string) {
	if t.hwnd == 0 || !t.iconAdded.Load() {
		return
	}

	nid := NOTIFYICONDATA{
		CbSize: uint32(unsafe.Sizeof(NOTIFYICONDATA{})),
		HWnd:   t.hwnd,
		UID:    1,
		UFlags: NIF_TIP,
	}

	copy(nid.SzTip[:], windows.StringToUTF16(tooltip))

	if err := shellNotifyIcon(NIM_MODIFY, &nid); err != nil {
		t.logf("warn", "[Tray] updateTrayTooltip failed: %v", err)

		// 轻量自愈：NIM_MODIFY 失败时尝试重新添加一次
		if err := t.addTrayIcon(); err != nil {
			t.logf("error", "[Tray] re-add tray icon after modify failure failed: %v", err)
		}
	}
}

// wndProc is the window procedure for handling messages
func (t *TrayRuntime) wndProc(hwnd windows.HWND, msg uint32, wparam uintptr, lparam uintptr) uintptr {
	// Handle TaskbarCreated message (Explorer restart)
	if msg == t.taskbarCreatedMsg && t.taskbarCreatedMsg != 0 {
		t.logf("info", "[Tray] Explorer restarted, re-adding tray icon")
		t.retryAddCount.Store(0)
		if err := t.addTrayIcon(); err != nil {
			t.scheduleTrayAddRetry()
		} else {
			t.renderOnTrayThread(t.snapshot())
		}
		return 0
	}

	switch msg {
	case WM_TRAY_ADD_RETRY:
		if err := t.addTrayIcon(); err != nil {
			t.logf("warn", "[Tray] async add tray icon failed: %v", err)
			t.scheduleTrayAddRetry()
		} else {
			t.retryAddCount.Store(0)
			t.renderOnTrayThread(t.snapshot())
		}
		return 0
	case WM_TRAYICON:
		switch lparam {
		case WM_RBUTTONUP:
			t.showContextMenu()
		case WM_LBUTTONDBLCLK:
			t.PostCommand(TrayCmdShowMainWindow)
		}
		return 0

	case WM_TRAYRENDER:
		t.renderQueued.Store(false)
		t.renderOnTrayThread(t.snapshot())
		return 0

	case WM_TRAYQUIT:
		t.deleteTrayIcon()
		procPostQuitMessage.Call(0)
		return 0

	case WM_COMMAND:
		cmdID := uint32(wparam & 0xFFFF)
		t.dispatchMenuCommand(cmdID)
		return 0

	case WM_DESTROY:
		t.deleteTrayIcon()
		procPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(
		uintptr(hwnd),
		uintptr(msg),
		wparam,
		lparam,
	)
	return ret
}

// renderOnTrayThread updates the tray state (must be called from tray thread)
func (t *TrayRuntime) renderOnTrayThread(state appcore.AppState) {
	if !t.ready.Load() || !t.iconAdded.Load() {
		return
	}

	tooltip := "GoclashZ - 未选择配置"
	if state.ActiveConfigName != "" {
		tooltip = "GoclashZ - " + state.ActiveConfigName
	}

	t.updateTrayTooltip(tooltip)
}

// showContextMenu creates and shows the context menu (called from tray thread)
func (t *TrayRuntime) showContextMenu() {
	state := t.snapshot()

	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	t.appendMenu(hMenu, idShow, "显示界面", false)

	procAppendMenuW.Call(hMenu, MF_SEPARATOR, 0, 0)

	t.appendMenu(hMenu, idSysProxy, "系统代理", state.SystemProxy)
	t.appendMenu(hMenu, idTun, "TUN 模式", state.Tun)

	procAppendMenuW.Call(hMenu, MF_SEPARATOR, 0, 0)

	hModeMenu, _, _ := procCreatePopupMenu.Call()
	if hModeMenu != 0 {
		t.appendMenu(hModeMenu, idModeRule, "规则 (Rule)", state.Mode == "rule")
		t.appendMenu(hModeMenu, idModeGlobal, "全局 (Global)", state.Mode == "global")
		t.appendMenu(hModeMenu, idModeDirect, "直连 (Direct)", state.Mode == "direct")

		modeText, _ := windows.UTF16PtrFromString("出站模式")
		procAppendMenuW.Call(hMenu, MF_POPUP, hModeMenu, uintptr(unsafe.Pointer(modeText)))
	}

	procAppendMenuW.Call(hMenu, MF_SEPARATOR, 0, 0)

	t.appendMenu(hMenu, idRestart, "重启内核", false)
	t.appendMenu(hMenu, idQuit, "退出程序", false)

	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	procSetForegroundWindow.Call(uintptr(t.hwnd))

	ret, _, _ := procTrackPopupMenu.Call(
		hMenu,
		TPM_RIGHTBUTTON|TPM_RETURNCMD,
		uintptr(pt.X),
		uintptr(pt.Y),
		0,
		uintptr(t.hwnd),
		0,
	)

	// Post WM_NULL to dismiss menu properly (Win32 requirement)
	procPostMessageW.Call(uintptr(t.hwnd), WM_NULL, 0, 0)

	if ret > 0 {
		t.dispatchMenuCommand(uint32(ret))
	}
}

// appendMenu adds a menu item with optional check mark
func (t *TrayRuntime) appendMenu(hMenu uintptr, id uint32, text string, checked bool) {
	flags := MF_STRING
	if checked {
		flags |= MF_CHECKED
	}

	textPtr, _ := windows.UTF16PtrFromString(text)
	procAppendMenuW.Call(hMenu, uintptr(flags), uintptr(id), uintptr(unsafe.Pointer(textPtr)))
}

// dispatchMenuCommand dispatches menu commands to the command channel
func (t *TrayRuntime) dispatchMenuCommand(id uint32) {
	switch id {
	case idShow:
		t.PostCommand(TrayCmdShowMainWindow)
	case idSysProxy:
		t.PostCommand(TrayCmdToggleSystemProxy)
	case idTun:
		t.PostCommand(TrayCmdToggleTun)
	case idModeRule:
		t.PostCommand(TrayCmdModeRule)
	case idModeGlobal:
		t.PostCommand(TrayCmdModeGlobal)
	case idModeDirect:
		t.PostCommand(TrayCmdModeDirect)
	case idRestart:
		t.PostCommand(TrayCmdRestartCore)
	case idQuit:
		t.PostCommand(TrayCmdQuitApp)
	}
}

// commandLoop processes commands from the command channel
func (t *TrayRuntime) commandLoop() {
	for {
		select {
		case <-t.ctx.Done():
			return
		case cmd := <-t.cmdCh:
			t.handleCommand(cmd)
		}
	}
}

// reportTrayError reports tray command errors to the UI
func (t *TrayRuntime) reportTrayError(action string, err error) {
	if err == nil {
		return
	}
	t.logf("error", "[Tray] %s failed: %v", action, err)
	t.app.core.GetEvents().Emit("notify-error", fmt.Sprintf("%s失败: %v", action, err))
}

// handleCommand executes a tray command
func (t *TrayRuntime) handleCommand(cmd TrayCommand) {
	defer func() {
		if r := recover(); r != nil {
			t.logf("error", "[Tray] Command panic: %v", r)
		}
	}()

	switch cmd {
	case TrayCmdShowMainWindow:
		t.app.safeShowMainWindow()

	case TrayCmdToggleSystemProxy:
		state := t.snapshot()
		err := t.app.core.ToggleSystemProxy(t.ctx, !state.SystemProxy)
		t.reportTrayError("切换系统代理", err)

	case TrayCmdToggleTun:
		state := t.snapshot()
		err := t.app.core.ToggleTunMode(t.ctx, !state.Tun)
		t.reportTrayError("切换 TUN", err)

	case TrayCmdModeRule:
		err := t.app.core.UpdateClashMode(t.ctx, "rule")
		t.reportTrayError("切换模式", err)

	case TrayCmdModeGlobal:
		err := t.app.core.UpdateClashMode(t.ctx, "global")
		t.reportTrayError("切换模式", err)

	case TrayCmdModeDirect:
		err := t.app.core.UpdateClashMode(t.ctx, "direct")
		t.reportTrayError("切换模式", err)

	case TrayCmdRestartCore:
		err := t.app.core.RestartCoreWithReason(t.ctx, "manual")
		t.reportTrayError("重启内核", err)

	case TrayCmdQuitApp:
		t.app.safeQuit()
	}
}
