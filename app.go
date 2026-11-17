//go:build windows

package main

import (
	"context"
	_ "embed"
	jsonv2 "encoding/json/v2"
	"fmt"
	"goclashz/core/appcore"
	"goclashz/core/clash"
	"goclashz/core/logger"
	"goclashz/core/runtimeassets"
	"goclashz/core/sys"
	"goclashz/core/utils"
	"os"
	"path/filepath"
	"strings"
	"time"
	"uuid"

	"goclashz/core/version"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"
	"gopkg.in/yaml.v3"
)

//go:embed build/windows/icon.ico
var iconData []byte

type FileInfo struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

type App struct {
	ctx context.Context

	// win 是 v3 主窗口（Name = "main"）的强引用，由 main 在 NewWithOptions 之后赋值。
	// 它同时替代了两种旧实现：
	//   1) 每次调用 application.Get().Window.GetByName("main") 的字符串查找；
	//   2) 内部维护的 windowVisible 布尔状态（现在一律以窗口真实状态 IsVisible 为准）。
	// application.Run() 之前、或窗口被销毁之后为 nil，所有访问都必须 nil 安全。
	win *application.WebviewWindow

	// silentLaunch 由 main 在创建主窗口前算好（口径与 v2 一致：--silent || behavior.SilentStart，
	// --startup 不参与窗口可见性），窗口创建选项 Hidden 与运行时就绪后的
	// WindowRuntimeReady 兜底隐藏都使用同一个值。
	silentLaunch bool

	// 全局热键线程 ID (Ctrl+Alt+Q 退出)
	hotkeyTID atomic.Uint32

	tray *TrayRuntime
	core *appcore.Controller

	// 资产自愈的异步进度（RepairRuntimeAssetsAsync / GetRuntimeAssetRepairProgress）。
	// 同步版 RepairRuntimeAssets 仍保留（兼容既有绑定），但 UI 应改用异步版。
	assetRepairMu       sync.Mutex
	assetRepairProgress AssetRepairProgress
}

// mainWindow 返回 v3 主窗口引用；返回 nil 表示窗口尚未创建或已销毁，调用方需容忍。
func (a *App) mainWindow() *application.WebviewWindow {
	if a == nil {
		return nil
	}
	return a.win
}

// isWindowVisible 直接查询窗口的真实可见状态（v3 Windows 下为 WS_VISIBLE，
// 最小化仍算可见，与 v2 的语义一致）。窗口不存在时返回 false。
func (a *App) isWindowVisible() bool {
	w := a.mainWindow()
	return w != nil && w.IsVisible()
}

// emitEvent 是 WailsEventSink 之外的低层事件出口，等价 v2 的 runtime.EventsEmit。
func emitEvent(name string, data ...any) bool {
	app := application.Get()
	if app == nil {
		return false
	}
	return app.Event.Emit(name, data...)
}

// quitApplication 等价 v2 的 runtime.Quit(ctx)。
func quitApplication() {
	if app := application.Get(); app != nil {
		app.Quit()
		return
	}
	os.Exit(0)
}

// dialogCancelled 判断 v3 文件对话框返回的错误是否为「用户取消」。
// v3 取消时返回 internal/go-common-file-dialog/cfd.ErrorCancelled（文案 "cancelled by user"，
// 该 internal 包无法从本模块 import），而 v2 取消时返回 nil error。
// 这里按错误文案归一化，保证对外语义与 v2 一致（取消不影响 UI）。
func dialogCancelled(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "cancelled by user")
}

func (a *App) ShowMainWindow() {
	if a.ctx == nil {
		return
	}

	// v2: runtime.WindowShow + runtime.WindowUnmaximise
	if w := a.mainWindow(); w != nil {
		w.Show()
		w.UnMaximise()
	}

	// Windows 下从托盘恢复时，有时需要主动拉前台并闪烁提醒
	sys.FocusMainWindowAndFlashTwiceWin32Only()
}

func (a *App) HideMainWindow() {
	if a.ctx == nil {
		return
	}

	// v2: runtime.WindowHide
	if w := a.mainWindow(); w != nil {
		w.Hide()
	}
}

func (a *App) ToggleMainWindow() {
	if a.isWindowVisible() {
		a.HideMainWindow()
		return
	}

	a.ShowMainWindow()
}

func (a *App) logApp(level string, format string, args ...any) {
	if a == nil || a.core == nil {
		logger.AppLogs.Add(logger.LogEntry{
			Type:    logger.NormalizeLevel(level),
			Source:  logger.LogSourceApp,
			Payload: fmt.Sprintf(format, args...),
			Time:    time.Now().Format("15:04:05"),
		})
		return
	}

	a.core.LogApp(level, format, args...)
}

func NewApp() *App {
	appcore.MigrateLegacyRootSettings()

	app := &App{}
	app.core = appcore.NewController(appcore.Options{
		Events:        &WailsEventSink{},
		Version:       version.AppVersion,
		OnStateChange: app.syncTrayState,
	})
	app.tray = NewTrayRuntime(app)
	return app
}

func (a *App) runStartupRuntimeAssetMaintenance(ctx context.Context) {
	// 迁移保留同步：它是"就近采纳旧布局资产"的前置，且 MigrateLegacyAssets 内部已改为
	// 按需哈希（没有旧布局文件时 0 次 SHA256）。
	runtimeassets.MigrateLegacyAssets()

	// 深检 + 自愈放后台：避免窗口出现前被 95MB 读盘/`clash.exe -v` 阻塞（F-A0-06 收尾）。
	// 内核真正启动前的门禁在 controller 里另有 EnsureReady，不会因为这里后台化而漏检。
	go a.maintainRuntimeAssetsAsync(ctx)
}

func (a *App) maintainRuntimeAssetsAsync(ctx context.Context) {
	// 第一阶段：必须修好 core + wintun
	coreStatus, coreErr := runtimeassets.EnsureReady(ctx, runtimeassets.RequireTun, runtimeassets.RepairInvalid)
	if coreErr != nil {
		logger.Errorf("核心运行组件自修复失败: %v", coreErr)
		for _, asset := range coreStatus.Assets {
			if !asset.Ready && asset.Error != "" {
				logger.Errorf("组件不可用: %s path=%s error=%s", asset.Key, asset.Path, asset.Error)
			}
		}
	}

	// 第二阶段：尝试修 geo，失败不影响应用启动
	geoStatus, geoErr := runtimeassets.EnsureReady(ctx, runtimeassets.RequireGeoOnly, runtimeassets.RepairMissingOnly)
	if geoErr != nil {
		logger.Warnf("Geo 数据组件自修复失败，可稍后由在线更新补齐: %v", geoErr)
	}

	status := runtimeassets.MergeStatus(coreStatus, geoStatus)

	emitEvent("runtime-assets-status", status)

	go runtimeassets.CleanupLegacyAssets()
}

