//go:build windows

package appcore

import (
	"context"
	"fmt"
	"goclashz/core/clash"
	"goclashz/core/sys"
	"goclashz/core/tasks"
	"goclashz/core/utils"
	"goclashz/core/logger"
	"sync"
	"time"
)

type LogEntry = logger.LogEntry

type AutoDelayRefreshOptions struct {
	Immediate bool
	Reason    string
}

type Options struct {
	Events       EventSink
	Version      string
}

type Controller struct {
	events       EventSink
	Behavior     *BehaviorStore
	Offline      *OfflineNodeStore
	Tasks        *tasks.Manager
	version      string

	mu              sync.RWMutex
	coreLifecycleMu   sync.Mutex
	componentUpdateMu sync.Mutex
	sysProxyActive    bool
	tunActive         bool

	// 自动测速任务控制
	autoTestQuit chan struct{}
	autoTestMu   sync.Mutex

	traffic     *TrafficStreamManager
	logs        *LogStreamManager
	Delay       *DelayTestManager
	proxyState  *ProxyStateMonitor
	connections *ConnectionMonitorManager
	ctx         context.Context

	updateReady   bool
	newAppVersion string

	downloadedUpdatePath    string
	downloadedUpdateVersion string

	GeoUpdates *GeoUpdateManager

	pendingCoreUpdateAssetURL string
	pendingCoreUpdateVersion  string

	// 🚀 核心：静默测速内核生命周期管理
	delayCoreMu       sync.Mutex
	delayCoreRefs     int
	delayCoreStarted  bool
	delayCoreStarting bool
	delayCoreReady    chan struct{}
	delayCoreStartErr error
}

func NewController(opts Options) *Controller {
	c := &Controller{
		events:       opts.Events,
		version:      opts.Version,
		Behavior:     NewBehaviorStore(),
		Offline:      NewOfflineNodeStore(),
		Tasks:        tasks.NewManager(opts.Events),
	}
	c.traffic = NewTrafficStreamManager(opts.Events, func() string {
		return c.Behavior.Get().LogLevel
	})
	c.logs = NewLogStreamManager(opts.Events)
	c.Delay = NewDelayTestManager(opts.Events, c)
	c.proxyState = NewProxyStateMonitor(opts.Events)
	c.connections = NewConnectionMonitorManager(opts.Events)

	c.GeoUpdates = NewGeoUpdateManager(opts.Events, c.updateGeoDatabase)

	return c
}

func (c *Controller) Startup(ctx context.Context) {
	c.ctx = ctx
	CleanLegacyFiles(c.version)
	c.RefreshAutoDelayTest(AutoDelayRefreshOptions{
		Immediate: true,
		Reason:    "startup",
	})
	c.RefreshAppAutoUpdate()

	// 🚀 接入内核退出回调：感知底层进程的非预期崩溃
	clash.SetOnExitCallback(func(e clash.ExitEvent) {
		if !e.Intentional {
			c.mu.Lock()
			wasSysProxy := c.sysProxyActive
			c.sysProxyActive = false
			c.tunActive = false
			c.mu.Unlock()

			// 🛡️ 核心修复：如果崩溃前开启了系统代理，必须强制关闭以防断网
			if wasSysProxy {
				_ = sys.DisableSystemProxy()
			}

			c.events.Emit("clash-exited", e.Message)
			c.SyncState()
		}
	})
}

func (c *Controller) GetEvents() EventSink {
	return c.events
}

// AppState 定义全局状态同步结构
type AppState struct {
	IsRunning bool   `json:"isRunning"`
	Mode      string `json:"mode"`
	Theme     string `json:"theme"`
	HideLogs  bool   `json:"hideLogs"`
	// 👇 新增以下字段，统一接管 UI
	SystemProxy bool   `json:"systemProxy"`
	Tun         bool   `json:"tun"`
	Version     string `json:"version"`
	AppVersion  string `json:"appVersion"` // 👈 新增：应用版本
	// 🚀 新增：让前端实时知道当前在跑哪个配置
	ActiveConfig     string `json:"activeConfig"`
	ActiveConfigName string `json:"activeConfigName"`
	ActiveConfigType string `json:"activeConfigType"`
	// 👇 新增：延迟保留相关
	DelayRetention     bool   `json:"delayRetention"`
	DelayRetentionTime string `json:"delayRetentionTime"`

	// 👇 新增：应用更新相关
	UpdateReady      bool   `json:"updateReady"`
	NewAppVersion    string `json:"newAppVersion"`
	UpdateDownloaded bool   `json:"updateDownloaded"`
	DownloadedPath   string `json:"downloadedPath"`
}

