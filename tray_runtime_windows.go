//go:build windows

package main

// 托盘实现（v3 原生化，第二轮 #10）
//
// 本文件用 Wails v3 的 application.SystemTray + application.Menu 替换了原先约 860 行的
// 自研 Win32 实现（RegisterClassExW 隐藏窗口 + Shell_NotifyIconW + TrackPopupMenu）。
// 对上层公开的 API 与语义保持 1:1：
//
//	NewTrayRuntime(app *App) *TrayRuntime
//	(*TrayRuntime).Start(ctx context.Context)
//	(*TrayRuntime).Stop()
//	(*TrayRuntime).UpdateState(state appcore.AppState)
//	TrayCommand 常量 + handleCommand 语义
//
// 行为对照（与 v2/v3.1.0 旧实现一致）：
//   - 右键：弹出菜单，顺序 = 显示 / 系统代理（端口代理模式下改为「端口代理」，勾选 DesiredPortProxy
//     或 DesiredSystemProxy）/ TUN（勾选 DesiredTun）/ 分隔 / 出站模式子菜单（rule|global|direct，
//     按当前 Mode 勾选）/ 分隔 / 重启内核 / 退出。
//   - 双击：显示主窗口（不是切换）；左键：无动作（v3 默认单击是 ToggleWindow，这里显式覆盖为空）。
//   - 多语言：沿用 getTrayTexts 的 8 套文案；工具提示 = "GoclashZ - 配置名/未选择配置"。
//   - 状态变化：菜单不重建（避免闪烁与句柄泄漏），只就地更新文案/勾选/工具提示，
//     并在主线程合并渲染（同一时刻最多排队一次），无变化时直接跳过。
//   - 图标：build/windows/icon.ico（iconData）；若 CreateSmallHIconFromImage 无法解析，
//     回退到 build/appicon.png；再失败则交给框架使用 exe 资源图标。

import (
	"context"
	_ "embed"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"goclashz/core/appcore"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"
	"golang.org/x/sys/windows"
)

// trayFallbackIconPNG 是 ICO 不可用时的兜底托盘图标（PNG，v3 CreateSmallHIconFromImage 支持）。
//
//go:embed build/appicon.png
var trayFallbackIconPNG []byte

// trayAppName 是工具提示前缀，与旧实现保持一致。
const trayAppName = "GoclashZ"

var (
	procGetDoubleClickTime = windows.NewLazySystemDLL("user32.dll").NewProc("GetDoubleClickTime")

	// lastLeftClickMS 记录上一次左键单击时间，用于双击兜底判定。
	lastLeftClickMS atomic.Int64
)

// systemDoubleClickTimeout 返回系统双击间隔（GetDoubleClickTime），失败时回退 500ms。
func systemDoubleClickTimeout() time.Duration {
	if err := procGetDoubleClickTime.Find(); err != nil {
		return 500 * time.Millisecond
	}
	if ms, _, _ := procGetDoubleClickTime.Call(); ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return 500 * time.Millisecond
}

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

// trayView 是托盘一次渲染所需的全部文案与勾选状态。
// 字段全部可比较，用来跳过无变化的重复渲染。
type trayView struct {
	show       string
	sysProxy   string
	tun        string
	modeRule   string
	modeGlobal string
	modeDirect string
	outbound   string
	restart    string
	quit       string
	tooltip    string

	sysProxyChecked bool
	tunChecked      bool
	mode            string
}

// TrayRuntime 是 v3 SystemTray 版本的托盘运行时。
type TrayRuntime struct {
	app  *App
	logf func(level string, format string, args ...any)

	ctx    context.Context
	cancel context.CancelFunc

	cmdCh chan TrayCommand

	started atomic.Bool
	stopped atomic.Bool

	latestMu sync.Mutex
	latest   appcore.AppState

	// 合并渲染：同一时刻最多排队一次主线程渲染。
	renderQueued atomic.Bool

	// tray 与以下菜单对象只在主线程创建/访问（Start 由 ServiceStartup 在主线程调用，
	// 渲染走 InvokeAsync，菜单回调也在主线程）。
	tray atomic.Pointer[application.SystemTray]
	menu *application.Menu

	itemShow       *application.MenuItem
	itemSysProxy   *application.MenuItem
	itemTun        *application.MenuItem
	itemOutbound   *application.MenuItem
	itemModeRule   *application.MenuItem
	itemModeGlobal *application.MenuItem
	itemModeDirect *application.MenuItem
	itemRestart    *application.MenuItem
	itemQuit       *application.MenuItem

	applied    trayView
	hasApplied bool
}

