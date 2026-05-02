package main

import (
	"context"
	_ "embed"
	"fmt"
	"goclashz/core/appcore"
	"goclashz/core/clash"
	"goclashz/core/sys"
	"goclashz/core/utils"
	"os"
	"os/exec"
	"strings"
	"path/filepath"
	"sync"
	"time"

	"github.com/energye/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/windows/icon.ico
var iconData []byte

const CurrentAppVersion = "v1.1.3"

type FileInfo struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

type App struct {
	ctx   context.Context
	mu    sync.RWMutex

	mSysProxy   *systray.MenuItem
	mTun        *systray.MenuItem
	mModeRule   *systray.MenuItem
	mModeGlobal *systray.MenuItem
	mModeDirect *systray.MenuItem

	trayMu        sync.Mutex
	lastTrayClick int64
	trayOnce      sync.Once

	core *appcore.Controller
}

func NewApp() *App {
	a := &App{}

	sink := &WailsEventSink{}
	core := appcore.NewController(appcore.Options{
		Events:  sink,
		Version: CurrentAppVersion,
		RunDelayTest: func() {
			// 🚀 核心修复：连接自动测速钩子
			state := a.core.GetAppState()
			if state.IsRunning {
				// 获取所有非系统节点的名称（简化处理，触发全量测速）
				go a.TestAllProxies(nil)
			}
		},
	})
	a.core = core

	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if sink, ok := a.core.GetEvents().(*WailsEventSink); ok {
		sink.ctx = ctx
	}
	a.core.Startup(ctx)
	clash.LoadIndex()

	config := a.core.Behavior.Get()
	if !config.SilentStart {
		runtime.WindowShow(ctx)
	}

	a.SyncState()

	go func() {
		time.Sleep(500 * time.Millisecond)
		a.SetupSystray()
	}()
}

func (a *App) shutdown(ctx context.Context) {
	a.core.StopCoreService()
	a.core.StopTrafficStream()
	systray.Quit()
}

// --- AppState & Sync ---

type AppState = appcore.AppState

func (a *App) GetAppState() AppState {
	return a.core.GetAppState()
}

func (a *App) SyncState() {
	// 内部会发送 app-state-sync 并且自动启停 traffic stream
	a.core.SyncState()

	state := a.core.GetAppState()
	if a.mSysProxy != nil {
		if state.SystemProxy {
			a.mSysProxy.Check()
		} else {
			a.mSysProxy.Uncheck()
		}
	}
	if a.mTun != nil {
		if state.Tun {
			a.mTun.Check()
		} else {
			a.mTun.Uncheck()
		}
	}
	if a.mModeRule != nil {
		a.mModeRule.Uncheck()
		a.mModeGlobal.Uncheck()
		a.mModeDirect.Uncheck()
		switch state.Mode {
		case "rule":
			a.mModeRule.Check()
		case "global":
			a.mModeGlobal.Check()
		case "direct":
			a.mModeDirect.Check()
		}
	}
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
			appcore.MergeOfflineSelection(offlineData, a.core.Offline.Snapshot())
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

// --- Toggles & Controls ---

func (a *App) ToggleSystemProxy(enable bool) error {
	err := a.core.ToggleSystemProxy(a.ctx, enable)
	a.SyncState()
	return err
}

func (a *App) ToggleTunMode(enable bool) error {
	err := a.core.ToggleTunMode(a.ctx, enable)
	a.SyncState()
	return err
}

func (a *App) UpdateClashMode(mode string) error {
	err := a.core.UpdateClashMode(a.ctx, mode)
	a.SyncState()
	return err
}

func (a *App) RestartCore() error {
	return a.restartCoreAndSync()
}

func (a *App) restartCoreAndSync() error {
	// 💡 核心：因为 Controller 的 RestartCore 内部执行 ensureCoreRunning 失败或成功都会调用 c.SyncState()
	// 而 c.SyncState() 现在会自动根据 state.IsRunning 管理 traffic 的 Stop 和 Start
	err := a.core.RestartCore(a.ctx)
	a.SyncState() // 同步前端托盘
	return err
}

func (a *App) SelectProxy(groupName, nodeName string) error {
	a.core.Offline.Mark(groupName, nodeName)
	if !clash.IsRunning() {
		return nil
	}
	err := clash.SelectProxy(groupName, nodeName)
	if err == nil {
		a.SyncState()
	}
	return err
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
	clash.IndexLock.RLock()
	defer clash.IndexLock.RUnlock()
	return clash.SubIndex
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

func (a *App) StopStreamingLogs() {
	a.core.StopLogStream()
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

// --- Behavior & Theme ---

type AppBehavior = appcore.AppBehavior

func (a *App) GetAppBehavior() AppBehavior {
	return a.core.Behavior.Get()
}

func (a *App) SaveAppBehavior(config AppBehavior) error {
	oldLogLevel := a.core.Behavior.Get().LogLevel
	err := a.core.Behavior.SetAndSave(config)
	if err == nil {
		if oldLogLevel != config.LogLevel {
			if a.core.IsLogStreaming() {
				go func() {
					a.StopStreamingLogs()
					time.Sleep(50 * time.Millisecond)
					a.StartStreamingLogs()
				}()
			}
		}
		a.core.RefreshAutoDelayTest()
		a.SyncState()
	}
	return err
}

func (a *App) ResetComponentSettings(_ string) error {
	err := a.core.Behavior.SetAndSave(a.core.Behavior.Default())
	a.SyncState()
	return err
}

func (a *App) SaveThemePreference(isDark bool) {
	theme := "light"
	if isDark {
		theme = "dark"
	}
	_ = utils.SaveGlobalTheme(theme)
	a.SyncState()
}

// --- System Tools ---

func (a *App) FixUWPNetwork() error {
	if !sys.CheckAdmin() {
		return fmt.Errorf("admin privileges required")
	}
	return sys.ExemptAllUWP()
}

func (a *App) CheckTunEnv() map[string]bool {
	return map[string]bool{
		"isAdmin":   sys.CheckAdmin(),
		"hasWintun": sys.IsWintunInstalled(),
	}
}

func (a *App) ElevatePrivileges() error {
	return sys.RequestAdmin()
}

func (a *App) InstallTunDriverAsync(_ bool) {
	a.core.InstallTunDriverAsync(a.ctx)
}

func (a *App) GetWintunVersion() string {
	if sys.IsWintunInstalled() {
		return "Installed"
	}
	return "Not Installed"
}

func (a *App) GetUwpApps() ([]sys.UwpApp, error) {
	return sys.GetUwpAppList()
}

func (a *App) SaveUwpExemptions(sids []string) error {
	return sys.SaveUwpExemptions(sids)
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
	return a.core.DeleteConfig(id)
}

func (a *App) OpenConfigFile(id string) error {
	safeId, err := utils.SanitizeFilename(id)
	if err != nil {
		return err
	}
	path := filepath.Join(utils.GetSubscriptionsDir(), safeId+".yaml")
	if id == "" || id == "config.yaml" {
		path = filepath.Join(utils.GetDataDir(), "config.yaml")
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", path)
	}

	return exec.Command("cmd", "/c", "start", "", path).Start()
}

func (a *App) SelectLocalConfig(id string) error {
	return a.core.SelectLocalConfig(a.ctx, id)
}

func (a *App) SelectLocalFile() (FileInfo, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择本地配置文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "Clash 配置文件 (*.yaml; *.yml)", Pattern: "*.yaml;*.yml"},
		},
	})
	if err != nil || path == "" {
		return FileInfo{}, err
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
	if err := a.core.Behavior.SetActiveConfig(id); err != nil {
		return err
	}
	return a.restartCoreAndSync()
}

// --- Extra Utilities ---

func (a *App) GetCoreVersion() string {
	return a.core.GetCoreVersion()
}

func (a *App) GetProxyDelay(proxyName, testUrl string) (int, error) {
	return clash.GetProxyDelay(a.ctx, proxyName, testUrl)
}

func (a *App) GetCustomRules(id string) ([]string, error) {
	return clash.GetCustomRules(id)
}

func (a *App) SaveCustomRules(id string, rules []string) error {
	return clash.SaveCustomRules(id, rules)
}

func (a *App) SyncRules(id string) error {
	return clash.SyncRulesFromYaml(id)
}

func (a *App) GetAppVersion() string {
	return CurrentAppVersion
}

func (a *App) FlashWindow() {
	sys.FocusMainWindowAndFlashTwiceWin32Only()
}

// --- Delay Test ---

func (a *App) TestAllProxies(nodeNames []string) {
	go a.core.Delay.TestAllProxies(a.ctx, nodeNames)
}

func (a *App) TestProxy(name string) (int, error) {
	return a.core.Delay.TestProxy(name)
}

// --- Updates (Core & App) ---

func (a *App) UpdateCoreComponentAsync() {
	a.core.UpdateCoreComponentAsync(a.ctx)
}

func (a *App) UpdateGeoDatabaseAsync(key string) {
	a.core.UpdateGeoDatabaseAsync(a.ctx, key)
}

func (a *App) UpdateAllGeoDatabasesAsync() {
	a.core.UpdateAllGeoDatabasesAsync(a.ctx)
}

func (a *App) CheckAndDownloadAppUpdateAsync() {
	a.core.CheckAndDownloadAppUpdateAsync(a.ctx, CurrentAppVersion)
}

func (a *App) ApplyAppUpdate() error {
	// 🛡️ 核心修复：明确告知暂未开启自动安装
	return fmt.Errorf("自动安装功能暂未开启，请手动下载并覆盖安装")
}

func (a *App) ManualCheckAppUpdate() (string, error) {
	return a.core.ManualCheckAppUpdate(a.ctx)
}

// --- Backup ---

func (a *App) ExportBackup() (string, error) {
	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "选择备份保存位置",
		DefaultFilename: fmt.Sprintf("GoclashZ_Backup_%s.gocz", time.Now().Format("20060102")),
		Filters: []runtime.FileFilter{
			{DisplayName: "GoclashZ 备份文件 (*.gocz)", Pattern: "*.gocz"},
		},
	})
	if err != nil {
		return "", err
	}
	if savePath == "" {
		return "CANCELLED", nil
	}

	// 补全后缀
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
	selected, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择要还原的备份文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "GoclashZ 备份文件 (*.gocz)", Pattern: "*.gocz"},
			{DisplayName: "Zip 压缩包 (*.zip)", Pattern: "*.zip"},
		},
	})
	if err != nil {
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

// --- Helpers ---


func (a *App) SetupSystray() {
	a.trayMu.Lock()
	defer a.trayMu.Unlock()

	a.trayOnce.Do(func() {
		go systray.Run(a.onTrayReady, a.onTrayExit)
	})
}

func (a *App) onTrayReady() {
	systray.SetIcon(iconData)
	systray.SetTitle("GoclashZ")
	systray.SetTooltip("GoclashZ - Mihomo GUI")

	mShow := systray.AddMenuItem("显示界面", "显示主窗口")
	systray.AddSeparator()

	a.mSysProxy = systray.AddMenuItem("系统代理", "开启/关闭系统代理")
	a.mTun = systray.AddMenuItem("TUN 模式", "开启/关闭 TUN 模式")

	systray.AddSeparator()
	mModes := systray.AddMenuItem("出站模式", "切换 Clash 路由模式")
	a.mModeRule = mModes.AddSubMenuItem("规则 (Rule)", "Rule 模式")
	a.mModeGlobal = mModes.AddSubMenuItem("全局 (Global)", "Global 模式")
	a.mModeDirect = mModes.AddSubMenuItem("直连 (Direct)", "Direct 模式")

	systray.AddSeparator()
	mRestart := systray.AddMenuItem("重启内核", "重启 Clash 内核")
	mQuit := systray.AddMenuItem("退出程序", "彻底退出 GoclashZ")

	mShow.Click(func() {
		runtime.WindowShow(a.ctx)
		runtime.WindowUnmaximise(a.ctx)
	})
	a.mSysProxy.Click(func() {
		state := a.core.GetAppState()
		_ = a.ToggleSystemProxy(!state.SystemProxy)
	})
	a.mTun.Click(func() {
		state := a.core.GetAppState()
		_ = a.ToggleTunMode(!state.Tun)
	})
	a.mModeRule.Click(func() {
		_ = a.UpdateClashMode("rule")
	})
	a.mModeGlobal.Click(func() {
		_ = a.UpdateClashMode("global")
	})
	a.mModeDirect.Click(func() {
		_ = a.UpdateClashMode("direct")
	})
	mRestart.Click(func() {
		_ = a.RestartCore()
	})
	mQuit.Click(func() {
		runtime.Quit(a.ctx)
	})

	a.SyncState()
}

func (a *App) onTrayExit() {
}