// GetAppState 获取应用运行状态快照
func (c *Controller) GetAppState() AppState {
	behavior := c.Behavior.Get()
	activeConfig := behavior.ActiveConfig

	c.mu.RLock()
	sysProxy := c.sysProxyActive
	tunActive := c.tunActive
	c.mu.RUnlock()

	// 🚀 核心逻辑：将内核物理运行状态与 UI 业务接管状态解耦
	// 只有系统代理或 TUN 开启时，才向前端汇报 true，屏蔽静默测速引发的闪烁
	logicalIsRunning := clash.IsRunning() && (sysProxy || tunActive)

	state := AppState{
		IsRunning:          logicalIsRunning,
		Mode:               behavior.ActiveMode,
		SystemProxy:        sysProxy,
		Tun:                tunActive,
		Theme:              utils.GetGlobalTheme(),
		Version:            clash.GetLocalCoreVersion(c.ctx),
		AppVersion:         c.version,
		ActiveConfig:       activeConfig,
		DelayRetention:     behavior.DelayRetention,
		DelayRetentionTime: behavior.DelayRetentionTime,
		HideLogs:           behavior.HideLogs,
		UpdateReady:        c.updateReady,
		NewAppVersion:      c.newAppVersion,
		UpdateDownloaded:   c.downloadedUpdatePath != "",
		DownloadedPath:     c.downloadedUpdatePath,
	}

	if activeConfig != "" {
		state.ActiveConfigName = activeConfig
		clash.IndexLock.RLock()
		for _, item := range clash.SubIndex {
			if item.ID == activeConfig {
				state.ActiveConfigName = item.Name
				state.ActiveConfigType = item.Type
				break
			}
		}
		clash.IndexLock.RUnlock()
	}

	return state
}

// SyncState 触发状态同步事件
func (c *Controller) SyncState() {
	state := c.GetAppState()
	c.events.Emit("app-state-sync", state)

	// 🚀 核心修复：Controller 自助管理流量流
	if c.ctx != nil {
		if state.IsRunning {
			behavior := c.Behavior.Get()
			c.traffic.Start(c.ctx, clash.APIURL("/traffic"), behavior.ProxyTrafficOnly)
			c.proxyState.Start(c.ctx)
		} else {
			c.traffic.Stop()
			c.proxyState.Stop()
		}
	}
}

// --- 内部方法 (需要提前持有 coreLifecycleMu) ---

