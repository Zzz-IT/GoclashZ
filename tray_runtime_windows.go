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
	"unsafe"

	"golang.org/x/sys/windows"
)

// Win32 constants
const (
	WM_USER            = 0x0400
	WM_TRAYICON        = WM_USER + 1
	WM_TRAYRENDER      = WM_USER + 2
	WM_TRAYQUIT        = WM_USER + 3
	WM_COMMAND          = 0x0111
	WM_RBUTTONUP       = 0x0205
	WM_LBUTTONDBLCLK   = 0x0203
	WM_DESTROY         = 0x0002
	WM_CLOSE           = 0x0010

	NIM_ADD        = 0x00000000
	NIM_MODIFY     = 0x00000001
	NIM_DELETE     = 0x00000002
	NIF_MESSAGE    = 0x00000001
	NIF_ICON       = 0x00000002
	NIF_TIP        = 0x00000004

	MF_STRING     = 0x00000000
	MF_SEPARATOR  = 0x00000800
	MF_CHECKED    = 0x00000008
	MF_UNCHECKED  = 0x00000000
	MF_POPUP      = 0x00000010
	MF_GRAYED     = 0x00000001

	TPM_RIGHTBUTTON  = 0x0002
	TPM_RETURNCMD    = 0x0100

	CW_USEDEFAULT = 0x80000000

	IMAGE_ICON     = 1
	LR_DEFAULTSIZE = 0x00000040
	LR_LOADFROMFILE = 0x00000010

	IDI_APPLICATION = 32512

	GCLP_HICON = -14

	WNDPROC_STDCALL = 0
)

// Menu command IDs
const (
	idShow      = 1001
	idSysProxy  = 1002
	idTun       = 1003
	idModeRule  = 1004
	idModeGlobal = 1005
	idModeDirect = 1006
	idRestart   = 1007
	idQuit      = 1008
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
	user32    = windows.NewLazyDLL("user32.dll")
	shell32   = windows.NewLazyDLL("shell32.dll")
	kernel32  = windows.NewLazyDLL("kernel32.dll")
	gdi32     = windows.NewLazyDLL("gdi32.dll")

	procRegisterClassExW     = user32.NewProc("RegisterClassExW")
	procCreateWindowExW      = user32.NewProc("CreateWindowExW")
	procDefWindowProcW       = user32.NewProc("DefWindowProcW")
	procGetMessageW          = user32.NewProc("GetMessageW")
	procTranslateMessage     = user32.NewProc("TranslateMessage")
	procDispatchMessageW     = user32.NewProc("DispatchMessageW")
	procPostMessageW         = user32.NewProc("PostMessageW")
	procPostQuitMessage      = user32.NewProc("PostQuitMessage")
	procDestroyWindow        = user32.NewProc("DestroyWindow")
	procSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	procGetCursorPos         = user32.NewProc("GetCursorPos")
	procLoadIconW            = user32.NewProc("LoadIconW")
	procLoadImageW           = user32.NewProc("LoadImageW")
	procDestroyIcon          = user32.NewProc("DestroyIcon")
	procCreatePopupMenu      = user32.NewProc("CreatePopupMenu")
	procDestroyMenu          = user32.NewProc("DestroyMenu")
	procAppendMenuW          = user32.NewProc("AppendMenuW")
	procTrackPopupMenu       = user32.NewProc("TrackPopupMenu")
	procCheckMenuItem        = user32.NewProc("CheckMenuItem")
	procGetModuleHandleW     = kernel32.NewProc("GetModuleHandleW")
	procRegisterWindowMessageW = user32.NewProc("RegisterWindowMessageW")

	procShell_NotifyIconW    = shell32.NewProc("Shell_NotifyIconW")

	procCreateIconFromResource = user32.NewProc("CreateIconFromResource")
)

// TrayRuntime is the Win32-based system tray implementation
type TrayRuntime struct {
	app *App

	ctx    context.Context
	cancel context.CancelFunc

	stateCh   chan appcore.AppState
	cmdCh     chan TrayCommand

	started atomic.Bool
	ready   atomic.Bool

	latestMu    sync.Mutex
	latest      appcore.AppState

	// Win32 state
	hwnd             windows.HWND
	hIcon            uintptr
	taskbarCreatedMsg uint32

	// State for menu rendering
	checkedSysProxy bool
	checkedTun      bool
	mode            string

	// State coalescing
	renderQueued atomic.Bool
}

