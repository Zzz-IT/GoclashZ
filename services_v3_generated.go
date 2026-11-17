//go:build windows

package main

// 本文件由生成器根据 app.go / outbound_ip.go / config_editor.go 中 *App 的真实方法签名
// 自动生成（禁止手工编辑）。每个转发方法的方法体只有一句 s.app.<Method>(...)，
// 不含任何业务逻辑；参数类型、返回值类型与 App 上同名方法逐字一致。
// 方法归属（App/Core/Update）见 services_v3_mapping.go。

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"

	"goclashz/core/appcore"
	"goclashz/core/clash"
	"goclashz/core/runtimeassets"
	"goclashz/core/sys"
)

// AppService 是「UI 与系统设置」转发服务：窗口/托盘/主题/设置行为/诊断/备份/数据目录/启动项/Helper/UWP/管理员等。
// 所有方法仅转发到 *App，签名与 App 上同名方法完全一致。
// ServiceStartup/ServiceShutdown 也由本服务承接（main.go 中排第一，保持原有启动顺序与行为）。
type AppService struct {
	app *App
}

// ServiceStartup 原样转发到 App.ServiceStartup：启动流程、窗口/托盘/热键初始化顺序零改动。
func (s *AppService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return s.app.ServiceStartup(ctx, options)
}

// ServiceShutdown 原样转发到 App.ServiceShutdown：退出清理顺序（tray -> helper -> core）零改动。
func (s *AppService) ServiceShutdown() error {
	return s.app.ServiceShutdown()
}

func (s *AppService) ShowMainWindow() {
	s.app.ShowMainWindow()
}

func (s *AppService) HideMainWindow() {
	s.app.HideMainWindow()
}

func (s *AppService) ToggleMainWindow() {
	s.app.ToggleMainWindow()
}

func (s *AppService) GetAppState() AppState {
	return s.app.GetAppState()
}

func (s *AppService) SyncState() {
	s.app.SyncState()
}

func (s *AppService) GetInitialData() (map[string]interface{}, error) {
	return s.app.GetInitialData()
}

func (s *AppService) GetAppBehavior() AppBehavior {
	return s.app.GetAppBehavior()
}

func (s *AppService) SaveAppBehavior(config AppBehavior) error {
	return s.app.SaveAppBehavior(config)
}

func (s *AppService) ResetComponentSettings(section string) error {
	return s.app.ResetComponentSettings(section)
}

func (s *AppService) SaveThemePreference(isDark bool) {
	s.app.SaveThemePreference(isDark)
}

func (s *AppService) GetDiagnosticInfo() appcore.DiagnosticInfo {
	return s.app.GetDiagnosticInfo()
}

func (s *AppService) ExportDiagnostics() (string, error) {
	return s.app.ExportDiagnostics()
}

func (s *AppService) ExportBackup() (string, error) {
	return s.app.ExportBackup()
}

func (s *AppService) SelectBackupFile() (string, error) {
	return s.app.SelectBackupFile()
}

func (s *AppService) ExecuteRestore(selected string, mode string) (string, error) {
	return s.app.ExecuteRestore(selected, mode)
}

func (s *AppService) GetDataDirInfo() sys.DataDirInfo {
	return s.app.GetDataDirInfo()
}

func (s *AppService) RepairDataDirMigration() error {
	return s.app.RepairDataDirMigration()
}

func (s *AppService) RepairDataDirPermission() error {
	return s.app.RepairDataDirPermission()
}

func (s *AppService) RepairCoreLayout() error {
	return s.app.RepairCoreLayout()
}

func (s *AppService) GetStartupTaskInfo() (sys.StartupTaskInfo, error) {
	return s.app.GetStartupTaskInfo()
}

func (s *AppService) RepairStartupTask() error {
	return s.app.RepairStartupTask()
}

// GetHelperServiceStatus 返回后台服务状态
func (s *AppService) GetHelperServiceStatus() sys.HelperStatusData {
	return s.app.GetHelperServiceStatus()
}

// InstallHelperService 安装后台服务（需要 UAC 提权）
func (s *AppService) InstallHelperService() error {
	return s.app.InstallHelperService()
}