func (c *Controller) ensureCoreRunningLocked(ctx context.Context) error {
	if clash.IsRunning() {
		return nil
	}

	behavior := c.Behavior.Get()
	activeConfig := behavior.ActiveConfig
	if activeConfig == "" {
		return fmt.Errorf("no active config selected")
	}

	err := clash.BuildRuntimeConfig(activeConfig, behavior.ActiveMode, behavior.LogLevel)
	if err != nil {
		return err
	}

	if err := clash.Start(ctx); err != nil {
		return err
	}

	// 🛡️ 核心修复：API 探针带超时判定，失败必须报错并清理僵尸进程
	apiReady := false
	for i := 0; i < 20; i++ {
		if _, err := clash.GetInitialData(); err == nil {
			apiReady = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !apiReady {
		clash.Stop() // 探针失败，及时清理掉内核进程
		return fmt.Errorf("内核进程已启动，但 API 未能在预期时间内就绪")
	}
	return nil
}

func (c *Controller) stopCoreProcessLocked() error {
	clash.Stop()
	return nil
}

// StopCoreProcessIfIdle 静默测速结束后，只有在用户未显式开启代理/TUN 时才停止内核
func (c *Controller) StopCoreProcessIfIdle() {
	c.coreLifecycleMu.Lock()
	defer c.coreLifecycleMu.Unlock()

	c.mu.RLock()
	needCore := c.sysProxyActive || c.tunActive
	c.mu.RUnlock()

	if needCore {
		return
	}

	c.stopCoreProcessLocked()
	c.SyncState()
}

// --- 导出方法 ---

// EnsureCoreRunning 确保内核已启动并就绪 (带锁包装)
func (c *Controller) EnsureCoreRunning(ctx context.Context) error {
	c.coreLifecycleMu.Lock()
	defer c.coreLifecycleMu.Unlock()
	return c.ensureCoreRunningLocked(ctx)
}

// StopCoreService 停止内核并清理所有运行时状态（通常用于程序退出）
func (c *Controller) StopCoreService() {
	c.DisableAll()
}

// DisableAll 彻底清理并关闭所有功能
func (c *Controller) DisableAll() {
	c.coreLifecycleMu.Lock()
	defer c.coreLifecycleMu.Unlock()

	c.stopCoreProcessLocked()
	_ = sys.DisableSystemProxy()

	c.mu.Lock()
	c.sysProxyActive = false
	c.tunActive = false
	c.mu.Unlock()

	shouldStopDelayCore := false

	c.delayCoreMu.Lock()
	if c.delayCoreStarted || c.delayCoreStarting {
		shouldStopDelayCore = true
	}

	c.delayCoreRefs = 0
	c.delayCoreStarted = false
	c.delayCoreStarting = false
	c.delayCoreStartErr = nil

	if c.delayCoreReady != nil {
		// 这里不 close，因为 starting 协程可能正在等它。
		// 但由于 we set starting = false, EnsureDelayCore 会直接退出。
		// 实际上 close 是安全的，因为 ready 只是个信号。
		close(c.delayCoreReady)
		c.delayCoreReady = nil
	}
	c.delayCoreMu.Unlock()

	if shouldStopDelayCore {
		clash.Stop()
	}

	c.SyncState()
}

// 🚀 核心新增：静默测速内核保障机制
func (c *Controller) EnsureDelayCore(ctx context.Context) (func(), error) {
	c.delayCoreMu.Lock()

	// 情况 1：内核已在物理运行（无论是正式启用还是之前的静默测速）
	if clash.IsRunning() {
		c.delayCoreRefs++
		c.delayCoreMu.Unlock()
		return c.releaseDelayCore, nil
	}

	// 情况 2：已有其他协程正在启动内核，挂载并等待
	if c.delayCoreStarting {
		ready := c.delayCoreReady
		c.delayCoreMu.Unlock()

		select {
		case <-ready:
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		c.delayCoreMu.Lock()
		defer c.delayCoreMu.Unlock()

		if c.delayCoreStartErr != nil {
			return nil, c.delayCoreStartErr
		}

		if !clash.IsRunning() {
			return nil, fmt.Errorf("测速内核启动失败")
		}

		c.delayCoreRefs++
		return c.releaseDelayCore, nil
	}

	// 情况 3：内核未运行，且本协程是第一个发起启动的
	profileID := c.Behavior.Get().ActiveConfig
	if profileID == "" {
		c.delayCoreMu.Unlock()
		return nil, fmt.Errorf("请先在订阅管理中选择并应用一个配置文件")
	}

	c.delayCoreStarting = true
	c.delayCoreReady = make(chan struct{})
	ready := c.delayCoreReady
	c.delayCoreStartErr = nil
	c.delayCoreMu.Unlock()

	// 在锁外执行耗时的启动与探测
	err := c.startDelayCoreAndWaitReady(ctx, profileID)

	c.delayCoreMu.Lock()
	defer c.delayCoreMu.Unlock()

	c.delayCoreStarting = false
	c.delayCoreStartErr = err
	close(ready)

	if err != nil {
		return nil, err
	}

	c.delayCoreStarted = true
	c.delayCoreRefs = 1

	return c.releaseDelayCore, nil
}

func (c *Controller) startDelayCoreAndWaitReady(ctx context.Context, profileID string) error {
	if err := c.StartCoreOnly(ctx, profileID); err != nil {
		return fmt.Errorf("启动测速内核失败: %w", err)
	}

	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	// 使用更短的 3s 探测超时
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-timeout.C:
			return fmt.Errorf("测速内核启动超时")

		case <-ticker.C:
			if _, err := clash.GetInitialData(); err == nil {
				return nil
			}
		}
	}
}

func (c *Controller) releaseDelayCore() {
	shouldStop := false

	c.delayCoreMu.Lock()

	if c.delayCoreRefs > 0 {
		c.delayCoreRefs--
	}

	// 还有其他测速任务正在使用，不退出
	if c.delayCoreRefs == 0 && c.delayCoreStarted {
		// 🚀 核心判定：只有在用户没有正式开启代理时，才静默杀掉 core
		if !c.IsUserRunning() {
			shouldStop = true
		}
		c.delayCoreStarted = false
	}

	c.delayCoreMu.Unlock()

	// 锁外执行 IO 与同步逻辑，避免死锁
	if shouldStop {
		clash.Stop()
		c.SyncState()
	}
}

func (c *Controller) IsUserRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sysProxyActive || c.tunActive
}