// NewTrayRuntime creates a new Win32 tray runtime
func NewTrayRuntime(app *App) *TrayRuntime {
	return &TrayRuntime{
		app:    app,
		stateCh: make(chan appcore.AppState, 10),
		cmdCh:   make(chan TrayCommand, 10),
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

	if !t.renderQueued.CompareAndSwap(false, true) {
		return
	}

	if t.ready.Load() && t.hwnd != 0 {
		procPostMessageW.Call(uintptr(t.hwnd), WM_TRAYRENDER, 0, 0)
	}
}

// RunAction posts a function to be executed (for compatibility with existing code)
func (t *TrayRuntime) RunAction(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Tray action error: %v\n", r)
			}
		}()
		fn()
	}()
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

	// Register TaskbarCreated message for Explorer restart recovery
	taskbarCreatedPtr, _ := windows.UTF16PtrFromString("TaskbarCreated")
	ret, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(taskbarCreatedPtr)))
	t.taskbarCreatedMsg = uint32(ret)

	// Create hidden window
	hwnd := t.createHiddenWindow()
	if hwnd == 0 {
		fmt.Println("Failed to create tray window")
		return
	}
	t.hwnd = hwnd

	// Load icon
	t.loadIcon()

	// Add tray icon
	t.addTrayIcon()

	t.ready.Store(true)

	// Initial render if we have state
	state := t.snapshot()
	if state != (appcore.AppState{}) {
		t.renderOnTrayThread(state)
	}

	// Message loop
	var msg MSG
	for {
		ret, _, _ := procGetMessageW.Call(
			uintptr(unsafe.Pointer(&msg)),
			0,
			0,
			0,
		)
		if ret == 0 || ret == ^uintptr(0) { // WM_QUIT or error
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	// Cleanup
	t.deleteTrayIcon()
	if t.hIcon != 0 {
		procDestroyIcon.Call(uintptr(t.hIcon))
	}
}

// createHiddenWindow creates a hidden window for receiving messages
func (t *TrayRuntime) createHiddenWindow() windows.HWND {
	className, _ := windows.UTF16PtrFromString("GoclashZTrayWindow")

	hInstance, _, _ := procGetModuleHandleW.Call(0)

	// Create callback for WndProc
	wndProc := windows.NewCallback(t.wndProc)

	wc := WNDCLASSEX{
		CbSize:      uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc: wndProc,
		HInstance:   windows.Handle(hInstance),
		LpszClassName: className,
	}

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	// Create the window
	hwnd, _, _ := procCreateWindowExW.Call(
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

	return windows.HWND(hwnd)
}

// loadIcon loads the application icon
func (t *TrayRuntime) loadIcon() {
	if len(iconData) == 0 {
		// Use default application icon
		ret, _, _ := procLoadIconW.Call(0, IDI_APPLICATION)
		t.hIcon = ret
		return
	}

	// Try to load icon from embedded data via temp file
	iconPath := filepath.Join(os.TempDir(), "goclashz_tray.ico")
	if err := os.WriteFile(iconPath, iconData, 0644); err == nil {
		defer os.Remove(iconPath)
		
		// Load icon from file using LoadImageW
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
			return
		}
	}

	// Fallback to default application icon
	ret, _, _ := procLoadIconW.Call(0, IDI_APPLICATION)
	t.hIcon = ret
}

// addTrayIcon adds the tray icon
func (t *TrayRuntime) addTrayIcon() {
	if t.hwnd == 0 {
		return
	}

	nid := NOTIFYICONDATA{
		CbSize:           uint32(unsafe.Sizeof(NOTIFYICONDATA{})),
		HWnd:             t.hwnd,
		UID:              1,
		UFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP,
		UCallbackMessage: WM_TRAYICON,
		HIcon:            t.hIcon,
	}

	tip := "GoclashZ"
	copy(nid.SzTip[:], windows.StringToUTF16(tip))

	procShell_NotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&nid)))
}

// deleteTrayIcon removes the tray icon
func (t *TrayRuntime) deleteTrayIcon() {
	if t.hwnd == 0 {
		return
	}

	nid := NOTIFYICONDATA{
		CbSize: uint32(unsafe.Sizeof(NOTIFYICONDATA{})),
		HWnd:   t.hwnd,
		UID:    1,
	}

	procShell_NotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
}