// UninstallHelperService 卸载后台服务（需要 UAC 提权）
func (s *AppService) UninstallHelperService() error {
	return s.app.UninstallHelperService()
}

// RestartHelperService 重启后台服务（需要 UAC 提权）
func (s *AppService) RestartHelperService() error {
	return s.app.RestartHelperService()
}

func (s *AppService) GetUwpApps() ([]sys.UwpApp, error) {
	return s.app.GetUwpApps()
}

func (s *AppService) SaveUwpExemptions(sids []string) error {
	return s.app.SaveUwpExemptions(sids)
}

// ApplyUwpExemptionChanges UWP 豁免差异应用：只动传入的两组 SID，不枚举、不推断其它豁免
func (s *AppService) ApplyUwpExemptionChanges(added []string, removed []string) error {
	return s.app.ApplyUwpExemptionChanges(added, removed)
}

// ClearAllUwpExemptions 清空全部 UWP 豁免：只删除本次枚举到的已豁免 SID
func (s *AppService) ClearAllUwpExemptions() error {
	return s.app.ClearAllUwpExemptions()
}

func (s *AppService) RestartAsAdmin() error {
	return s.app.RestartAsAdmin()
}

func (s *AppService) GetAppVersion() string {
	return s.app.GetAppVersion()
}

// CoreService 是「clash 内核与网络运行时」转发服务：内核/代理/规则/订阅/连接/日志/流量/配置/DNS/TUN/网络/运行时资产等。
// 所有方法仅转发到 *App，签名与 App 上同名方法完全一致。
type CoreService struct {
	app *App
}

func (s *CoreService) GetConnections() (appcore.ConnectionsSnapshot, error) {
	return s.app.GetConnections()
}

func (s *CoreService) StartConnectionMonitor() {
	s.app.StartConnectionMonitor()
}

func (s *CoreService) StopConnectionMonitor() {
	s.app.StopConnectionMonitor()
}

func (s *CoreService) CloseAllConnections() error {
	return s.app.CloseAllConnections()
}

func (s *CoreService) CloseConnection(id string) error {
	return s.app.CloseConnection(id)
}

func (s *CoreService) ToggleSystemProxy(enable bool) error {
	return s.app.ToggleSystemProxy(enable)
}

func (s *CoreService) ToggleTunMode(enable bool) error {
	return s.app.ToggleTunMode(enable)
}

func (s *CoreService) UpdateClashMode(mode string) error {
	return s.app.UpdateClashMode(mode)
}

func (s *CoreService) RestartCore() error {
	return s.app.RestartCore()
}

func (s *CoreService) StartClash(id string) error {
	return s.app.StartClash(id)
}

// Deprecated: 等价于 GetRuntimeAssetStatus().Assets[core].Version，保留供 tray/单值查询使用。
func (s *CoreService) GetCoreVersion() string {
	return s.app.GetCoreVersion()
}

func (s *CoreService) SelectProxy(groupName, proxyName string) error {
	return s.app.SelectProxy(groupName, proxyName)
}

func (s *CoreService) GetProxyDelay(proxyName, testUrl string) (int, error) {
	return s.app.GetProxyDelay(proxyName, testUrl)
}

func (s *CoreService) TestAllProxies(nodeNames []string) {
	s.app.TestAllProxies(nodeNames)
}

func (s *CoreService) TestProxy(name string) (int, error) {
	return s.app.TestProxy(name)
}

func (s *CoreService) StopDelayTest() {
	s.app.StopDelayTest()
}

func (s *CoreService) FlushFakeIP() error {
	return s.app.FlushFakeIP()
}

func (s *CoreService) GetLocalConfigs() []clash.SubIndexItem {
	return s.app.GetLocalConfigs()
}

func (s *CoreService) UpdateSub(name, url string) error {
	return s.app.UpdateSub(name, url)
}

func (s *CoreService) UpdateSingleSub(id string) error {
	return s.app.UpdateSingleSub(id)
}

func (s *CoreService) UpdateAllSubsAsync() {
	s.app.UpdateAllSubsAsync()
}

func (s *CoreService) StartStreamingLogs() {
	s.app.StartStreamingLogs()
}

