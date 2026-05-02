package main

import (
	"context"
	_ "embed"
	"fmt"
	"goclashz/core/appcore"
	"goclashz/core/clash"
	"goclashz/core/downloader"
	"goclashz/core/logger"
	"goclashz/core/sys"
	"goclashz/core/traffic"
	"goclashz/core/utils"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	ctx           context.Context
	cancelTraffic context.CancelFunc
	cancelLogs    context.CancelFunc
	logGen        int
	logRunning    bool
	mu            sync.RWMutex

	mSysProxy   *systray.MenuItem
	mTun        *systray.MenuItem
	mModeRule   *systray.MenuItem
	mModeGlobal *systray.MenuItem
	mModeDirect *systray.MenuItem

	updateMu        sync.Mutex
	coreLifecycleMu sync.Mutex
	themeCache      string

	testMu        sync.Mutex
	activeTests   int
	testSemaphore chan struct{}
	isSilentCore  bool

	appUpdateReady   bool
	newAppVersion    string
	appUpdateTaskMu  sync.Mutex
	isDownloadingApp bool

	logRestartMu sync.Mutex
	autoTestMu   sync.Mutex

	trayMu        sync.Mutex
	lastTrayClick int64
	trayOnce      sync.Once

	core *appcore.Controller
}

func NewApp() *App {
	// 初始化事件桥接
	sink := &WailsEventSink{}
	core := appcore.NewController(appcore.Options{
		Events:  sink,
		Version: CurrentAppVersion,
	})

	return &App{
		core:          core,
		testSemaphore: make(chan struct{}, 16),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if sink, ok := a.core.GetEvents().(*WailsEventSink); ok {
		sink.ctx = ctx
	}
	a.core.Startup(ctx)

	clash.LoadIndex()

	// 初始化主题缓存
	themeFile := filepath.Join(utils.GetDataDir(), "theme_setting.txt")
	if themeData, err := os.ReadFile(themeFile); err == nil && len(themeData) > 0 {
		a.themeCache = strings.TrimSpace(string(themeData))
	} else {
		a.themeCache = "dark"
	}

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
	_ = sys.ClearSystemProxy()
	a.core.StopCoreService()
	a.StopTrafficStream()
	systray.Quit()
}

// --- AppState & Sync ---

type AppState = appcore.AppState

func (a *App) GetAppState() AppState {
	return a.core.GetAppState()
}

func (a *App) SyncState() {
	state := a.core.GetAppState()
	a.core.SyncState()

	// 更新托盘勾选状态
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

	// 下发排序信息
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
	return a.core.ToggleSystemProxy(a.ctx, enable)
}

func (a *App) ToggleTunMode(enable bool) error {
	return a.core.ToggleTunMode(a.ctx, enable)
}

func (a *App) UpdateClashMode(mode string) error {
	return a.core.UpdateClashMode(a.ctx, mode)
}

func (a *App) RestartCore() error {
	return a.core.RestartCore(a.ctx)
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
	ua := a.core.Behavior.Get().SubUA
	id, err := clash.DownloadSub(a.ctx, name, url, "", ua)
	if err == nil {
		state := a.core.GetAppState()
		if state.ActiveConfig == id && state.IsRunning {
			clash.BuildRuntimeConfig(id, state.Mode, a.core.Behavior.Get().LogLevel)
			clash.ReloadConfig()
		}
	}
	return err
}

func (a *App) UpdateSingleSub(id string) error {
	clash.IndexLock.RLock()
	var url, name string
	for _, item := range clash.SubIndex {
		if item.ID == id {
			url = item.URL
			name = item.Name
			break
		}
	}
	clash.IndexLock.RUnlock()
	if url == "" {
		return fmt.Errorf("subscription not found")
	}

	ua := a.core.Behavior.Get().SubUA
	_, err := clash.DownloadSub(a.ctx, name, url, id, ua)
	if err == nil {
		state := a.core.GetAppState()
		if state.ActiveConfig == id && state.IsRunning {
			clash.BuildRuntimeConfig(id, state.Mode, a.core.Behavior.Get().LogLevel)
			clash.ReloadConfig()
		}
	}
	return err
}

func (a *App) UpdateAllSubsAsync() {
	a.core.Tasks.Run(a.ctx, "subs-update", true, func(ctx context.Context) error {
		clash.IndexLock.RLock()
		items := make([]clash.SubIndexItem, len(clash.SubIndex))
		copy(items, clash.SubIndex)
		clash.IndexLock.RUnlock()

		ua := a.core.Behavior.Get().SubUA
		for _, item := range items {
			if item.URL != "" && item.Type == "remote" {
				_, _ = clash.DownloadSub(ctx, item.Name, item.URL, item.ID, ua)
			}
		}

		state := a.core.GetAppState()
		if state.ActiveConfig != "" && state.IsRunning {
			clash.BuildRuntimeConfig(state.ActiveConfig, state.Mode, a.core.Behavior.Get().LogLevel)
			clash.ReloadConfig()
		}
		return nil
	})
}

// --- Traffic & Logs ---

func (a *App) StartTrafficStream() {
	a.mu.Lock()
	if a.cancelTraffic != nil {
		a.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelTraffic = cancel
	a.mu.Unlock()

	go func() {
		defer func() {
			a.mu.Lock()
			if a.cancelTraffic != nil {
				a.cancelTraffic = nil
			}
			a.mu.Unlock()
			runtime.EventsEmit(a.ctx, "traffic-data", map[string]string{"up": "0 B", "down": "0 B"})
		}()

		traffic.StreamTraffic(ctx, clash.APIURL("/traffic"), func(up, down string) {
			runtime.EventsEmit(a.ctx, "traffic-data", map[string]string{"up": up, "down": down})
		})
	}()
}

func (a *App) StopTrafficStream() {
	a.mu.Lock()
	if a.cancelTraffic != nil {
		a.cancelTraffic()
		a.cancelTraffic = nil
	}
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "traffic-data", map[string]string{"up": "0 B", "down": "0 B"})
}

func (a *App) StartStreamingLogs() {
	a.mu.Lock()
	if a.logRunning {
		a.mu.Unlock()
		return
	}
	a.logRunning = true
	a.logGen++
	currentGen := a.logGen
	logCtx, cancel := context.WithCancel(a.ctx)
	a.cancelLogs = cancel
	logLevel := a.core.Behavior.Get().LogLevel
	a.mu.Unlock()

	go func() {
		defer func() {
			a.mu.Lock()
			if a.logGen == currentGen {
				a.logRunning = false
				a.cancelLogs = nil
			}
			a.mu.Unlock()
			cancel()
		}()

		clash.FetchLogs(logCtx, logLevel, func(data interface{}) {
			if m, ok := data.(map[string]interface{}); ok {
				entry := logger.LogEntry{
					Type:    fmt.Sprintf("%v", m["type"]),
					Payload: fmt.Sprintf("%v", m["payload"]),
					Time:    time.Now().Format("15:04:05"),
				}
				logger.AppLogs.Add(entry)
				runtime.EventsEmit(a.ctx, "log-message", entry)
			}
		})
	}()
}

func (a *App) StopStreamingLogs() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.logRunning && a.cancelLogs != nil {
		a.cancelLogs()
		a.cancelLogs = nil
		a.logRunning = false
		a.logGen++
	}
}