// NewTrayRuntime creates a new tray runtime
func NewTrayRuntime(app *App) *TrayRuntime {
	return &TrayRuntime{
		app:   app,
		cmdCh: make(chan TrayCommand, 10),
		logf:  app.logApp,
	}
}

// Start 创建系统托盘（v3 会在 App.Run 就绪后真正把图标挂到任务栏）。
func (t *TrayRuntime) Start(ctx context.Context) {
	if !t.started.CompareAndSwap(false, true) {
		return
	}

	t.ctx, t.cancel = context.WithCancel(ctx)

	wailsApp := application.Get()
	if wailsApp == nil || wailsApp.SystemTray == nil {
		t.logf("error", "[Tray] Wails 应用尚未创建，无法创建系统托盘")
		return
	}

	view := t.viewFor(t.snapshot())

	tray := wailsApp.SystemTray.New()
	t.tray.Store(tray)

	// 先装配菜单/图标/回调，再交给框架在 App.Run 后显示，避免启动期闪一下默认图标。
	t.buildMenu(view)
	if icon := t.resolveIcon(); len(icon) > 0 {
		tray.SetIcon(icon)
	}
	tray.SetTooltip(view.tooltip)

	// v3 默认单击是 ToggleWindow，这里显式覆盖，保持「左键无动作」。
	// 同时做双击兜底：v3 的 Windows 托盘按 NOTIFYICON_VERSION_4 注册，
	// WM_LBUTTONDBLCLK 是否上报取决于系统版本；用「系统双击间隔内连续两次单击」
	// 判定双击，确保「双击 = 显示主窗口、单击 = 无动作」与 v2 一致。
	tray.OnClick(func() {
		now := time.Now().UnixMilli()
		prev := lastLeftClickMS.Swap(now)
		if prev != 0 && now-prev <= systemDoubleClickTimeout().Milliseconds() {
			lastLeftClickMS.Store(0)
			t.PostCommand(TrayCmdShowMainWindow)
		}
	})
	// 右键弹出菜单（与旧实现的 WM_RBUTTONUP 一致）；打开前再同步一次缓存状态，
	// 等价旧实现「每次弹菜单都按最新状态重建」的效果。
	tray.OnRightClick(func() {
		t.applyView(t.viewFor(t.snapshot()))
		tray.ShowMenu()
	})
	// 双击显示主窗口（注意：不是 toggle），与旧实现的 WM_LBUTTONDBLCLK 一致。
	tray.OnDoubleClick(func() {
		t.PostCommand(TrayCmdShowMainWindow)
	})

	tray.SetMenu(t.menu)

	t.applied = view
	t.hasApplied = true

	go t.commandLoop()
}

// Stop 销毁托盘图标。应用退出时由 ServiceShutdown 调用（v3 的 destroy 不做主线程调度，
// 这里统一投递到主线程执行，保证从任意 goroutine 调用都安全）。
func (t *TrayRuntime) Stop() {
	if t.stopped.Swap(true) {
		return
	}
	if t.cancel != nil {
		t.cancel()
	}

	tray := t.tray.Swap(nil)
	if tray == nil {
		return
	}

	if application.Get() == nil {
		return
	}
	application.InvokeSync(func() {
		defer func() {
			if r := recover(); r != nil {
				t.logf("error", "[Tray] 销毁系统托盘异常: %v", r)
			}
		}()
		tray.Destroy()
	})
}

// UpdateState 接收最新 AppState 并驱动托盘文案/勾选/工具提示更新（非阻塞、可高频调用）。
func (t *TrayRuntime) UpdateState(state appcore.AppState) {
	t.latestMu.Lock()
	t.latest = state
	t.latestMu.Unlock()

	if !t.started.Load() || t.stopped.Load() || t.tray.Load() == nil {
		return
	}
	t.requestRender()
}

// PostCommand posts a command to be executed
func (t *TrayRuntime) PostCommand(cmd TrayCommand) {
	if t.stopped.Load() {
		return
	}
	// F-A5-20：退出 / 显示这类「终端动作」不能被一条耗时命令挡住。
	// commandLoop 是单 goroutine 串行执行，一次 TUN 开关会占住它 20~45s，
	// 期间队列（10 条）满后新命令会被静默丢弃——包括"退出程序"。
	// 这两类命令走旁路，保证「点了退出就一定退出、点了显示就一定弹窗」。
	if cmd == TrayCmdQuitApp || cmd == TrayCmdShowMainWindow {
		go t.handleCommand(cmd)
		return
	}
	select {
	case t.cmdCh <- cmd:
	default:
		// 丢弃必须留痕，否则用户连点没有任何反馈也查不到原因。
		t.logf("warn", "[Tray] 命令队列已满，丢弃命令 %d（正在执行耗时操作）", cmd)
	}
}