// ServiceStartup 取代 v2 的 options.App.OnStartup，启动顺序与 v2 完全一致。
// 注意：v3 在窗口真正创建/显示之前调用它，因此这里绝不能依赖 Win32 窗口句柄，
// 也不再调用 WindowShow/WindowHide（窗口可见性由 main 里的 Hidden 选项决定）。
func (a *App) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	cleanupAppliedUpdatePackage()

	a.ctx = ctx
	if sink, ok := a.core.GetEvents().(*WailsEventSink); ok {
		sink.SetContext(ctx)
	}

	a.runStartupRuntimeAssetMaintenance(ctx)

	// 必须先加载订阅索引，再启动 appcore 的自动任务。
	clash.LoadIndex()

	// 执行规则存储从 V1 (_rules.json) 到 V2 (yaml/overlay) 的全量迁移
	_ = clash.MigrateRuleStorageV2()

	config := a.core.Behavior.Get()
	bootstrapSilent := hasFlag("--silent") || config.SilentStart

	a.core.Bootstrap(ctx, appcore.BootstrapOptions{
		IsStartupLaunch: hasFlag("--startup"),
		Silent:          bootstrapSilent,
	})

	// Best-effort: if the helper service was left running from a previous
	// session, register this process so its watchdog tracks the new PID.
	// Errors are intentionally ignored — helper may not be installed.
	go func() {
		client := sys.NewHelperClient()
		for i := 0; i < 5; i++ {
			if err := client.RegisterParent(os.Getpid()); err == nil {
				break
			}
			time.Sleep(1 * time.Second)
		}
	}()

	// #4：窗口可见性完全由 main 决定 —— WebviewWindowOptions.Hidden = isSilent
	// （创建期就不带 WS_VISIBLE，从根上避免首帧白闪），运行时就绪后由 main 注册的
	// WindowRuntimeReady 回调再 Hide() 一次兜底。原先这里的
	// sys.SafeHideMainWindowWin32 / sys.EnsureMainWindowHiddenSilently 已随之删除
	// （core/sys 中的这两个函数本身保留不动，不影响其它潜在调用方）。

	a.SyncState()

	go a.startHotkeyWorker(ctx)
	a.tray.Start(ctx)

	return nil
}

// ServiceShutdown 由 v3 的 service shutdown 阶段调用（取代 v2 options.App.OnShutdown，
// 以及首轮迁移时临时挂在 application.Options.OnShutdown 上的写法）。
// 退出顺序与 v2 完全一致：tray.Stop → helper Shutdown → core StopCoreService/StopTrafficStream（5s 超时）。
// 注意：v3 在 cleanup() 的主线程 InvokeSync 内调用本方法，因此这里不能做长时间阻塞操作。
func (a *App) ServiceShutdown() error {
	if a.tray != nil {
		a.tray.Stop()
	}

	// 通知 Helper 服务自行退出（通过 Named Pipe，不需要管理员权限）
	go func() {
		client := sys.NewHelperClient()
		_ = client.Shutdown()
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		a.core.StopCoreService()
		a.core.StopTrafficStream()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		logger.Warnf("停止内核/流量流超时，继续退出")
	}

	return nil
}

// --- AppState & Sync ---

type AppState = appcore.AppState

func (a *App) GetAppState() AppState {
	return a.core.GetAppState()
}

func (a *App) SyncState() {
	// 内部会发送 app-state-sync、自动启停 traffic stream，以及通过 OnStateChange 同步托盘
	a.core.SyncState()
}

func (a *App) syncTrayState() {
	if a.ctx == nil || a.tray == nil {
		return
	}
	state := a.core.GetAppState()
	a.tray.UpdateState(state)
}

func (a *App) GetInitialData() (map[string]interface{}, error) {
	state := a.core.GetAppState()
	var data map[string]interface{}

	if !state.IsRunning {
		offlineData, err := clash.GetOfflineData(state.ActiveConfig)
		if err != nil {
			data = map[string]interface{}{
				"mode":         state.Mode,
				"groups":       make(map[string]interface{}),
				"activeConfig": state.ActiveConfig,
				"isOffline":    true,
			}
		} else {
			appcore.MergeOfflineSelection(offlineData, a.core.Offline.Snapshot(state.ActiveConfig))
			offlineData["activeConfig"] = state.ActiveConfig
			offlineData["mode"] = state.Mode
			offlineData["isOffline"] = true
			data = offlineData
		}
	} else {
		apiData, err := clash.GetInitialData()
		if err != nil {
			data = map[string]interface{}{
				"mode":         state.Mode,
				"groups":       make(map[string]interface{}),
				"activeConfig": state.ActiveConfig,
				"isOffline":    true,
			}
		} else {
			apiData["activeConfig"] = state.ActiveConfig
			apiData["mode"] = state.Mode
			apiData["isOffline"] = false
			data = apiData
		}
	}

	data["activeConfigName"] = state.ActiveConfigName
	data["activeConfigType"] = state.ActiveConfigType

	configPath := filepath.Join(utils.GetSubscriptionsDir(), state.ActiveConfig+".yaml")
	if state.ActiveConfig == "" || state.ActiveConfig == "config.yaml" {
		configPath = clash.GetConfigPath()
	}
	if yamlData, err := os.ReadFile(configPath); err == nil {
		data["groupOrder"] = clash.ExtractGroupOrder(yamlData)
	}

	return data, nil
}

func (a *App) GetConnections() (appcore.ConnectionsSnapshot, error) {
	return a.core.GetConnections()
}

func (a *App) StartConnectionMonitor() {
	a.core.StartConnectionMonitor(a.ctx)
}

func (a *App) StopConnectionMonitor() {
	a.core.StopConnectionMonitor()
}

// --- Toggles & Controls ---

func (a *App) ToggleSystemProxy(enable bool) error {
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return a.core.ToggleSystemProxy(ctx, enable)
}