// StartCoreOnly 严格执行“只启内核，不改状态”
func (c *Controller) StartCoreOnly(ctx context.Context, id string) error {
	behavior := c.Behavior.Get()
	// 仅构建运行时 YAML 并启动进程，不触碰系统代理、TUN 和 isRunning 逻辑
	if err := clash.BuildRuntimeConfig(id, behavior.ActiveMode, behavior.LogLevel); err != nil {
		return err
	}
	return clash.Start(ctx)
}

func (c *Controller) GetActiveProfilePath() string {
	id := c.Behavior.Get().ActiveConfig
	if id == "" {
		return ""
	}
	// 这里直接复用 clash 包里的逻辑
	return clash.GetConfigPath()
}

// StopCoreProcess 仅停止物理进程，保留用户开启意图
func (c *Controller) StopCoreProcess() {
	c.coreLifecycleMu.Lock()
	defer c.coreLifecycleMu.Unlock()
	c.stopCoreProcessLocked()
}

// ToggleSystemProxy 开关：系统代理
func (c *Controller) ToggleSystemProxy(ctx context.Context, enable bool) error {
	c.coreLifecycleMu.Lock()
	defer c.coreLifecycleMu.Unlock()

	if enable {
		behavior := c.Behavior.Get()
		if behavior.ActiveConfig == "" {
			return fmt.Errorf("请先选择一个订阅配置")
		}
		if err := c.ensureCoreRunningLocked(ctx); err != nil {
			return err
		}
		if !clash.IsRunning() {
			return fmt.Errorf("内核未能成功启动，系统代理开启失败")
		}

		// 获取实际运行端口并开启系统代理
		var port int
		if netCfg, err := clash.GetNetworkConfig(); err == nil {
			port = netCfg.MixedPort
			if port == 0 {
				port = netCfg.Port
			}
		}
		if port == 0 {
			port = 7890 // 兜底
		}

		if err := sys.EnableSystemProxy("127.0.0.1", port, "localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*;<local>"); err != nil {
			return fmt.Errorf("设置 Windows 系统代理失败: %v", err)
		}
	} else {
		_ = sys.DisableSystemProxy()
	}

	c.mu.Lock()
	c.sysProxyActive = enable
	needCore := c.sysProxyActive || c.tunActive
	c.mu.Unlock()

	if !needCore {
		c.stopCoreProcessLocked()
	}
	c.SyncState()
	return nil
}

// ToggleTunMode 开关：TUN 模式
func (c *Controller) ToggleTunMode(ctx context.Context, enable bool) error {
	c.coreLifecycleMu.Lock()
	defer c.coreLifecycleMu.Unlock()

	if enable {
		if !sys.IsWintunInstalled() {
			return fmt.Errorf("缺失 Wintun 驱动，请先安装")
		}
		if !sys.CheckAdmin() {
			c.events.Emit("notify-error", "TUN 模式必须以管理员身份运行")
			return fmt.Errorf("permission denied")
		}
	}

	// 🛡️ 核心修复：遵循“先写配置、改状态，最后统一重启一次”的原子化路径，杜绝启动抖动
	tunCfg, _ := clash.GetTunConfig()
	if tunCfg == nil {
		tunCfg = &clash.TunConfig{Stack: "gvisor", AutoRoute: true, StrictRoute: true}
	}
	tunCfg.Enable = enable
	if err := clash.UpdateTunConfig(tunCfg); err != nil {
		return err
	}

	c.mu.Lock()
	c.tunActive = enable
	needCore := c.sysProxyActive || c.tunActive
	c.mu.Unlock()

	// 无论开启还是关闭，只要影响了 TUN 配置，就需要重启内核来应用
	c.stopCoreProcessLocked()
	if needCore {
		if err := c.ensureCoreRunningLocked(ctx); err != nil {
			// 🛡️ 核心修复：开启失败时回滚状态
			if enable {
				c.mu.Lock()
				c.tunActive = false
				c.mu.Unlock()

				tunCfg.Enable = false
				_ = clash.UpdateTunConfig(tunCfg)
			}
			c.SyncState()
			return err
		}
	}

	c.events.Emit("core-restarted", map[string]any{"reason": "internal"})
	c.SyncState()
	return nil
}

