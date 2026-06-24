//go:build windows

package main

import (
	"context"
	_ "embed"
	"fmt"
	"goclashz/core/appcore"
	"goclashz/core/clash"
	"goclashz/core/logger"
	"goclashz/core/sys"
	"goclashz/core/utils"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"goclashz/core/version"
	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/runtime"
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

	windowMu      sync.Mutex
	windowVisible bool

	// 全局热键线程 ID (Ctrl+Alt+Q 退出)
	hotkeyTID atomic.Uint32

	tray *TrayRuntime
	core *appcore.Controller
}

func (a *App) setWindowVisible(v bool) {
	a.windowMu.Lock()
	a.windowVisible = v
	a.windowMu.Unlock()
}

func (a *App) isWindowVisible() bool {
	a.windowMu.Lock()
	defer a.windowMu.Unlock()
	return a.windowVisible
}

func (a *App) ShowMainWindow() {
	if a.ctx == nil {
		return
	}

	runtime.WindowShow(a.ctx)
	runtime.WindowUnmaximise(a.ctx)

	// Windows 下从托盘恢复时，有时需要主动拉前台并闪烁提醒
	sys.FocusMainWindowAndFlashTwiceWin32Only()

	a.setWindowVisible(true)
}

func (a *App) HideMainWindow() {
	if a.ctx == nil {
		return
	}

	runtime.WindowHide(a.ctx)
	a.setWindowVisible(false)
}

func (a *App) ToggleMainWindow() {
	if a.isWindowVisible() {
		a.HideMainWindow()
		return
	}

	a.ShowMainWindow()
}

func NewApp() *App {
	app := &App{}
	app.core = appcore.NewController(appcore.Options{
		Events:        &WailsEventSink{},
		Version:       version.AppVersion,
		OnStateChange: app.syncTrayState,
	})
	app.tray = NewTrayRuntime(app)
	return app
}

func (a *App) startup(ctx context.Context) {
	cleanupAppliedUpdatePackage()

	a.ctx = ctx
	if sink, ok := a.core.GetEvents().(*WailsEventSink); ok {
		sink.ctx = ctx
	}
	// 必须先加载订阅索引，再启动 appcore 的自动任务。
	clash.LoadIndex()
	
	// 执行规则存储从 V1 (_rules.json) 到 V2 (yaml/overlay) 的全量迁移
	_ = clash.MigrateRuleStorageV2()

	config := a.core.Behavior.Get()
	isSilent := hasFlag("--silent") || config.SilentStart

	a.core.Bootstrap(ctx, appcore.BootstrapOptions{
		IsStartupLaunch: hasFlag("--startup"),
		Silent:          isSilent,
	})

	if !isSilent {
		runtime.WindowShow(ctx)
		a.setWindowVisible(true)
	} else {
		runtime.WindowHide(ctx)
		a.setWindowVisible(false)
	}

	a.SyncState()

	go a.startHotkeyWorker(ctx)
	a.tray.Start(ctx)
}

func (a *App) shutdown(ctx context.Context) {
	if a.tray != nil {
		a.tray.Stop()
	}
	a.core.StopCoreService()
	a.core.StopTrafficStream()

	// 主程序退出时停止 Helper 服务
	_ = sys.StopHelperService()
}

// --- AppState & Sync ---

type AppState = appcore.AppState
type ComponentFileInfo = appcore.ComponentFileInfo

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
	ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
	defer cancel()
	return a.core.ToggleSystemProxy(ctx, enable)
}

func (a *App) ToggleTunMode(enable bool) error {
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return a.core.ToggleTunMode(ctx, enable)
}

func (a *App) UpdateClashMode(mode string) error {
	return a.core.UpdateClashMode(a.ctx, mode)
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

func (a *App) GetAppBehavior() AppBehavior {
	return a.core.Behavior.Get()
}

func (a *App) SaveAppBehavior(config AppBehavior) error {
	return a.core.SaveAppBehavior(config)
}

func (a *App) ResetComponentSettings(section string) error {
	switch section {
	case "behavior":
		old := a.core.Behavior.Get()
		cfg := a.core.Behavior.Default()
		cfg.ActiveConfig = old.ActiveConfig
		cfg.ActiveMode = old.ActiveMode

		if err := a.core.SaveAppBehavior(cfg); err != nil {
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
	a.SyncState()
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

	// 非管理员需要提权
	if !sys.CheckAdmin() {
		if err := sys.RunElevatedWithArgsWait("--install-helper"); err != nil {
			return fmt.Errorf("安装后台服务失败: %w", err)
		}
		// 等待服务就绪
		time.Sleep(500 * time.Millisecond)
		return sys.WaitForHelperReady(5, 1*time.Second)
	}

	if err := sys.InstallHelperService(exePath); err != nil {
		return err
	}
	if err := sys.StartHelperService(); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	return sys.WaitForHelperReady(5, 1*time.Second)
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
	return sys.GetDataDirInfo()
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
	clash.MigrateCoreAssetsToBin()
	if !sys.CheckAdmin() {
		return sys.RunElevatedWithArgsWait("--repair-core-layout")
	}
	return utils.RepairCoreBinPermission()
}

func (a *App) CheckTunEnv() map[string]bool {
	helperStatus := sys.CheckHelperService()
	return map[string]bool{
		"isAdmin":       sys.CheckAdmin(),
		"hasWintun":     sys.IsWintunInstalled(),
		"helperRunning": helperStatus.Reachable,
	}
}


func (a *App) InstallTunDriverAsync(_ bool) {
	a.core.InstallTunDriverAsync(a.ctx)
}

func (a *App) GetWintunVersion() string {
	path := filepath.Join(utils.GetCoreBinDir(), "wintun.dll")
	if _, err := os.Stat(path); err != nil {
		return "未安装"
	}
	v, err := sys.GetFileVersion(path)
	if err != nil || strings.TrimSpace(v) == "" {
		return "已安装，版本未知"
	}
	return v
}

func (a *App) GetComponentFileInfo() map[string]ComponentFileInfo {
	return appcore.GetComponentFileInfo()
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
	if err := a.core.SelectLocalConfig(a.ctx, id); err != nil {
		return err
	}
	if !a.core.GetAppState().IsRunning {
		return a.core.RestartCoreWithReason(a.ctx, "config-switch")
	}
	return nil
}

// --- Extra Utilities ---

func (a *App) GetCoreVersion() string {
	return a.core.GetCoreVersion(a.ctx)
}

func (a *App) GetProxyDelay(proxyName, testUrl string) (int, error) {
	return clash.GetProxyDelay(a.ctx, proxyName, testUrl, 8000)
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

	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "导出配置文件",
		DefaultFilename: defaultName + ".yaml",
		Filters: []runtime.FileFilter{
			{DisplayName: "YAML 配置文件 (*.yaml)", Pattern: "*.yaml"},
		},
	})
	if err != nil {
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

// --- Updates (Core & App) ---

func (a *App) UpdateCoreComponentAsync() {
	a.core.UpdateCoreComponentAsync(a.ctx)
}

func (a *App) CheckCoreUpdateAsync() {
	a.core.CheckCoreUpdateAsync(a.ctx)
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
	runtime.Quit(a.ctx)
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
		}
	}()

	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}