func (a *App) ToggleTunMode(enable bool) error {
	ctx, cancel := context.WithTimeout(a.ctx, 25*time.Second)
	defer cancel()
	return a.core.ToggleTunMode(ctx, enable)
}

func (a *App) UpdateClashMode(mode string) error {
	return a.core.UpdateClashMode(a.ctx, mode)
}

func (a *App) GetDiagnosticInfo() appcore.DiagnosticInfo {
	return a.core.GetDiagnosticInfo()
}

func (a *App) ExportDiagnostics() (string, error) {
	return a.core.ExportDiagnosticsWithPath()
}

func (a *App) RestartCore() error {
	return a.core.RestartCoreWithReason(a.ctx, "manual")
}

func (a *App) SelectProxy(groupName, proxyName string) error {
	state := a.core.GetAppState()
	profileID := state.ActiveConfig
	mode := state.Mode

	if clash.IsRunning() && state.IsRunning {
		ctx, cancel := context.WithTimeout(a.ctx, 3*time.Second)
		defer cancel()

		if err := a.core.SelectProxyWithModeSync(ctx, profileID, mode, groupName, proxyName); err != nil {
			return err
		}
		a.SyncState()
		return nil
	}

	a.core.SelectOfflineProxyWithModeSync(profileID, mode, groupName, proxyName)
	a.SyncState()
	return nil
}

func (a *App) FlushFakeIP() error {
	return clash.FlushFakeIP()
}

func (a *App) CloseAllConnections() error {
	return clash.CloseAllConnections()
}

func (a *App) CloseConnection(id string) error {
	return clash.CloseConnection(id)
}

// --- Subscriptions ---

func (a *App) GetLocalConfigs() []clash.SubIndexItem {
	return clash.ListSubIndex()
}

func (a *App) UpdateSub(name, url string) error {
	return a.core.UpdateSub(a.ctx, name, url)
}

func (a *App) UpdateSingleSub(id string) error {
	return a.core.UpdateSingleSub(a.ctx, id)
}

func (a *App) UpdateAllSubsAsync() {
	a.core.UpdateAllSubsAsync(a.ctx)
}

// --- Traffic & Logs ---

func (a *App) StartStreamingLogs() {
	a.core.StartLogStream(a.ctx)
}

func (a *App) GetRecentLogs() []appcore.LogEntry {
	return a.core.GetRecentLogs()
}

func (a *App) SearchLogs(keyword string) []appcore.LogEntry {
	return a.core.SearchLogs(keyword)
}

func (a *App) ClearLogs() {
	a.core.ClearLogs()
}

func (a *App) ResetTrafficTotals() {
	a.core.ResetTrafficTotals()
}

// --- Behavior & Theme ---

type AppBehavior = appcore.AppBehavior
type CoreSeedCheckResult = appcore.CoreSeedCheckResult

func (a *App) GetAppBehavior() AppBehavior {
	return a.core.Behavior.Get()
}

// windowBaseColour 返回窗口不透明底色（dark #111111 / light #F2F2F2）。
// 与 main.go 创建窗口时的默认底色、前端 --app-bg 保持同一套值。
func windowBaseColour(isDark bool) (uint8, uint8, uint8) {
	if isDark {
		return 17, 17, 17
	}
	return 242, 242, 242
}

// resolveBehaviorIsDark 把 themeMode 解析成「当前实际明暗」。
// light/dark 直接取用户偏好；system 交给 Windows 当前明暗——theme_setting.txt
// 是前端解析后的快照，可能滞后，因此不作为 system 的依据。
func resolveBehaviorIsDark(behavior AppBehavior) bool {
	// 单一实现：与 GetAppState().Theme 用同一套判定，避免出现
	// 「窗口按暗色画、界面报浅色」这类自相矛盾。实现见 appcore.ResolveThemeName。
	return appcore.ResolveThemeName(behavior) == "dark"
}

// applyWindowAppearance 按「玻璃开关 + 明暗」设置主窗口底色。
//
// ⚠️ 事实前提（wails v3.1.0 源码）：窗口以 application.BackgroundTypeTranslucent
// 创建时，windowsWebviewWindow.setBackgroundColour 无条件把 WebView 背景设成
// (0,0,0,0)，传入的颜色被完全忽略（webview_window_windows.go:455-462）。
// 因此本函数在当前窗口配置下是**空操作**，不会改变任何渲染结果；它的价值是语义
// 正确、以及未来把窗口改成 BackgroundTypeSolid 时能直接生效。
// 玻璃观感由 CSS 层（utils/glass.ts、styles/surfaces.css）决定，排查玻璃问题不要
// 从这里切入，也不要按"不透明底色盖死 Acrylic"这类错误结论去找后端。
//
// 约定：
//   - 开启玻璃：底色 alpha=0，交给 DWM Acrylic 合成；
//   - 关闭玻璃：恢复不透明主题底色（dark #111111 / light #F2F2F2）。
//
// 窗口可能在 application.Run() 之前或已销毁时为 nil（单测/无窗口场景），必须判空。
func (a *App) applyWindowAppearance(behavior AppBehavior, isDark bool) {
	w := a.mainWindow()
	if w == nil {
		return
	}

	r, g, b := windowBaseColour(isDark)
	if behavior.GlassBlurEnabled {
		// 开启玻璃：alpha=0，把底色交给 DWM 合成。
		// 注意当前 translucent 窗口下 wails 会忽略传入颜色（见上方事实前提），
		// 这里是语义表达 + 未来切 BackgroundTypeSolid 时生效。
		w.SetBackgroundColour(application.NewRGBA(r, g, b, 0))
		return
	}
	w.SetBackgroundColour(application.NewRGB(r, g, b))
}

func (a *App) SaveAppBehavior(config AppBehavior) error {
	if err := a.core.SaveAppBehavior(config); err != nil {
		return err
	}

	// 以落盘后的真实配置为准：normalizeBehavior 可能把非法值回填成默认值，
	// 窗口外观必须按最终生效的值设置，不能按前端传来的原始值。
	applied := a.core.Behavior.Get()
	isDark := resolveBehaviorIsDark(applied)

	if w := a.mainWindow(); w != nil {
		// strength 用背景着色语义（GlassTintStrength，0..80）；
		// 旧字段 GlassBlurStrength 只保留做迁移，不再驱动原生 tint。
		sys.ApplyWindowGlass(uintptr(w.NativeWindow()), applied.GlassBlurEnabled, applied.GlassTintStrength)
	}

	// 必须同步重设窗口底色：只调 ApplyWindowGlass 会让玻璃开/关后底色仍是旧值。
	a.applyWindowAppearance(applied, isDark)
	return nil
}