func (a *App) GetRecentLogs() []logger.LogEntry {
	return logger.AppLogs.GetAll()
}

func (a *App) SearchLogs(keyword string) []logger.LogEntry {
	return logger.AppLogs.Search(keyword)
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
			a.mu.Lock()
			if a.logRunning {
				go func() {
					a.logRestartMu.Lock()
					defer a.logRestartMu.Unlock()
					a.StopStreamingLogs()
					time.Sleep(50 * time.Millisecond)
					a.StartStreamingLogs()
				}()
			}
			a.mu.Unlock()
		}
		a.core.RefreshAutoDelayTest()
		a.SyncState()
	}
	return err
}

func (a *App) ResetComponentSettings(_ string) error {
	return a.core.Behavior.SetAndSave(a.core.Behavior.Default())
}

func (a *App) SaveThemePreference(isDark bool) {
	theme := "light"
	if isDark {
		theme = "dark"
	}
	a.mu.Lock()
	if a.themeCache == theme {
		a.mu.Unlock()
		return
	}
	a.themeCache = theme
	a.mu.Unlock()

	themeFile := filepath.Join(utils.GetDataDir(), "theme_setting.txt")
	go os.WriteFile(themeFile, []byte(theme), 0644)
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

func (a *App) InstallTunDriverAsync(force bool) {
	a.core.Tasks.Run(a.ctx, "tun-driver-install", false, func(ctx context.Context) error {
		a.updateMu.Lock()
		defer a.updateMu.Unlock()

		state := a.core.GetAppState()
		isActive := state.SystemProxy || state.Tun

		if isActive {
			a.coreLifecycleMu.Lock()
			a.core.StopCoreService()
			a.coreLifecycleMu.Unlock()
		}

		_, err := sys.InstallWintun(ctx, force)
		if isActive {
			a.coreLifecycleMu.Lock()
			_ = a.core.EnsureCoreRunning(a.ctx)
			a.coreLifecycleMu.Unlock()
		}

		if err == nil {
			runtime.EventsEmit(a.ctx, "tun-driver-install-updated", map[string]any{
				"message": "Wintun 驱动安装完成",
			})
		}
		return err
	})
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
	err := clash.UpdateDNSConfig(cfg)
	if err == nil {
		state := a.core.GetAppState()
		if state.SystemProxy || state.Tun {
			a.coreLifecycleMu.Lock()
			defer a.coreLifecycleMu.Unlock()
			a.core.StopCoreService()
			_ = a.core.EnsureCoreRunning(a.ctx)
		}
	}
	return err
}

func (a *App) GetTunConfig() (*clash.TunConfig, error) {
	return clash.GetTunConfig()
}

func (a *App) SaveTunConfig(cfg *clash.TunConfig) error {
	err := clash.UpdateTunConfig(cfg)
	if err == nil {
		state := a.core.GetAppState()
		if state.SystemProxy || state.Tun {
			a.coreLifecycleMu.Lock()
			defer a.coreLifecycleMu.Unlock()
			a.core.StopCoreService()
			_ = a.core.EnsureCoreRunning(a.ctx)
		}
	}
	return err
}

func (a *App) GetNetworkConfig() (*clash.NetworkConfig, error) {
	return clash.GetNetworkConfig()
}

func (a *App) SaveNetworkConfig(cfg *clash.NetworkConfig) error {
	err := clash.UpdateNetworkConfig(cfg)
	if err == nil {
		state := a.core.GetAppState()
		if state.SystemProxy || state.Tun {
			a.coreLifecycleMu.Lock()
			defer a.coreLifecycleMu.Unlock()
			a.core.StopCoreService()
			_ = a.core.EnsureCoreRunning(a.ctx)
		}
	}
	return err
}

func (a *App) RenameConfig(id, newName string) error {
	state := a.core.GetAppState()
	isActiveConfig := (state.ActiveConfig == id)
	wasActive := state.SystemProxy || state.Tun

	if isActiveConfig {
		a.coreLifecycleMu.Lock()
		defer a.coreLifecycleMu.Unlock()
		if state.IsRunning {
			a.core.StopCoreService()
		}
	}

	err := clash.RenameConfig(id, newName)
	if isActiveConfig {
		if wasActive {
			_ = a.core.EnsureCoreRunning(a.ctx)
			a.SyncState()
		}
	}
	return err
}

func (a *App) DeleteConfig(id string) error {
	state := a.core.GetAppState()
	if state.ActiveConfig == id {
		a.coreLifecycleMu.Lock()
		defer a.coreLifecycleMu.Unlock()
		a.core.StopCoreService()
		a.core.Behavior.SetActiveConfig("")
	}
	err := clash.DeleteConfig(id)
	a.SyncState()
	return err
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
	state := a.core.GetAppState()
	if state.ActiveConfig == id {
		return nil
	}

	a.coreLifecycleMu.Lock()
	defer a.coreLifecycleMu.Unlock()

	wasRunning := state.IsRunning
	if wasRunning {
		a.core.StopCoreService()
	}

	a.core.Behavior.SetActiveConfig(id)
	if wasRunning {
		err := a.core.EnsureCoreRunning(a.ctx)
		a.SyncState()
		return err
	}
	a.SyncState()
	return nil
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
	return clash.ImportLocalConfig(srcPath, name)
}

func (a *App) StartClash(id string) error {
	a.core.Behavior.SetActiveConfig(id)
	a.coreLifecycleMu.Lock()
	defer a.coreLifecycleMu.Unlock()
	a.core.StopCoreService()
	err := a.core.EnsureCoreRunning(a.ctx)
	a.SyncState()
	return err
}

// --- Extra Utilities ---

func (a *App) GetCoreVersion() string {
	binDir := utils.GetCoreBinDir()
	exePath := filepath.Join(binDir, "clash.exe")
	if ver := getLocalCoreVersion(exePath); ver != "" {
		return ver
	}
	return clash.GetVersion()
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

// --- Speed Test ---

func (a *App) TestAllProxies(nodeNames []string) {
	a.testMu.Lock()
	a.activeTests++
	a.testMu.Unlock()

	a.coreLifecycleMu.Lock()
	if !clash.IsRunning() {
		a.testMu.Lock()
		a.isSilentCore = true
		a.testMu.Unlock()

		state := a.core.GetAppState()
		if state.ActiveConfig != "" {
			_ = clash.BuildRuntimeConfig(state.ActiveConfig, state.Mode, a.core.Behavior.Get().LogLevel)
		}
		_ = clash.Start(a.ctx)
		// Wait for API
		for i := 0; i < 20; i++ {
			if _, err := clash.GetInitialData(); err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	a.coreLifecycleMu.Unlock()

	go func() {
		defer func() {
			a.testMu.Lock()
			a.activeTests--
			remaining := a.activeTests
			isSilent := a.isSilentCore
			a.testMu.Unlock()

			if remaining == 0 && isSilent {
				a.coreLifecycleMu.Lock()
				state := a.core.GetAppState()
				if !state.SystemProxy && !state.Tun {
					clash.Stop()
				}
				a.testMu.Lock()
				a.isSilentCore = false
				a.testMu.Unlock()
				a.coreLifecycleMu.Unlock()
			}
		}()

		testUrl := "http://www.gstatic.com/generate_204"
		if netCfg, err := clash.GetNetworkConfig(); err == nil && netCfg.TestURL != "" {
			testUrl = netCfg.TestURL
		}

		jobs := make(chan string, len(nodeNames))
		var wg sync.WaitGroup
		workerCount := 16
		if len(nodeNames) < workerCount {
			workerCount = len(nodeNames)
		}

		for i := 0; i < workerCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for nName := range jobs {
					select {
					case a.testSemaphore <- struct{}{}:
					case <-a.ctx.Done():
						return
					}
					reqCtx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
					delay, err := clash.GetProxyDelay(reqCtx, nName, testUrl)
					cancel()
					<-a.testSemaphore

					status := "success"
					if err != nil || delay <= 0 {
						status = "timeout"
						delay = 0
					}
					runtime.EventsEmit(a.ctx, "proxy-delay-update", map[string]interface{}{
						"name": nName, "delay": delay, "status": status,
					})
				}
			}()
		}

		for _, name := range nodeNames {
			runtime.EventsEmit(a.ctx, "proxy-test-start", name)
			jobs <- name
		}
		close(jobs)
		wg.Wait()
		runtime.EventsEmit(a.ctx, "proxy-test-finished", "测速完成")
	}()
}

// --- Updates (Core & App) ---

func (a *App) UpdateCoreComponentAsync() {
	a.core.Tasks.Run(a.ctx, "core-update", true, func(ctx context.Context) error {
		a.updateMu.Lock()
		defer a.updateMu.Unlock()

		state := a.core.GetAppState()
		if state.IsRunning {
			a.coreLifecycleMu.Lock()
			a.core.StopCoreService()
			a.coreLifecycleMu.Unlock()
		}

		_, err := clash.UpdateCore(ctx)
		if state.SystemProxy || state.Tun {
			a.coreLifecycleMu.Lock()
			_ = a.core.EnsureCoreRunning(a.ctx)
			a.coreLifecycleMu.Unlock()
		}
		return err
	})
}

func (a *App) UpdateGeoDatabaseAsync(key string) {
	a.core.Tasks.Run(a.ctx, "geo-update-"+key, true, func(ctx context.Context) error {
		behavior := a.core.Behavior.Get()
		url := ""
		switch key {
		case "geoip":
			url = behavior.GeoIpLink
		case "geosite":
			url = behavior.GeoSiteLink
		case "mmdb":
			url = behavior.MmdbLink
		case "asn":
			url = behavior.AsnLink
		}
		if url == "" {
			return fmt.Errorf("no URL configured for %s", key)
		}
		return clash.UpdateGeoDB(ctx, key, url)
	})
}

func (a *App) UpdateAllGeoDatabasesAsync() {
	a.UpdateGeoDatabaseAsync("geoip")
	a.UpdateGeoDatabaseAsync("geosite")
	a.UpdateGeoDatabaseAsync("mmdb")
	a.UpdateGeoDatabaseAsync("asn")
}

func (a *App) CheckAndDownloadAppUpdateAsync() {
	a.core.Tasks.Run(a.ctx, "app-update-check", false, func(ctx context.Context) error {
		info, err := downloader.CheckAppUpdate(ctx, CurrentAppVersion)
		if err != nil {
			return err
		}
		if info != nil && info.HasUpdate {
			a.mu.Lock()
			a.newAppVersion = info.Version
			a.appUpdateReady = true
			a.mu.Unlock()
			runtime.EventsEmit(a.ctx, "app-update-ready", info)
		}
		return nil
	})
}

func (a *App) ApplyAppUpdate() error {
	// 实现应用更新逻辑
	return nil
}

func (a *App) ManualCheckAppUpdate() (string, error) {
	info, err := downloader.CheckAppUpdate(a.ctx, CurrentAppVersion)
	if err != nil {
		return "", err
	}
	if info != nil && info.HasUpdate {
		return info.Version, nil
	}
	return "", nil
}

// --- Helpers ---

func getThemeConfigPath() string {
	return filepath.Join(utils.GetDataDir(), "theme_setting.txt")
}

func getLocalCoreVersion(_ string) string {
	// 实现本地版本读取逻辑
	return ""
}

func normalizeVersion(v string) string {
	return strings.TrimSpace(strings.ToLower(v))
}

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

	// 信号处理
	mShow.Click(func() {
		runtime.WindowShow(a.ctx)
		runtime.WindowUnmaximise(a.ctx)
	})
	a.mSysProxy.Click(func() {
		state := a.core.GetAppState()
		a.ToggleSystemProxy(!state.SystemProxy)
	})
	a.mTun.Click(func() {
		state := a.core.GetAppState()
		a.ToggleTunMode(!state.Tun)
	})
	a.mModeRule.Click(func() {
		a.UpdateClashMode("rule")
	})
	a.mModeGlobal.Click(func() {
		a.UpdateClashMode("global")
	})
	a.mModeDirect.Click(func() {
		a.UpdateClashMode("direct")
	})
	mRestart.Click(func() {
		a.RestartCore()
	})
	mQuit.Click(func() {
		runtime.Quit(a.ctx)
	})

	a.SyncState()
}

func (a *App) onTrayExit() {
	// 托盘退出清理
}