func (s *CoreService) GetRecentLogs() []appcore.LogEntry {
	return s.app.GetRecentLogs()
}

func (s *CoreService) SearchLogs(keyword string) []appcore.LogEntry {
	return s.app.SearchLogs(keyword)
}

func (s *CoreService) ClearLogs() {
	s.app.ClearLogs()
}

func (s *CoreService) ResetTrafficTotals() {
	s.app.ResetTrafficTotals()
}

func (s *CoreService) GetDNSConfig() (*clash.DNSConfig, error) {
	return s.app.GetDNSConfig()
}

func (s *CoreService) SaveDNSConfig(cfg *clash.DNSConfig) error {
	return s.app.SaveDNSConfig(cfg)
}

func (s *CoreService) GetTunConfig() (*clash.TunConfig, error) {
	return s.app.GetTunConfig()
}

func (s *CoreService) SaveTunConfig(cfg *clash.TunConfig) error {
	return s.app.SaveTunConfig(cfg)
}

func (s *CoreService) GetNetworkConfig() (*clash.NetworkConfig, error) {
	return s.app.GetNetworkConfig()
}

func (s *CoreService) SaveNetworkConfig(cfg *clash.NetworkConfig) error {
	return s.app.SaveNetworkConfig(cfg)
}

func (s *CoreService) RenameConfig(id, newName string) error {
	return s.app.RenameConfig(id, newName)
}

func (s *CoreService) DeleteConfig(id string) error {
	return s.app.DeleteConfig(id)
}

func (s *CoreService) SelectLocalConfig(id string) error {
	return s.app.SelectLocalConfig(id)
}

func (s *CoreService) SelectLocalFile() (FileInfo, error) {
	return s.app.SelectLocalFile()
}

func (s *CoreService) DoLocalImport(srcPath, name string) (string, error) {
	return s.app.DoLocalImport(srcPath, name)
}

func (s *CoreService) ExportConfig(id string, useCustomRules bool) error {
	return s.app.ExportConfig(id, useCustomRules)
}

func (s *CoreService) GetRuntimeAssetStatus() runtimeassets.RuntimeAssetStatus {
	return s.app.GetRuntimeAssetStatus()
}

func (s *CoreService) RepairRuntimeAssets() runtimeassets.RuntimeAssetStatus {
	return s.app.RepairRuntimeAssets()
}

// RepairRuntimeAssetsAsync 后台重写全部运行组件，立即返回进度快照（不阻塞 UI）。
func (s *CoreService) RepairRuntimeAssetsAsync() AssetRepairProgress {
	return s.app.RepairRuntimeAssetsAsync()
}

// GetRuntimeAssetRepairProgress 轮询异步修复进度（轮询即可，无需新增事件类型）。
func (s *CoreService) GetRuntimeAssetRepairProgress() AssetRepairProgress {
	return s.app.GetRuntimeAssetRepairProgress()
}

// GetConfigDeviations 配置偏差：返回保存了但不会生效的设置项（module 为空表示全部）
func (s *CoreService) GetConfigDeviations(module string) ([]clash.ConfigDeviation, error) {
	return s.app.GetConfigDeviations(module)
}

// ExplainDNS DNS 解析决策：该域名最终用哪个解析器（静态推演，不发真实查询）
func (s *CoreService) ExplainDNS(domain string) (*clash.DNSExplain, error) {
	return s.app.ExplainDNS(domain)
}

// Deprecated: use new AddRule/DeleteRule/SaveRuleSection
func (s *CoreService) GetCustomRules(id string) ([]string, error) {
	return s.app.GetCustomRules(id)
}

// Deprecated: use new AddRule/DeleteRule/SaveRuleSection
func (s *CoreService) SaveCustomRules(id string, rules []string) error {
	return s.app.SaveCustomRules(id, rules)
}

// Deprecated: no longer needed in V2 rule engine
func (s *CoreService) SyncRules(id string) error {
	return s.app.SyncRules(id)
}

func (s *CoreService) GetRulePageData(id string) (clash.RulePageData, error) {
	return s.app.GetRulePageData(id)
}