func (a *App) ResetComponentSettings(section string) error {
	switch section {
	case "behavior":
		old := a.core.Behavior.Get()
		cfg := a.core.Behavior.Default()
		cfg.ActiveConfig = old.ActiveConfig
		cfg.ActiveMode = old.ActiveMode

		if err := a.SaveAppBehavior(cfg); err != nil {
			return err
		}

		a.SyncState()
		return nil

	case "network":
		return a.core.ResetNetworkConfig(a.ctx)

	case "dns":
		return a.core.ResetDNSConfig(a.ctx)

	case "tun":
		return a.core.ResetTunConfig(a.ctx)

	default:
		return fmt.Errorf("unknown settings section: %s", section)
	}
}

func (a *App) SaveThemePreference(isDark bool) {
	theme := "light"
	if isDark {
		theme = "dark"
	}
	_ = utils.SaveGlobalTheme(theme)

	// v3 不再提供 runtime.WindowSetDarkTheme/WindowSetLightTheme：
	// 这里用原生 HWND 调 w32.SetTheme 复刻 v2 的标题栏/边框配色切换，
	// 并同步窗口背景色，避免 WebView 重绘瞬间露出旧底色。
	if w := a.mainWindow(); w != nil {
		if hwnd := w.NativeWindow(); hwnd != nil {
			w32.SetTheme(uintptr(hwnd), isDark)
		}
		// 走统一入口：玻璃开启时 alpha 必须保持 0，这里不能再无条件写成不透明，
		// 否则切明暗会把 Acrylic 盖死（与 SaveAppBehavior 保持同一套逻辑）。
		a.applyWindowAppearance(a.core.Behavior.Get(), isDark)
	}

	// 这里刻意不调 SyncState()。
	// 这个函数是跟随系统模式下「OS 明暗变化」的落盘出口，调用方（前端 watchTheme）
	// 此刻已经知道新值；再广播一次全量状态，会让 GetAppState().Theme 立刻回灌前端，
	// 而 Behavior.ThemeMode 走到 SaveAppBehavior 才落盘（异步），中间那段读到的
	// 是旧值 —— 表现就是明暗开关被顶回去、来回翻。
	// 需要广播的场景由 SaveAppBehavior 自己负责（它末尾会 SyncState）。
}

// --- System Tools ---

func (a *App) GetStartupTaskInfo() (sys.StartupTaskInfo, error) {
	return sys.CheckStartupTask()
}

func (a *App) RepairStartupTask() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	return sys.CreateStartupTask(exePath)
}

// --- Helper Service Management ---

// GetHelperServiceStatus 返回后台服务状态
func (a *App) GetHelperServiceStatus() sys.HelperStatusData {
	return sys.CheckHelperService()
}

// InstallHelperService 安装后台服务（需要 UAC 提权）
func (a *App) InstallHelperService() error {
	exePath := filepath.Join(utils.GetAppDir(), "GoclashZHelper.exe")
	if _, err := os.Stat(exePath); err != nil {
		return fmt.Errorf("GoclashZHelper.exe 不存在于 %s，请先部署 helper 程序", exePath)
	}

	sid, err := sys.CurrentUserSID()
	if err != nil {
		return fmt.Errorf("获取当前用户 SID 失败: %w", err)
	}

	if !sys.CheckAdmin() {
		if err := sys.RunElevatedWithArgsWait("--install-helper", "--allowed-sid", sid); err != nil {
			return fmt.Errorf("安装后台服务失败: %w", err)
		}
		return sys.WaitForHelperReady(15, 500*time.Millisecond)
	}

	return sys.RecoverHelperServiceForUser(exePath, sid)
}

// UninstallHelperService 卸载后台服务（需要 UAC 提权）
func (a *App) UninstallHelperService() error {
	if !sys.CheckAdmin() {
		return sys.RunElevatedWithArgsWait("--uninstall-helper")
	}
	return sys.UninstallHelperService()
}

// RestartHelperService 重启后台服务（需要 UAC 提权）
func (a *App) RestartHelperService() error {
	if !sys.CheckAdmin() {
		return sys.RunElevatedWithArgsWait("--restart-helper")
	}
	if err := sys.StopHelperService(); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	return sys.StartHelperService()
}

func (a *App) GetDataDirInfo() sys.DataDirInfo {
	info := sys.GetDataDirInfo()

	// 用 runtimeassets 唯一权威状态覆盖 stat 判断
	status := runtimeassets.GetStatusCached(context.Background())
	if core, ok := status.Assets[runtimeassets.AssetCore]; ok {
		info.CoreExists = core.Exists
		info.CoreReady = core.Ready
		info.CoreError = core.Error
	}
	if wintun, ok := status.Assets[runtimeassets.AssetWintun]; ok {
		info.WintunExists = wintun.Exists
		info.WintunReady = wintun.Ready
		info.WintunError = wintun.Error
	}
	info.LayoutOK = info.CoreExists

	// seed 目录真实文件状态
	// 种子库快照走快检 + manifest 元信息：不再对 seed 的 clash.exe 跑 -v（单次最多 5s）
	seedStatus := runtimeassets.GetSeedCatalogStatusFast(context.Background())
	info.SeedManifestOK = seedStatus.ManifestOK
	info.SeedManifestError = seedStatus.ManifestError
	for _, sa := range seedStatus.Assets {
		switch sa.Key {
		case runtimeassets.AssetCore:
			info.SeedCoreReady = sa.Ready
		case runtimeassets.AssetWintun:
			info.SeedWintunReady = sa.Ready
		case runtimeassets.AssetGeoIP:
			info.SeedGeoIPReady = sa.Ready
		case runtimeassets.AssetGeoSite:
			info.SeedGeoSiteReady = sa.Ready
		case runtimeassets.AssetMMDB:
			info.SeedMMDBReady = sa.Ready
		case runtimeassets.AssetASN:
			info.SeedASNReady = sa.Ready
		}
	}

	// 自动修复能力判断
	info.CanAutoRepairCore = info.SeedCoreReady && !info.CoreReady
	info.CanAutoRepairWintun = info.SeedWintunReady && !info.WintunReady

	return info
}