// snapshot returns the latest state
func (t *TrayRuntime) snapshot() appcore.AppState {
	t.latestMu.Lock()
	defer t.latestMu.Unlock()
	return t.latest
}

// viewFor 从 AppState 推导出托盘应显示的全部内容。
func (t *TrayRuntime) viewFor(state appcore.AppState) trayView {
	txt := getTrayTexts(state.Language)

	// 仅端口代理模式下，菜单项文字改为「端口代理」，勾选状态反映 DesiredPortProxy
	proxyLabel := txt.sysProxy
	proxyChecked := state.DesiredSystemProxy
	if state.PortProxyMode {
		proxyLabel = txt.portProxy
		proxyChecked = state.DesiredPortProxy
	}

	tooltip := trayAppName + " - " + txt.noConfig
	if state.ActiveConfigName != "" {
		tooltip = trayAppName + " - " + state.ActiveConfigName
	}

	return trayView{
		show:            txt.show,
		sysProxy:        proxyLabel,
		tun:             txt.tun,
		modeRule:        txt.modeRule,
		modeGlobal:      txt.modeGlobal,
		modeDirect:      txt.modeDirect,
		outbound:        txt.outbound,
		restart:         txt.restart,
		quit:            txt.quit,
		tooltip:         tooltip,
		sysProxyChecked: proxyChecked,
		tunChecked:      state.DesiredTun,
		mode:            state.Mode,
	}
}

// resolveIcon 返回可用的托盘图标字节。
func (t *TrayRuntime) resolveIcon() []byte {
	if len(iconData) > 0 {
		if h, err := w32.CreateSmallHIconFromImage(iconData); err == nil {
			if h != 0 {
				w32.DestroyIcon(h)
			}
			return iconData
		} else {
			t.logf("warn", "[Tray] ICO 托盘图标不可用，回退 PNG: %v", err)
		}
	}

	if len(trayFallbackIconPNG) > 0 {
		if h, err := w32.CreateSmallHIconFromImage(trayFallbackIconPNG); err == nil {
			if h != 0 {
				w32.DestroyIcon(h)
			}
			return trayFallbackIconPNG
		} else {
			t.logf("error", "[Tray] PNG 托盘图标同样不可用，使用 exe 资源图标: %v", err)
		}
	}

	return nil
}

// buildMenu 构建托盘菜单（只在 Start 时创建一次，后续就地更新）。
func (t *TrayRuntime) buildMenu(view trayView) {
	menu := application.NewMenu()

	t.itemShow = menu.Add(view.show)
	t.itemShow.OnClick(func(*application.Context) { t.PostCommand(TrayCmdShowMainWindow) })

	menu.AddSeparator()

	t.itemSysProxy = menu.AddCheckbox(view.sysProxy, view.sysProxyChecked)
	t.itemSysProxy.OnClick(func(*application.Context) { t.PostCommand(TrayCmdToggleSystemProxy) })

	t.itemTun = menu.AddCheckbox(view.tun, view.tunChecked)
	t.itemTun.OnClick(func(*application.Context) { t.PostCommand(TrayCmdToggleTun) })

	menu.AddSeparator()

	// 出站模式子菜单：AddSubmenu 只返回子菜单，这里用 NewSubMenuItem 以便后续改父项文案。
	t.itemOutbound = application.NewSubMenuItem(view.outbound)
	modeMenu := t.itemOutbound.GetSubmenu()

	t.itemModeRule = modeMenu.AddRadio(view.modeRule, view.mode == "rule")
	t.itemModeRule.OnClick(func(*application.Context) { t.PostCommand(TrayCmdModeRule) })

	t.itemModeGlobal = modeMenu.AddRadio(view.modeGlobal, view.mode == "global")
	t.itemModeGlobal.OnClick(func(*application.Context) { t.PostCommand(TrayCmdModeGlobal) })

	t.itemModeDirect = modeMenu.AddRadio(view.modeDirect, view.mode == "direct")
	t.itemModeDirect.OnClick(func(*application.Context) { t.PostCommand(TrayCmdModeDirect) })

	menu.Append(application.NewMenuFromItems(t.itemOutbound))

	menu.AddSeparator()

	t.itemRestart = menu.Add(view.restart)
	t.itemRestart.OnClick(func(*application.Context) { t.PostCommand(TrayCmdRestartCore) })

	t.itemQuit = menu.Add(view.quit)
	t.itemQuit.OnClick(func(*application.Context) { t.PostCommand(TrayCmdQuitApp) })

	t.menu = menu
}