// RestartCore 重启内核（默认为内部调用原因）
func (c *Controller) RestartCore(ctx context.Context) error {
	return c.RestartCoreWithReason(ctx, "internal")
}

// RestartCoreWithReason 重启内核并携带显式原因，决定是否触发缓存清理
func (c *Controller) RestartCoreWithReason(ctx context.Context, reason string) error {
	c.coreLifecycleMu.Lock()
	defer c.coreLifecycleMu.Unlock()

	_ = c.stopCoreProcessLocked()

	if err := c.ensureCoreRunningLocked(ctx); err != nil {
		c.SyncState()
		return err
	}

	// 发送通用的重启信号
	c.events.Emit("core-restarted", map[string]any{
		"reason": reason,
	})

	// 🛡️ 核心改进：仅在用户明确意图或核心配置变更时，才通知前端清理延迟缓存
	switch reason {
	case "manual", "config-switch", "subscription-update", "restore":
		c.events.Emit("delay-cache-clear", reason)
	}

	c.SyncState()
	return nil
}

// UpdateClashMode 切换 Clash 路由模式
func (c *Controller) UpdateClashMode(ctx context.Context, mode string) error {
	behavior := c.Behavior.Get()
	if behavior.ActiveMode == mode {
		return nil
	}

	// 1. 更新配置并写盘
	behavior.ActiveMode = mode
	if err := c.Behavior.SetAndSave(behavior); err != nil {
		c.events.Emit("notify-error", "模式持久化保存失败: "+err.Error())
	}

	// 2. 如果内核正在运行，尝试通过 API 热切换
	if clash.IsRunning() {
		if err := clash.UpdateMode(mode); err != nil {
			c.SyncState()
			return fmt.Errorf("内核模式切换失败: %v", err)
		}
	}

	c.SyncState()
	return nil
}

// RefreshAutoDelayTest 刷新定时测速任务
func (c *Controller) RefreshAutoDelayTest(opts AutoDelayRefreshOptions) {
	c.autoTestMu.Lock()

	if c.autoTestQuit != nil {
		close(c.autoTestQuit)
		c.autoTestQuit = nil
	}

	behavior := c.Behavior.Get()
	if !behavior.AutoDelayTest {
		c.autoTestMu.Unlock()
		return
	}

	intervalMin := behavior.AutoDelayTestInterval
	if intervalMin <= 0 {
		intervalMin = 60
	}

	quit := make(chan struct{})
	c.autoTestQuit = quit
	c.autoTestMu.Unlock()

	if opts.Immediate {
		delay := autoDelayInitialDelay(opts.Reason)
		go c.runAutoDelayTestOnceDelayed(quit, opts.Reason, delay)
	}

	go func(quit <-chan struct{}, intervalMin int) {
		ticker := time.NewTicker(time.Duration(intervalMin) * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-quit:
				return

			case <-ticker.C:
				c.runAutoDelayTestOnce("scheduled")
			}
		}
	}(quit, intervalMin)
}

func autoDelayInitialDelay(reason string) time.Duration {
	switch reason {
	case "startup":
		return 20 * time.Second // 冷启动给予 20s 宽限，避开系统启动高峰
	case "restore":
		return 30 * time.Second // 恢复备份后给予 30s 宽限
	case "enabled":
		return 3 * time.Second  // 手动开启功能：3 秒
	default:
		return 5 * time.Second
	}
}

func (c *Controller) runAutoDelayTestOnceDelayed(
	quit <-chan struct{},
	reason string,
	delay time.Duration,
) {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-quit:
		return

	case <-timer.C:
		c.runAutoDelayTestOnce(reason)
	}
}

func (c *Controller) runAutoDelayTestOnce(reason string) {
	if c.ctx == nil {
		return
	}

	behavior := c.Behavior.Get()
	if !behavior.AutoDelayTest {
		return
	}

	logger.Infof("Triggering auto delay test, reason: %s", reason)
	go c.Delay.TestAllProxiesAuto(c.ctx, reason)
}