func (a *App) RepairDataDirMigration() error {
	return utils.ForceMigrateLegacyAppDataToInstallData()
}

func (a *App) RepairDataDirPermission() error {
	if !sys.CheckAdmin() {
		return sys.RunElevatedWithArgsWait("--repair-permissions")
	}
	return utils.RepairDataDirPermission()
}

func (a *App) RepairCoreLayout() error {
	runtimeassets.MigrateLegacyAssets()
	if !sys.CheckAdmin() {
		return sys.RunElevatedWithArgsWait("--repair-core-layout")
	}
	return utils.RepairCoreBinPermission()
}

func (a *App) CheckTunEnv() map[string]any {
	status := runtimeassets.GetStatusCached(a.ctx)
	helperStatus := sys.CheckHelperService()

	return map[string]any{
		"isAdmin":       sys.CheckAdmin(),
		"hasWintun":     status.Assets[runtimeassets.AssetWintun].Ready,
		"wintun":        status.Assets[runtimeassets.AssetWintun],
		"helperRunning": helperStatus.Reachable,
		"supportsMips":  clash.CoreSupportsMips(a.ctx),
	}
}

func (a *App) InstallTunDriverAsync(_ bool) {
	a.core.InstallTunDriverAsync(a.ctx)
}

func (a *App) RestartAsAdmin() error {
	return sys.RequestAdmin()
}

func (a *App) GetUwpApps() ([]sys.UwpApp, error) {
	return sys.GetUwpAppList()
}

func (a *App) SaveUwpExemptions(sids []string) error {
	if !sys.CheckAdmin() {
		// 普通权限模式下，借助 UAC 提权执行子进程以保存系统豁免配置
		tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("goclashz-uwp-%s.json", uuid.New().String()))
		data, err := jsonv2.Marshal(sids)
		if err != nil {
			return fmt.Errorf("编码 UWP 列表失败: %w", err)
		}
		if err := os.WriteFile(tempFile, data, 0600); err != nil {
			return fmt.Errorf("创建临时文件失败: %w", err)
		}
		defer os.Remove(tempFile)

		if err := sys.RunElevatedWithArgsWait("--save-uwp-exemptions", tempFile); err != nil {
			return fmt.Errorf("提权保存 UWP 规则失败: %w", err)
		}
		return nil
	}

	return sys.SaveUwpExemptions(sids)
}

// ApplyUwpExemptionChanges UWP 豁免差异应用：只动传入的两组 SID，不枚举、不推断其它豁免
func (a *App) ApplyUwpExemptionChanges(added []string, removed []string) error {
	return a.core.ApplyUwpExemptionChanges(added, removed)
}

// ClearAllUwpExemptions 清空全部 UWP 豁免：只删除本次枚举到的已豁免 SID
func (a *App) ClearAllUwpExemptions() error {
	return a.core.ClearAllUwpExemptions()
}

// --- Config Management ---

func (a *App) GetDNSConfig() (*clash.DNSConfig, error) {
	return clash.GetDNSConfig()
}

func (a *App) SaveDNSConfig(cfg *clash.DNSConfig) error {
	return a.core.SaveDNSConfig(a.ctx, cfg)
}

func (a *App) GetTunConfig() (*clash.TunConfig, error) {
	return clash.GetTunConfig()
}

func (a *App) SaveTunConfig(cfg *clash.TunConfig) error {
	return a.core.SaveTunConfig(a.ctx, cfg)
}

func (a *App) GetNetworkConfig() (*clash.NetworkConfig, error) {
	return clash.GetNetworkConfig()
}

func (a *App) SaveNetworkConfig(cfg *clash.NetworkConfig) error {
	return a.core.SaveNetworkConfig(a.ctx, cfg)
}

func (a *App) RenameConfig(id, newName string) error {
	return a.core.RenameConfig(id, newName)
}

func (a *App) DeleteConfig(id string) error {
	return a.core.DeleteConfig(a.ctx, id)
}

func (a *App) SelectLocalConfig(id string) error {
	return a.core.SelectLocalConfig(a.ctx, id)
}

func (a *App) SelectLocalFile() (FileInfo, error) {
	path, err := application.Get().Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title: "选择本地配置文件",
		Filters: []application.FileFilter{
			{DisplayName: "Clash 配置文件 (*.yaml; *.yml)", Pattern: "*.yaml;*.yml"},
		},
	}).PromptForSingleSelection()
	if err != nil {
		if dialogCancelled(err) {
			return FileInfo{}, nil // 取消：v2 语义为返回空值且无错误
		}
		return FileInfo{}, err
	}
	if path == "" {
		return FileInfo{}, nil
	}
	return FileInfo{
		Path: path,
		Name: filepath.Base(path),
	}, nil
}

func (a *App) DoLocalImport(srcPath, name string) (string, error) {
	return a.core.DoLocalImport(srcPath, name)
}

func (a *App) StartClash(id string) error {
	if err := a.core.SelectLocalConfig(a.ctx, id); err != nil {
		return err
	}
	if !a.core.GetAppState().IsRunning {
		return a.core.RestartCoreWithReason(a.ctx, "config-switch")
	}
	return nil
}

// --- Extra Utilities ---

// Deprecated: 等价于 GetRuntimeAssetStatus().Assets[core].Version（同一条缓存）。
// 保留是因为它是"只要一个版本串"的最省调用；新代码请优先用 GetRuntimeAssetStatus。
func (a *App) GetCoreVersion() string {
	status := runtimeassets.GetStatusCached(a.ctx)
	core := status.Assets[runtimeassets.AssetCore]
	if !core.Ready {
		return "未安装"
	}
	if core.Version != "" {
		return core.Version
	}
	return "已安装，版本未知"
}

func (a *App) GetRuntimeAssetStatus() runtimeassets.RuntimeAssetStatus {
	return runtimeassets.GetStatusCached(a.ctx)
}

// AssetRepairProgress 是异步修复的进度快照（UI 轮询 GetRuntimeAssetRepairProgress）。
type AssetRepairProgress struct {
	Running    bool   `json:"running"`
	Done       int    `json:"done"`
	Total      int    `json:"total"`
	Current    string `json:"current,omitempty"`
	Error      string `json:"error,omitempty"`
	StartedAt  int64  `json:"startedAt,omitempty"`
	FinishedAt int64  `json:"finishedAt,omitempty"`
}