// requestRender 合并高频状态变更：同一时刻最多一次主线程渲染排队。
func (t *TrayRuntime) requestRender() {
	if t.tray.Load() == nil {
		return
	}
	if application.Get() == nil {
		return
	}
	if !t.renderQueued.CompareAndSwap(false, true) {
		return
	}
	application.InvokeAsync(func() {
		t.renderQueued.Store(false)
		if t.stopped.Load() || t.tray.Load() == nil {
			return
		}
		t.applyView(t.viewFor(t.snapshot()))
	})
}

// applyView 把最新视图就地应用到托盘菜单与工具提示上。
// 只在主线程调用（Start 由 ServiceStartup 在主线程执行；其余走 InvokeAsync）。
func (t *TrayRuntime) applyView(view trayView) {
	tray := t.tray.Load()
	if tray == nil || t.itemShow == nil {
		return
	}

	if t.hasApplied && t.applied == view {
		return
	}
	prev := t.applied
	first := !t.hasApplied

	if first || prev.show != view.show {
		t.itemShow.SetLabel(view.show)
	}
	if first || prev.sysProxy != view.sysProxy {
		t.itemSysProxy.SetLabel(view.sysProxy)
	}
	if first || prev.tun != view.tun {
		t.itemTun.SetLabel(view.tun)
	}
	if first || prev.outbound != view.outbound {
		t.itemOutbound.SetLabel(view.outbound)
	}
	if first || prev.modeRule != view.modeRule {
		t.itemModeRule.SetLabel(view.modeRule)
	}
	if first || prev.modeGlobal != view.modeGlobal {
		t.itemModeGlobal.SetLabel(view.modeGlobal)
	}
	if first || prev.modeDirect != view.modeDirect {
		t.itemModeDirect.SetLabel(view.modeDirect)
	}
	if first || prev.restart != view.restart {
		t.itemRestart.SetLabel(view.restart)
	}
	if first || prev.quit != view.quit {
		t.itemQuit.SetLabel(view.quit)
	}

	if first || prev.sysProxyChecked != view.sysProxyChecked {
		t.itemSysProxy.SetChecked(view.sysProxyChecked)
	}
	if first || prev.tunChecked != view.tunChecked {
		t.itemTun.SetChecked(view.tunChecked)
	}
	if first || prev.mode != view.mode {
		t.itemModeRule.SetChecked(view.mode == "rule")
		t.itemModeGlobal.SetChecked(view.mode == "global")
		t.itemModeDirect.SetChecked(view.mode == "direct")
	}

	if first || prev.tooltip != view.tooltip {
		tray.SetTooltip(view.tooltip)
	}

	t.applied = view
	t.hasApplied = true
}

// commandLoop processes commands from the command channel
func (t *TrayRuntime) commandLoop() {
	for {
		select {
		case <-t.ctx.Done():
			return
		case cmd := <-t.cmdCh:
			t.handleCommand(cmd)

			// 勾选状态以真实状态为准：命令失败时撤销菜单上「看起来」已切换的勾选。
			// （旧实现每次弹菜单都从状态重建，等价效果。）
			if cmd != TrayCmdQuitApp && cmd != TrayCmdShowMainWindow {
				t.resyncFromCore()
			}
		}
	}
}

// resyncFromCore 回读真实状态并重新渲染托盘（不阻塞命令处理之外的其他路径）。
func (t *TrayRuntime) resyncFromCore() {
	if t.app == nil || t.app.core == nil {
		return
	}
	state := t.app.core.GetAppState()
	t.latestMu.Lock()
	t.latest = state
	t.latestMu.Unlock()
	t.requestRender()
}