// updateTrayTooltip updates the tray icon tooltip
func (t *TrayRuntime) updateTrayTooltip(tooltip string) {
	if t.hwnd == 0 {
		return
	}

	nid := NOTIFYICONDATA{
		CbSize: uint32(unsafe.Sizeof(NOTIFYICONDATA{})),
		HWnd:   t.hwnd,
		UID:    1,
		UFlags: NIF_TIP,
	}

	copy(nid.SzTip[:], windows.StringToUTF16(tooltip))

	procShell_NotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&nid)))
}

// wndProc is the window procedure for handling messages
func (t *TrayRuntime) wndProc(hwnd windows.HWND, msg uint32, wparam uintptr, lparam uintptr) uintptr {
	// Handle TaskbarCreated message (Explorer restart)
	if msg == t.taskbarCreatedMsg && t.taskbarCreatedMsg != 0 {
		t.addTrayIcon()
		state := t.snapshot()
		t.renderOnTrayThread(state)
		return 0
	}

	switch msg {
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
		state := t.snapshot()
		t.renderOnTrayThread(state)
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
	if !t.ready.Load() {
		return
	}

	t.checkedSysProxy = state.SystemProxy
	t.checkedTun = state.Tun
	t.mode = state.Mode

	tooltip := "GoclashZ"
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

	// Show main window
	t.appendMenu(hMenu, idShow, "显示界面", false)

	// Separator
	procAppendMenuW.Call(hMenu, MF_SEPARATOR, 0, 0)

	// System Proxy
	t.appendMenu(hMenu, idSysProxy, "系统代理", state.SystemProxy)

	// TUN Mode
	t.appendMenu(hMenu, idTun, "TUN 模式", state.Tun)

	// Separator
	procAppendMenuW.Call(hMenu, MF_SEPARATOR, 0, 0)

	// Outbound Mode submenu
	hModeMenu, _, _ := procCreatePopupMenu.Call()
	if hModeMenu != 0 {
		t.appendMenu(hModeMenu, idModeRule, "规则 (Rule)", state.Mode == "rule")
		t.appendMenu(hModeMenu, idModeGlobal, "全局 (Global)", state.Mode == "global")
		t.appendMenu(hModeMenu, idModeDirect, "直连 (Direct)", state.Mode == "direct")

		modeText, _ := windows.UTF16PtrFromString("出站模式")
		procAppendMenuW.Call(hMenu, MF_POPUP, hModeMenu, uintptr(unsafe.Pointer(modeText)))
	}

	// Separator
	procAppendMenuW.Call(hMenu, MF_SEPARATOR, 0, 0)

	// Restart core
	t.appendMenu(hMenu, idRestart, "重启内核", false)

	// Quit
	t.appendMenu(hMenu, idQuit, "退出程序", false)

	// Get cursor position
	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	// Set foreground window to ensure menu disappears when clicking elsewhere
	procSetForegroundWindow.Call(uintptr(t.hwnd))

	// Track popup menu
	ret, _, _ := procTrackPopupMenu.Call(
		hMenu,
		TPM_RIGHTBUTTON|TPM_RETURNCMD,
		uintptr(pt.X),
		uintptr(pt.Y),
		0,
		uintptr(t.hwnd),
		0,
	)

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

// handleCommand executes a tray command
func (t *TrayRuntime) handleCommand(cmd TrayCommand) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Tray command error: %v\n", r)
		}
	}()

	switch cmd {
	case TrayCmdShowMainWindow:
		t.app.safeShowMainWindow()

	case TrayCmdToggleSystemProxy:
		state := t.snapshot()
		_ = t.app.core.ToggleSystemProxy(t.ctx, !state.SystemProxy)

	case TrayCmdToggleTun:
		state := t.snapshot()
		_ = t.app.core.ToggleTunMode(t.ctx, !state.Tun)

	case TrayCmdModeRule:
		_ = t.app.core.UpdateClashMode(t.ctx, "rule")

	case TrayCmdModeGlobal:
		_ = t.app.core.UpdateClashMode(t.ctx, "global")

	case TrayCmdModeDirect:
		_ = t.app.core.UpdateClashMode(t.ctx, "direct")

	case TrayCmdRestartCore:
		_ = t.app.core.RestartCoreWithReason(t.ctx, "manual")

	case TrayCmdQuitApp:
		t.app.safeQuit()
	}
}