// RepairRuntimeAssets 同步修复（保留既有语义与返回值，供老绑定/一次性调用使用）。
// 长时间回收 95 MB 资产时它会阻塞调用方，UI 请改用 RepairRuntimeAssetsAsync。
func (a *App) RepairRuntimeAssets() runtimeassets.RuntimeAssetStatus {
	status, err := runtimeassets.EnsureReady(a.ctx, runtimeassets.RequireAll, runtimeassets.RepairForce)
	if err != nil {
		a.core.LogApp("error", "修复运行组件失败: %v", err)
	}
	return status
}

// RepairRuntimeAssetsAsync 在后台重写全部运行组件，并用 RepairAssetProgress 汇报进度，
// 完成后推送已注册的 runtime-assets-status 事件（前端会据此刷新状态）。
// 重复调用是幂等的：已经在跑时直接返回当前进度（F-A0-04 / F-A1-17）。
func (a *App) RepairRuntimeAssetsAsync() AssetRepairProgress {
	a.assetRepairMu.Lock()
	if a.assetRepairProgress.Running {
		p := a.assetRepairProgress
		a.assetRepairMu.Unlock()
		return p
	}
	a.assetRepairProgress = AssetRepairProgress{Running: true, StartedAt: time.Now().UnixMilli()}
	progress := a.assetRepairProgress
	a.assetRepairMu.Unlock()

	go func() {
		// 资产重写 + 复制最多几分钟；给一个明确上限，避免后台任务永久挂住。
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		status, err := runtimeassets.EnsureReadyWithProgress(ctx, runtimeassets.RequireAll, runtimeassets.RepairForce,
			func(assetName string, done, total int) {
				a.assetRepairMu.Lock()
				a.assetRepairProgress.Current = assetName
				a.assetRepairProgress.Done = done
				a.assetRepairProgress.Total = total
				a.assetRepairMu.Unlock()
			})

		a.assetRepairMu.Lock()
		a.assetRepairProgress.Running = false
		a.assetRepairProgress.FinishedAt = time.Now().UnixMilli()
		if err != nil {
			a.assetRepairProgress.Error = err.Error()
		}
		a.assetRepairMu.Unlock()

		if err != nil {
			a.core.LogApp("error", "修复运行组件失败: %v", err)
		}
		emitEvent("runtime-assets-status", status)
	}()

	return progress
}

// GetRuntimeAssetRepairProgress 返回异步修复的当前进度（前端轮询，无需新增事件类型）。
func (a *App) GetRuntimeAssetRepairProgress() AssetRepairProgress {
	a.assetRepairMu.Lock()
	defer a.assetRepairMu.Unlock()
	return a.assetRepairProgress
}

func (a *App) GetProxyDelay(proxyName, testUrl string) (int, error) {
	return clash.GetProxyDelay(a.ctx, proxyName, testUrl, 8000)
}

// GetConfigDeviations 配置偏差：返回保存了但不会生效的设置项（module 为空表示全部）
func (a *App) GetConfigDeviations(module string) ([]clash.ConfigDeviation, error) {
	return a.core.GetConfigDeviations(module)
}

// ExplainDNS DNS 解析决策：该域名最终用哪个解析器（静态推演，不发真实查询）
func (a *App) ExplainDNS(domain string) (*clash.DNSExplain, error) {
	return a.core.ExplainDNS(domain)
}

// Deprecated: use new AddRule/DeleteRule/SaveRuleSection
func (a *App) GetCustomRules(id string) ([]string, error) {
	data, err := a.GetRulePageData(id)
	if err != nil {
		return nil, err
	}
	return data.EffectiveRules, nil
}

// Deprecated: use new AddRule/DeleteRule/SaveRuleSection
func (a *App) SaveCustomRules(id string, rules []string) error {
	item, _ := clash.FindSubIndexByID(id)
	if item.Type == "remote" {
		return fmt.Errorf("旧规则保存接口已废弃，远程配置请使用 AddRule/DeleteRule/SaveRuleSection")
	}
	return a.SaveRuleSection(id, "local", rules)
}

// Deprecated: no longer needed in V2 rule engine
func (a *App) SyncRules(id string) error {
	return fmt.Errorf("规则同步功能已废弃")
}

// --- Rule V2 API ---

func (a *App) GetRulePageData(id string) (clash.RulePageData, error) {
	var res clash.RulePageData
	err := clash.WithRuleStorageLock(func() error {
		if err := clash.EnsureRuleStorageMigratedLocked(id); err != nil {
			return fmt.Errorf("规则存储迁移失败: %w", err)
		}

		workingRoot, err := clash.ReadWorkingRootWithRecovery(id)
		if err != nil {
			return err
		}

		item, _ := clash.FindSubIndexByID(id)

		if item.Type != "remote" {
			res.ConfigType = "local"
			res.LocalRules = clash.ExtractRulesFromRootPublic(workingRoot)
			res.EffectiveRules = res.LocalRules
			return nil
		}

		// Remote
		overlay, err := clash.LoadRuleOverlay(id)
		if err != nil {
			return err
		}

		workingRules := clash.ExtractRulesFromRootPublic(workingRoot)
		visibleBaseRules := clash.ApplyDeleteOnly(workingRules, overlay.Delete)
		effectiveRules := clash.ApplyRuleOverlay(workingRules, overlay)

		res.ConfigType = "remote"
		res.SubscriptionRules = visibleBaseRules
		res.AddRules = overlay.Add
		res.DeleteRules = overlay.Delete
		res.EffectiveRules = effectiveRules

		return nil
	})
	return res, err
}

func (a *App) AddRule(id string, section string, ruleStr string) error {
	return clash.WithRuleStorageLock(func() error {
		_ = clash.EnsureRuleStorageMigratedLocked(id)

		ruleStr, err := clash.SanitizeRuleLine(ruleStr)
		if err != nil {
			return err
		}

		if section == "local" || section == "subscription" {
			workingPath, _ := clash.WorkingConfigPath(id)
			root, err := clash.ReadWorkingRootWithRecovery(id)
			if err != nil {
				return err
			}

			rules := clash.ExtractRulesFromRootPublic(root)
			rules = append([]string{ruleStr}, rules...)
			root["rules"] = rules

			out, _ := yaml.Marshal(root)
			if err := utils.WriteFileAtomic(workingPath, out, 0644); err != nil {
				return err
			}
			return nil
		}

		if section == "add" || section == "delete" {
			overlay, err := clash.LoadRuleOverlay(id)
			if err != nil {
				return err
			}
			if section == "add" {
				overlay.Add = append([]string{ruleStr}, overlay.Add...)
			} else {
				overlay.Delete = append([]string{ruleStr}, overlay.Delete...)
			}
			return clash.SaveRuleOverlay(id, overlay)
		}

		return fmt.Errorf("invalid section for adding: %s", section)
	})
}