func (s *CoreService) AddRule(id string, section string, ruleStr string) error {
	return s.app.AddRule(id, section, ruleStr)
}

func (s *CoreService) GetRuleFormOptions(id string) (clash.RuleFormOptions, error) {
	return s.app.GetRuleFormOptions(id)
}

func (s *CoreService) AddRuleFromForm(id string, section string, req clash.BuildRuleRequest) error {
	return s.app.AddRuleFromForm(id, section, req)
}

func (s *CoreService) DeleteRule(id string, section string, index int) error {
	return s.app.DeleteRule(id, section, index)
}

func (s *CoreService) SaveRuleSection(id string, section string, rules []string) error {
	return s.app.SaveRuleSection(id, section, rules)
}

func (s *CoreService) CheckTunEnv() map[string]any {
	return s.app.CheckTunEnv()
}

func (s *CoreService) GetOutboundIP(force bool) (OutboundIPResult, error) {
	return s.app.GetOutboundIP(force)
}

func (s *CoreService) GetOutboundIPForRoute(force bool, expectedRoute string) (OutboundIPResult, error) {
	return s.app.GetOutboundIPForRoute(force, expectedRoute)
}

func (s *CoreService) ReadConfigText(id string) (ConfigTextResult, error) {
	return s.app.ReadConfigText(id)
}

func (s *CoreService) SaveConfigText(id string, content string) error {
	return s.app.SaveConfigText(id, content)
}

func (s *CoreService) ValidateConfigText(content string) error {
	return s.app.ValidateConfigText(content)
}

func (s *CoreService) GetConfigFilePath(id string) string {
	return s.app.GetConfigFilePath(id)
}

func (s *CoreService) IsConfigEditable(id string) bool {
	return s.app.IsConfigEditable(id)
}

func (s *CoreService) GetEditableConfigs() []ConfigTextResult {
	return s.app.GetEditableConfigs()
}

// UpdateService 是「更新任务」转发服务：应用/内核/组件/geo/驱动等更新任务。
// 所有方法仅转发到 *App，签名与 App 上同名方法完全一致。
type UpdateService struct {
	app *App
}

func (s *UpdateService) UpdateCoreComponentAsync() {
	s.app.UpdateCoreComponentAsync()
}

func (s *UpdateService) CheckCoreUpdateAsync() {
	s.app.CheckCoreUpdateAsync()
}

func (s *UpdateService) CheckCoreAgainstSeed() appcore.CoreSeedCheckResult {
	return s.app.CheckCoreAgainstSeed()
}

func (s *UpdateService) ReplaceCoreWithSeed() (string, error) {
	return s.app.ReplaceCoreWithSeed()
}

func (s *UpdateService) DisableCoreComparisonPrompt() error {
	return s.app.DisableCoreComparisonPrompt()
}

func (s *UpdateService) UpdateGeoDatabaseAsync(key string) {
	s.app.UpdateGeoDatabaseAsync(key)
}

func (s *UpdateService) UpdateAllGeoDatabasesAsync() {
	s.app.UpdateAllGeoDatabasesAsync()
}

// ClearFinishedUpdateTasks clears completed, failed, or cancelled tasks
func (s *UpdateService) ClearFinishedUpdateTasks() {
	s.app.ClearFinishedUpdateTasks()
}

func (s *UpdateService) CancelUpdateTask(key string) {
	s.app.CancelUpdateTask(key)
}

func (s *UpdateService) RemoveUpdateTask(key string) {
	s.app.RemoveUpdateTask(key)
}

func (s *UpdateService) ClearUpdateCache(key string) {
	s.app.ClearUpdateCache(key)
}

func (s *UpdateService) DownloadPendingAppUpdateAsync() {
	s.app.DownloadPendingAppUpdateAsync()
}

func (s *UpdateService) CheckAndDownloadAppUpdateAsync() {
	s.app.CheckAndDownloadAppUpdateAsync()
}

func (s *UpdateService) ApplyAppUpdate(path string) error {
	return s.app.ApplyAppUpdate(path)
}

func (s *UpdateService) InstallTunDriverAsync(force bool) {
	s.app.InstallTunDriverAsync(force)
}