func (c *Controller) RefreshAppAutoUpdate() {
	if c.ctx == nil {
		return
	}

	behavior := c.Behavior.Get()
	if !behavior.AutoUpdate {
		return
	}

	now := time.Now().Unix()
	shouldCheck := false

	switch behavior.UpdateMethod {
	case "startup":
		shouldCheck = true

	case "scheduled":
		intervalDays := behavior.UpdateInterval
		if intervalDays <= 0 {
			intervalDays = 1
		}

		if behavior.LastUpdateCheck == 0 ||
			now-behavior.LastUpdateCheck >= int64(intervalDays)*24*3600 {
			shouldCheck = true
		}

	default:
		shouldCheck = true
	}

	if !shouldCheck {
		return
	}

	// 标记已检查，防止由于配置保存触发的副作用导致死循环
	behavior.LastUpdateCheck = now
	_ = c.Behavior.SetAndSave(behavior)

	go c.AutoCheckAndDownloadAppUpdateAsync(c.ctx, c.version)
}

func (c *Controller) SaveAppBehavior(b AppBehavior) error {
	old := c.Behavior.Get()

	if err := c.Behavior.SetAndSave(b); err != nil {
		return err
	}

	next := c.Behavior.Get()

	// 副作用调度
	if old.AutoDelayTest != next.AutoDelayTest ||
		old.AutoDelayTestInterval != next.AutoDelayTestInterval {
		c.RefreshAutoDelayTest(AutoDelayRefreshOptions{
			Immediate: !old.AutoDelayTest && next.AutoDelayTest,
			Reason:    "enabled",
		})
	}

	if old.AutoUpdate != next.AutoUpdate ||
		old.UpdateMethod != next.UpdateMethod ||
		old.UpdateInterval != next.UpdateInterval {
		c.RefreshAppAutoUpdate()
	}

	if old.LogLevel != next.LogLevel {
		c.StartLogStream(c.ctx)
	}

	if old.ProxyTrafficOnly != next.ProxyTrafficOnly {
		c.traffic.Stop()
		c.traffic.ResetRuntimeState()
		c.events.Emit("traffic-stat-mode-changed", next.ProxyTrafficOnly)
	}

	c.SyncState()
	return nil
}

func (c *Controller) SyncTrafficStream(ctx context.Context) {
	state := c.GetAppState()
	if state.IsRunning {
		behavior := c.Behavior.Get()
		c.traffic.Start(ctx, clash.APIURL("/traffic"), behavior.ProxyTrafficOnly)
	} else {
		c.traffic.Stop()
	}
}

func (c *Controller) StopTrafficStream() {
	c.traffic.Stop()
}

func (c *Controller) RestartTrafficStream(ctx context.Context) {
	behavior := c.Behavior.Get()
	c.traffic.Restart(ctx, clash.APIURL("/traffic"), behavior.ProxyTrafficOnly)
}

func (c *Controller) ResetTrafficTotals() {
	c.traffic.ResetRuntimeState()
}

// --- 日志流调度 ---

func (c *Controller) StartLogStream(ctx context.Context) {
	logLevel := c.Behavior.Get().LogLevel
	if logLevel == "" {
		logLevel = "info"
	}
	c.logs.Start(ctx, logLevel)
}

func (c *Controller) StopLogStream() {
	c.logs.Stop()
}

func (c *Controller) GetRecentLogs() []logger.LogEntry {
	return logger.AppLogs.GetAll()
}

func (c *Controller) SearchLogs(keyword string) []logger.LogEntry {
	return logger.AppLogs.Search(keyword)
}

func (c *Controller) ClearLogs() {
	logger.AppLogs.Clear()
}

func (c *Controller) IsLogStreaming() bool {
	return c.logs.IsRunning()
}

func (c *Controller) GetConnections() (ConnectionsSnapshot, error) {
	return c.connections.GetSnapshot()
}

func (c *Controller) StartConnectionMonitor(ctx context.Context) {
	c.connections.Start(ctx)
}

func (c *Controller) StopConnectionMonitor() {
	c.connections.Stop()
}

func (c *Controller) AppUpdateStatus() (bool, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.updateReady, c.newAppVersion
}

func (c *Controller) SetUpdateStatus(ready bool, version string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updateReady = ready
	c.newAppVersion = version
}

func (c *Controller) SetDownloadedAppUpdate(path, version string) {
	c.mu.Lock()
	changed := c.downloadedUpdatePath != path || c.downloadedUpdateVersion != version
	c.downloadedUpdatePath = path
	c.downloadedUpdateVersion = version
	c.mu.Unlock()

	if changed {
		c.SyncState()
	}
}

func (c *Controller) GetDownloadedUpdate() (string, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.downloadedUpdatePath, c.downloadedUpdateVersion
}