func (a *App) GetRuleFormOptions(id string) (clash.RuleFormOptions, error) {
	var opts clash.RuleFormOptions
	err := clash.WithRuleStorageLock(func() error {
		if err := clash.EnsureRuleStorageMigratedLocked(id); err != nil {
			return fmt.Errorf("规则存储迁移失败: %w", err)
		}
		var innerErr error
		opts, innerErr = clash.GetRuleFormOptionsData(id)
		return innerErr
	})
	return opts, err
}

func (a *App) AddRuleFromForm(id string, section string, req clash.BuildRuleRequest) error {
	rule, err := clash.BuildRuleFromForm(req)
	if err != nil {
		return err
	}
	return a.AddRule(id, section, rule)
}

func (a *App) DeleteRule(id string, section string, index int) error {
	return clash.WithRuleStorageLock(func() error {
		_ = clash.EnsureRuleStorageMigratedLocked(id)

		if section == "local" {
			workingPath, _ := clash.WorkingConfigPath(id)
			root, err := clash.ReadWorkingRootWithRecovery(id)
			if err != nil {
				return err
			}

			rules := clash.ExtractRulesFromRootPublic(root)
			if index >= 0 && index < len(rules) {
				rules = append(rules[:index], rules[index+1:]...)
				root["rules"] = rules
				out, _ := yaml.Marshal(root)
				if err := utils.WriteFileAtomic(workingPath, out, 0644); err != nil {
					return err
				}
			}
			return nil
		}

		if section == "subscription" {
			// Moving from subscription to overlay.delete
			root, err := clash.ReadWorkingRootWithRecovery(id)
			if err != nil {
				return err
			}

			workingRules := clash.ExtractRulesFromRootPublic(root)
			overlay, err := clash.LoadRuleOverlay(id)
			if err != nil {
				return err
			}

			// Get visible base rules
			visibleRules := clash.ApplyDeleteOnly(workingRules, overlay.Delete)

			if index >= 0 && index < len(visibleRules) {
				ruleToDel := visibleRules[index]
				overlay.Delete = append([]string{ruleToDel}, overlay.Delete...)
				return clash.SaveRuleOverlay(id, overlay)
			}
			return nil
		}

		if section == "add" || section == "delete" {
			overlay, err := clash.LoadRuleOverlay(id)
			if err != nil {
				return err
			}
			if section == "add" {
				if index >= 0 && index < len(overlay.Add) {
					overlay.Add = append(overlay.Add[:index], overlay.Add[index+1:]...)
				}
			} else {
				if index >= 0 && index < len(overlay.Delete) {
					overlay.Delete = append(overlay.Delete[:index], overlay.Delete[index+1:]...)
				}
			}
			return clash.SaveRuleOverlay(id, overlay)
		}

		return nil
	})
}

func (a *App) SaveRuleSection(id string, section string, rules []string) error {
	return clash.WithRuleStorageLock(func() error {
		_ = clash.EnsureRuleStorageMigratedLocked(id)

		sanitizedRules, err := clash.SanitizeRuleList(rules)
		if err != nil {
			return err
		}

		if section == "local" || section == "subscription" {
			workingPath, _ := clash.WorkingConfigPath(id)
			root, err := clash.ReadWorkingRootWithRecovery(id)
			if err != nil {
				return err
			}
			root["rules"] = sanitizedRules
			out, _ := yaml.Marshal(root)
			if err := utils.WriteFileAtomic(workingPath, out, 0644); err != nil {
				return err
			}
			return nil
		}

		if section == "add" || section == "delete" {
			overlay, err := clash.LoadRuleOverlay(id)
			if err != nil {
				return err
			}
			if section == "add" {
				overlay.Add = sanitizedRules
			} else {
				overlay.Delete = sanitizedRules
			}
			return clash.SaveRuleOverlay(id, overlay)
		}

		return fmt.Errorf("invalid section to save: %s", section)
	})
}

func (a *App) ExportConfig(id string, useCustomRules bool) error {
	_, yamlPath, err := clash.ProfilePathByID(id)
	if err != nil {
		return fmt.Errorf("找不到配置文件: %w", err)
	}

	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return fmt.Errorf("读取配置失败: %w", err)
	}

	if useCustomRules {
		var root map[string]interface{}
		if err := yaml.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("解析 YAML 失败: %w", err)
		}

		runtimeRules, err := clash.BuildRuntimeRules(id, root)
		if err != nil {
			return err
		}

		if len(runtimeRules) > 0 {
			ruleInterfaces := make([]interface{}, len(runtimeRules))
			for i, r := range runtimeRules {
				ruleInterfaces[i] = r
			}
			root["rules"] = ruleInterfaces
		}

		data, err = yaml.Marshal(root)
		if err != nil {
			return fmt.Errorf("序列化 YAML 失败: %w", err)
		}
	}

	item, ok := clash.FindSubIndexByID(id)
	defaultName := id
	if ok && item.Name != "" {
		defaultName = item.Name
	}

	savePath, err := application.Get().Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
		Title:    "导出配置文件",
		Filename: defaultName + ".yaml",
		Filters: []application.FileFilter{
			{DisplayName: "YAML 配置文件 (*.yaml)", Pattern: "*.yaml"},
		},
	}).PromptForSingleSelection()
	if err != nil {
		if dialogCancelled(err) {
			return nil // 取消：v2 语义为直接返回
		}
		return err
	}
	if savePath == "" {
		return nil
	}

	return os.WriteFile(savePath, data, 0644)
}

func (a *App) GetAppVersion() string {
	return version.AppVersion
}

// --- Delay Test ---

func (a *App) TestAllProxies(nodeNames []string) {
	go a.core.Delay.TestAllProxies(a.ctx, nodeNames)
}