// reportTrayError reports tray command errors to the UI
func (t *TrayRuntime) reportTrayError(action string, err error) {
	if err == nil {
		return
	}
	t.logf("error", "[Tray] %s failed: %v", action, err)
	message := fmt.Sprintf("%s失败: %v", action, err)
	t.app.core.GetEvents().Emit(appcore.EventNotification, appcore.Notification{
		Level:   "error",
		Code:    "TRAY_COMMAND_FAILED",
		Message: message,
		Source:  "tray",
	})
	t.app.core.GetEvents().Emit(appcore.EventNotifyError, message)
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
		err := t.app.core.ToggleSystemProxyFromCurrentDesired(t.ctx)
		t.reportTrayError("切换系统代理", err)

	case TrayCmdToggleTun:
		err := t.app.core.ToggleTunFromCurrentDesired(t.ctx)
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

type trayTexts struct {
	show       string
	portProxy  string
	sysProxy   string
	tun        string
	modeRule   string
	modeGlobal string
	modeDirect string
	outbound   string
	restart    string
	quit       string
	noConfig   string
}

func getTrayTexts(lang string) trayTexts {
	switch lang {
	case "zh-TW":
		return trayTexts{
			show:       "顯示介面",
			portProxy:  "連接埠代理",
			sysProxy:   "系統代理",
			tun:        "TUN 模式",
			modeRule:   "規則 (Rule)",
			modeGlobal: "全域 (Global)",
			modeDirect: "直連 (Direct)",
			outbound:   "出站模式",
			restart:    "重啟核心",
			quit:       "結束程式",
			noConfig:   "未選擇設定檔",
		}
	case "zh-HK":
		return trayTexts{
			show:       "顯示介面",
			portProxy:  "端口代理",
			sysProxy:   "系統代理",
			tun:        "TUN 模式",
			modeRule:   "規則 (Rule)",
			modeGlobal: "全局 (Global)",
			modeDirect: "直連 (Direct)",
			outbound:   "出站模式",
			restart:    "重啟核心",
			quit:       "退出程式",
			noConfig:   "未選擇配置文件",
		}
	case "en-US":
		return trayTexts{
			show:       "Show Window",
			portProxy:  "Port Proxy",
			sysProxy:   "System Proxy",
			tun:        "TUN Mode",
			modeRule:   "Rule",
			modeGlobal: "Global",
			modeDirect: "Direct",
			outbound:   "Outbound Mode",
			restart:    "Restart Core",
			quit:       "Quit",
			noConfig:   "No Profile Selected",
		}
	case "ja-JP":
		return trayTexts{
			show:       "ウィンドウを表示",
			portProxy:  "ポートプロキシ",
			sysProxy:   "システムプロキシ",
			tun:        "TUN モード",
			modeRule:   "ルール (Rule)",
			modeGlobal: "グローバル (Global)",
			modeDirect: "ダイレクト (Direct)",
			outbound:   "ルーティングモード",
			restart:    "コア再起動",
			quit:       "終了",
			noConfig:   "プロファイル未選択",
		}
	case "ru-RU":
		return trayTexts{
			show:       "Показать окно",
			portProxy:  "Прокси порта",
			sysProxy:   "Системный прокси",
			tun:        "Режим TUN",
			modeRule:   "По правилам (Rule)",
			modeGlobal: "Глобальный (Global)",
			modeDirect: "Прямой (Direct)",
			outbound:   "Режим маршрутизации",
			restart:    "Перезагрузить ядро",
			quit:       "Выход",
			noConfig:   "Профиль не выбран",
		}
	case "fr-FR":
		return trayTexts{
			show:       "Afficher la fenêtre",
			portProxy:  "Proxy de port",
			sysProxy:   "Proxy système",
			tun:        "Mode TUN",
			modeRule:   "Règle (Rule)",
			modeGlobal: "Global",
			modeDirect: "Direct",
			outbound:   "Mode de sortie",
			restart:    "Redémarrer le noyau",
			quit:       "Quitter",
			noConfig:   "Aucun profil sélectionné",
		}
	case "de-DE":
		return trayTexts{
			show:       "Fenster anzeigen",
			portProxy:  "Port-Proxy",
			sysProxy:   "System-Proxy",
			tun:        "TUN-Modus",
			modeRule:   "Regel (Rule)",
			modeGlobal: "Global",
			modeDirect: "Direkt",
			outbound:   "Ausgangsmodus",
			restart:    "Kernel neu starten",
			quit:       "Beenden",
			noConfig:   "Kein Profil ausgewählt",
		}
	default:
		return trayTexts{
			show:       "显示界面",
			portProxy:  "端口代理",
			sysProxy:   "系统代理",
			tun:        "TUN 模式",
			modeRule:   "规则 (Rule)",
			modeGlobal: "全局 (Global)",
			modeDirect: "直连 (Direct)",
			outbound:   "出站模式",
			restart:    "重启内核",
			quit:       "退出程序",
			noConfig:   "未选择配置",
		}
	}
}