func (a *App) TestProxy(name string) (int, error) {
	ctx, cancel := context.WithTimeout(a.ctx, appcore.SingleOuterTimeout)
	defer cancel()
	return a.core.Delay.TestProxy(ctx, name)
}

func (a *App) StopDelayTest() {
	a.core.Delay.StopDelayTest()
}

// --- Updates (Core & App) ---

func (a *App) UpdateCoreComponentAsync() {
	a.core.UpdateCoreComponentAsync(a.ctx)
}

func (a *App) CheckCoreUpdateAsync() {
	a.core.CheckCoreUpdateAsync(a.ctx)
}

func (a *App) CheckCoreAgainstSeed() appcore.CoreSeedCheckResult {
	return a.core.CheckCoreAgainstSeed(a.ctx)
}

func (a *App) ReplaceCoreWithSeed() (string, error) {
	return a.core.ReplaceCoreWithSeed(a.ctx)
}

func (a *App) DisableCoreComparisonPrompt() error {
	return a.core.DisableCoreComparisonPrompt()
}

func (a *App) UpdateGeoDatabaseAsync(key string) {
	a.core.UpdateGeoDatabaseAsync(a.ctx, key)
}

func (a *App) UpdateAllGeoDatabasesAsync() {
	a.core.UpdateAllGeoDatabasesAsync(a.ctx)
}

// ClearFinishedUpdateTasks clears completed, failed, or cancelled tasks
func (a *App) ClearFinishedUpdateTasks() {
	if a.core != nil && a.core.UpdateTasks != nil {
		a.core.UpdateTasks.ClearFinished()
	}
}

func (a *App) CancelUpdateTask(key string) {
	a.core.CancelUpdateTask(key)
}

func (a *App) RemoveUpdateTask(key string) {
	a.core.RemoveUpdateTask(key)
}

func (a *App) ClearUpdateCache(key string) {
	if a.core != nil {
		a.core.ClearUpdateCache(key)
	}
}

func (a *App) DownloadPendingAppUpdateAsync() {
	a.core.DownloadPendingAppUpdateAsync(a.ctx)
}

func (a *App) CheckAndDownloadAppUpdateAsync() {
	a.core.CheckAppUpdateAsync(a.ctx, version.AppVersion, true)
}

func (a *App) ApplyAppUpdate(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("更新包路径为空")
	}

	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("更新包不存在: %v", err)
	}

	// 标记：下次启动后清理这个安装包
	if err := markAppliedUpdateForCleanup(path); err != nil {
		logger.Errorf("写入更新清理标记失败: %v", err)
	}

	// 停止托盘，避免安装期间继续触发操作
	if a.tray != nil {
		a.tray.Stop()
	}

	// 停止内核与流量流
	if a.core != nil {
		a.core.StopTrafficStream()
		a.core.StopCoreService()
	}

	// 兜底清理系统代理
	sys.ClearOwnedSystemProxy()

	// 启动安装包
	if err := sys.ShellOpen(path); err != nil {
		return fmt.Errorf("无法启动安装程序: %v", err)
	}

	// 退出当前程序，让安装程序接管
	quitApplication()
	return nil
}

func markAppliedUpdateForCleanup(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}

	marker := filepath.Join(utils.GetDataDir(), "update_cleanup_pending")
	return os.WriteFile(marker, []byte(path), 0644)
}

func cleanupAppliedUpdatePackage() {
	marker := filepath.Join(utils.GetDataDir(), "update_cleanup_pending")

	data, err := os.ReadFile(marker)
	if err != nil {
		return
	}

	updatePath := strings.TrimSpace(string(data))
	if updatePath != "" {
		_ = os.Remove(updatePath)
	}

	_ = os.Remove(marker)

	updatesDir := filepath.Join(utils.GetDataDir(), "updates")
	_ = removeEmptyDir(updatesDir)
}

func removeEmptyDir(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return os.Remove(path)
	}
	return nil
}

// --- Backup ---

func (a *App) ExportBackup() (string, error) {
	savePath, err := application.Get().Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
		Title:    "选择备份保存位置",
		Filename: fmt.Sprintf("GoclashZ_Backup_%s.gocz", time.Now().Format("20060102")),
		Filters: []application.FileFilter{
			{DisplayName: "GoclashZ 备份文件 (*.gocz)", Pattern: "*.gocz"},
		},
	}).PromptForSingleSelection()
	if err != nil {
		if dialogCancelled(err) {
			return "CANCELLED", nil // 取消：v2 语义为返回 CANCELLED 且无错误
		}
		return "", err
	}
	if savePath == "" {
		return "CANCELLED", nil
	}
	if !strings.HasSuffix(strings.ToLower(savePath), ".gocz") {
		savePath += ".gocz"
	}
	err = a.core.ExportBackup(savePath)
	if err != nil {
		return "", err
	}
	return "SUCCESS", nil
}

func (a *App) SelectBackupFile() (string, error) {
	selected, err := application.Get().Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title: "选择要还原的备份文件",
		Filters: []application.FileFilter{
			{DisplayName: "GoclashZ 备份文件 (*.gocz)", Pattern: "*.gocz"},
			{DisplayName: "Zip 压缩包 (*.zip)", Pattern: "*.zip"},
		},
	}).PromptForSingleSelection()
	if err != nil {
		if dialogCancelled(err) {
			return "", nil // 取消：v2 语义为返回空值且无错误
		}
		return "", fmt.Errorf("选择文件失败: %v", err)
	}
	return selected, nil
}

func (a *App) ExecuteRestore(selected string, mode string) (string, error) {
	err := a.core.RestoreBackup(a.ctx, selected, mode)
	if err != nil {
		return "", err
	}
	a.SyncState()
	return "SUCCESS", nil
}

func (a *App) safeShowMainWindow() {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("显示主窗口异常: %v", r)
		}
	}()

	a.ShowMainWindow()
}

func (a *App) safeToggleMainWindow() {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("切换主窗口异常: %v", r)
		}
	}()

	a.ToggleMainWindow()
}

func (a *App) safeQuit() {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("退出程序异常: %v", r)
			os.Exit(0)
		}
	}()

	// 强制退出兜底：8秒后强制结束进程
	go func() {
		time.Sleep(8 * time.Second)
		logger.Warnf("正常退出超时，强制结束进程")
		os.Exit(0)
	}()

	quitApplication()
}
