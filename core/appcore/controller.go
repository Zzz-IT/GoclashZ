//go:build windows

package appcore

import (
	"context"
	"fmt"
	"os"

	"goclashz/core/clash"
	"goclashz/core/downloader"
	"goclashz/core/logger"
	"goclashz/core/sys"
	"goclashz/core/tasks"
	"goclashz/core/utils"
	"sync"
	"time"
)

type LogEntry = logger.LogEntry

type AutoDelayRefreshOptions struct {
	Immediate bool
	Reason    string
}

type Options struct {
	Events  EventSink
	Version string
}

type Controller struct {
	events   EventSink
	Behavior *BehaviorStore
	Offline  *OfflineNodeStore
	Tasks    *tasks.Manager
	version  string

	mu                sync.RWMutex
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
	pendingAppUpdateInfo    *downloader.AppUpdateInfo

	GeoUpdates *GeoUpdateManager

	pendingCoreUpdateAssetURL string
	pendingCoreUpdateVersion  string

	// 🚀 核心：静默测速内核生命周期管理
	delayCoreMu          sync.Mutex
	delayCoreRefs        int
	delayCoreStarted     bool
	delayCoreStarting    bool
	delayCoreReady       chan struct{}
	delayCoreStartCancel context.CancelFunc
	delayCoreStartErr    error
	delayCoreStopTimer   *time.Timer

	// 🚀 新增：明确用户运行意图
	userCoreRunning bool
	coreStartedAt   time.Time

	// 🚀 核心：自适应调度状态
	networkMu            sync.RWMutex
	appUpdateDownloading bool
	autoDelayRunning     bool
}

func (c *Controller) setAppUpdateDownloading(active bool) {
	c.networkMu.Lock()
	defer c.networkMu.Unlock()
	c.appUpdateDownloading = active
}

func (c *Controller) isAppUpdateDownloading() bool {
	c.networkMu.RLock()
	defer c.networkMu.RUnlock()
	return c.appUpdateDownloading
}

func (c *Controller) setAutoDelayRunning(active bool) {
	c.networkMu.Lock()
	defer c.networkMu.Unlock()
	c.autoDelayRunning = active
}

func (c *Controller) isAutoDelayRunning() bool {
	c.networkMu.RLock()
	defer c.networkMu.RUnlock()
	return c.autoDelayRunning
}

func NewController(opts Options) *Controller {
	behavior := NewBehaviorStore()
	activeConfig := behavior.Get().ActiveConfig

	c := &Controller{
		events:   opts.Events,
		version:  opts.Version,
		Behavior: behavior,
		Offline:  NewOfflineNodeStore(activeConfig),
		Tasks:    tasks.NewManager(opts.Events),
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

	// Sync startup task state with actual Task Scheduler state
	c.syncStartupTaskState()
}

func (c *Controller) syncStartupTaskState() {
	behavior := c.Behavior.Get()
	actual := sys.CheckStartupTask()
	if behavior.StartupWithOS != actual {
		behavior.StartupWithOS = actual
		_ = c.Behavior.SetAndSave(behavior)
	}
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
	userRunning := c.userCoreRunning
	c.mu.RUnlock()

	// 🚀 核心逻辑：将内核物理运行状态与 UI 业务接管状态解耦
	// 只有用户主动开启代理时，才向前端汇报 true，屏蔽静默测速引发的闪烁
	logicalIsRunning := clash.IsRunning() && userRunning

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
		if item, ok := clash.FindSubIndexByID(activeConfig); ok {
			state.ActiveConfigName = item.Name
			state.ActiveConfigType = item.Type
		}
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
		select {
		case <-ctx.Done():
			clash.Stop()
			c.mu.Lock()
			c.userCoreRunning = false
			c.coreStartedAt = time.Time{}
			c.mu.Unlock()
			return ctx.Err()
		default:
		}

		if _, err := clash.GetInitialDataWithContext(ctx); err == nil {
			apiReady = true
			break
		}

		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			clash.Stop()
			c.mu.Lock()
			c.userCoreRunning = false
			c.coreStartedAt = time.Time{}
			c.mu.Unlock()
			return ctx.Err()
		case <-timer.C:
		}
	}

	if !apiReady {
		clash.Stop() // 探针失败，及时清理掉内核进程
		c.mu.Lock()
		c.userCoreRunning = false
		c.coreStartedAt = time.Time{}
		c.mu.Unlock()
		return fmt.Errorf("内核进程已启动，但 API 未能在预期时间内就绪")
	}

	c.mu.Lock()
	c.userCoreRunning = true
	c.coreStartedAt = time.Now()
	c.mu.Unlock()

	// 🚀 核心：API ready 后，立即回放离线保存的节点选择
	c.applyStoredProxySelections(ctx, activeConfig)
	c.SyncProxyStateOnce()

	return nil
}

func (c *Controller) stopCoreProcessLocked() {
	clash.Stop()
	c.mu.Lock()
	c.userCoreRunning = false
	c.coreStartedAt = time.Time{}
	c.mu.Unlock()
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
	c.userCoreRunning = false
	c.coreStartedAt = time.Time{}
	c.mu.Unlock()

	shouldStopDelayCore := false
	var cancelStart context.CancelFunc
	var ready chan struct{}

	c.delayCoreMu.Lock()
	if c.delayCoreStarted || c.delayCoreStarting {
		shouldStopDelayCore = true
	}

	cancelStart = c.delayCoreStartCancel
	ready = c.delayCoreReady

	c.delayCoreRefs = 0
	c.delayCoreStarted = false
	c.delayCoreStarting = false
	c.delayCoreStartErr = fmt.Errorf("测速已取消")
	c.delayCoreStartCancel = nil
	c.delayCoreReady = nil

	if c.delayCoreStopTimer != nil {
		c.delayCoreStopTimer.Stop()
		c.delayCoreStopTimer = nil
	}
	c.delayCoreMu.Unlock()

	if cancelStart != nil {
		cancelStart()
	}

	if ready != nil {
		close(ready)
	}

	if shouldStopDelayCore {
		clash.Stop()
	}

	c.SyncState()
}

// 🚀 核心新增：静默测速内核保障机制
// 返回值：cleanup 释放引用, warmupRequired 是否处于启动预热窗口, err 错误
func (c *Controller) EnsureDelayCore(ctx context.Context) (cleanup func(), warmupRequired bool, err error) {
	c.delayCoreMu.Lock()

	// 如果有待执行的 idle 停止 timer，先取消
	if c.delayCoreStopTimer != nil {
		c.delayCoreStopTimer.Stop()
		c.delayCoreStopTimer = nil
	}

	// 情况 1：内核已在物理运行
	if clash.IsRunning() {
		c.delayCoreRefs++
		c.delayCoreMu.Unlock()
		warmup := c.NeedsDelayWarmup()
		return c.releaseDelayCore, warmup, nil
	}

	// 情况 2：已有其他协程正在启动内核，挂载并等待
	if c.delayCoreStarting {
		ready := c.delayCoreReady
		c.delayCoreMu.Unlock()

		select {
		case <-ready:
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}

		c.delayCoreMu.Lock()
		defer c.delayCoreMu.Unlock()

		if c.delayCoreStartErr != nil {
			return nil, false, c.delayCoreStartErr
		}

		if !clash.IsRunning() {
			return nil, false, fmt.Errorf("测速内核启动失败")
		}

		c.delayCoreRefs++
		return c.releaseDelayCore, true, nil
	}

	// 情况 3：内核未运行，且本协程是第一个发起启动的
	profileID := c.Behavior.Get().ActiveConfig
	if profileID == "" {
		c.delayCoreMu.Unlock()
		return nil, false, fmt.Errorf("请先在订阅管理中选择并应用一个配置文件")
	}

	startCtx, cancel := context.WithCancel(ctx)

	c.delayCoreStarting = true
	c.delayCoreReady = make(chan struct{})
	c.delayCoreStartCancel = cancel
	ready := c.delayCoreReady
	c.delayCoreStartErr = nil
	c.delayCoreMu.Unlock()

	// 在锁外执行耗时的启动与探测
	startErr := c.startDelayCoreAndWaitReady(startCtx, profileID)

	ok := c.finishDelayCoreStart(ready, startErr)
	if !ok {
		// 如果启动被 DisableAll 抢先取消了
		if startErr == nil && !c.IsUserRunning() {
			clash.Stop()
			c.SyncState()
		}
		return nil, false, fmt.Errorf("测速已取消")
	}

	if startErr != nil {
		return nil, false, startErr
	}

	c.delayCoreMu.Lock()
	c.delayCoreStarted = true
	c.delayCoreRefs = 1
	c.delayCoreMu.Unlock()

	c.mu.Lock()
	c.coreStartedAt = time.Now()
	c.mu.Unlock()

	return c.releaseDelayCore, true, nil
}

func (c *Controller) finishDelayCoreStart(ready chan struct{}, err error) bool {
	c.delayCoreMu.Lock()
	defer c.delayCoreMu.Unlock()

	if c.delayCoreReady != ready {
		return false
	}

	c.delayCoreStarting = false
	c.delayCoreStartErr = err
	c.delayCoreStartCancel = nil
	close(ready)
	c.delayCoreReady = nil

	return true
}

func (c *Controller) startDelayCoreAndWaitReady(ctx context.Context, profileID string) error {
	started := false
	ready := false

	if err := c.StartCoreOnly(ctx, profileID); err != nil {
		return fmt.Errorf("启动测速内核失败: %w", err)
	}
	started = true

	// 🛡️ 核心保障：如果启动失败或中途取消，必须停止这个临时 core
	defer func() {
		if started && !ready && !c.IsUserRunning() {
			clash.Stop()
			c.SyncState()
		}
	}()

	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

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
				ready = true
				return nil
			}
		}
	}
}

const DelayCoreIdleTTL = 15 * time.Second

func (c *Controller) releaseDelayCore() {
	c.delayCoreMu.Lock()

	if c.delayCoreRefs > 0 {
		c.delayCoreRefs--
	}

	shouldScheduleStop := false
	if c.delayCoreRefs == 0 && c.delayCoreStarted && !c.IsUserRunning() {
		shouldScheduleStop = true
	}

	if shouldScheduleStop {
		if c.delayCoreStopTimer != nil {
			c.delayCoreStopTimer.Stop()
		}

		c.delayCoreStopTimer = time.AfterFunc(DelayCoreIdleTTL, func() {
			c.delayCoreMu.Lock()

			if c.delayCoreRefs > 0 || !c.delayCoreStarted || c.IsUserRunning() {
				c.delayCoreMu.Unlock()
				return
			}

			c.delayCoreStarted = false
			c.delayCoreStopTimer = nil
			c.delayCoreMu.Unlock()

			c.mu.Lock()
			c.coreStartedAt = time.Time{}
			c.mu.Unlock()

			clash.Stop()
			c.SyncState()
		})
	}

	c.delayCoreMu.Unlock()
}

func (c *Controller) IsUserRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.userCoreRunning
}

const DelayWarmupWindow = 20 * time.Second

func (c *Controller) NeedsDelayWarmup() bool {
	c.mu.RLock()
	startedAt := c.coreStartedAt
	c.mu.RUnlock()

	if startedAt.IsZero() {
		return false
	}

	return time.Since(startedAt) <= DelayWarmupWindow
}

// StartCoreOnly 严格执行“只启内核，不改状态”
func (c *Controller) StartCoreOnly(ctx context.Context, id string) error {
	behavior := c.Behavior.Get()
	// 仅构建运行时 YAML 并启动进程，不触碰系统代理、TUN 和 userCoreRunning 逻辑
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

		port := 7890
		if netCfg, err := clash.GetNetworkConfig(); err == nil && netCfg != nil {
			if netCfg.MixedPort != 0 {
				port = netCfg.MixedPort
			} else if netCfg.Port != 0 {
				port = netCfg.Port
			}
		}

		err := sys.EnableSystemProxy(
			"127.0.0.1",
			port,
			"localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*;<local>",
		)
		if err != nil {
			c.mu.Lock()
			needCore := c.tunActive
			c.sysProxyActive = false
			if !needCore {
				c.userCoreRunning = false
				c.coreStartedAt = time.Time{}
			}
			c.mu.Unlock()

			if !needCore {
				clash.Stop()
			}

			c.SyncState()
			return fmt.Errorf("设置 Windows 系统代理失败: %w", err)
		}

		c.mu.Lock()
		c.sysProxyActive = true
		c.userCoreRunning = true
		if c.coreStartedAt.IsZero() {
			c.coreStartedAt = time.Now()
		}
		c.mu.Unlock()

		c.SyncState()
		return nil
	}

	_ = sys.DisableSystemProxy()

	c.mu.Lock()
	c.sysProxyActive = false
	needCore := c.tunActive
	if !needCore {
		c.userCoreRunning = false
		c.coreStartedAt = time.Time{}
	}
	c.mu.Unlock()

	if !needCore {
		clash.Stop()
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

	c.stopCoreProcessLocked()

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
		if err := clash.UpdateModeWithContext(ctx, mode); err != nil {
			c.SyncState()
			return fmt.Errorf("内核模式切换失败: %v", err)
		}

		// 🚀 核心修复：切换到全局模式时，主动同步隐藏 GLOBAL
		if mode == "global" {
			c.syncGlobalSelectionOnModeEnter(ctx, behavior.ActiveConfig)
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
		return 7 * time.Second // 冷启动给予 7s 宽限，避开系统启动高峰
	case "restore":
		return 15 * time.Second // 恢复备份后给予 15s 宽限
	case "enabled":
		return 3 * time.Second // 手动开启功能：3 秒
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

	if old.StartupWithOS != next.StartupWithOS {
		c.handleStartupWithOSChange(next.StartupWithOS)
	}

	c.SyncState()
	return nil
}

func (c *Controller) handleStartupWithOSChange(enable bool) {
	exePath, err := os.Executable()
	if err != nil {
		c.events.Emit("app-state-sync", c.GetAppState())
		return
	}

	if enable {
		if err := sys.CreateStartupTask(exePath); err != nil {
			// Roll back on failure
			behavior := c.Behavior.Get()
			behavior.StartupWithOS = false
			_ = c.Behavior.SetAndSave(behavior)
			c.events.Emit("app-state-sync", c.GetAppState())
		}
	} else {
		_ = sys.DeleteStartupTask()
	}
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

// --- Proxy Selection & Sync ---

func (c *Controller) SelectProxyWithModeSync(
	ctx context.Context,
	profileID string,
	mode string,
	groupName string,
	proxyName string,
) error {
	// 🚀 核心修复：全局模式下采取“先校验、双切换、双持久化”的严格路径
	if mode == "global" {
		// 1. 预校验 GLOBAL 是否支持该节点
		if err := c.validateProxySelection(ctx, groupName, proxyName, true); err != nil {
			return fmt.Errorf("全局同步预校验失败: %w", err)
		}

		// 2. 依次切换正常组和 GLOBAL 组
		if err := clash.SelectProxyWithContext(ctx, groupName, proxyName); err != nil {
			return fmt.Errorf("切换代理组失败: %w", err)
		}
		if err := clash.SelectProxyWithContext(ctx, "GLOBAL", proxyName); err != nil {
			return fmt.Errorf("同步全局出口失败: %w", err)
		}

		// 3. 依次记录持久化状态
		c.Offline.Mark(profileID, groupName, proxyName)
		c.Offline.Mark(profileID, "GLOBAL", proxyName)

	} else {
		// 规则/直连模式：仅切换正常组
		if err := clash.SelectProxyWithContext(ctx, groupName, proxyName); err != nil {
			return err
		}
		c.Offline.Mark(profileID, groupName, proxyName)
	}

	c.SyncProxyStateOnce()
	return nil
}

func (c *Controller) validateProxySelection(ctx context.Context, groupName, nodeName string, checkGlobal bool) error {
	if nodeName == "" || nodeName == "DIRECT" || nodeName == "REJECT" {
		return fmt.Errorf("无效的节点名称: %s", nodeName)
	}

	data, err := clash.GetInitialDataWithContext(ctx)
	if err != nil {
		return err
	}

	groups, ok := data["groups"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("无效的代理组数据")
	}

	// 1. 校验目标组是否包含该节点
	if g, ok := groups[groupName].(map[string]interface{}); ok {
		if !proxyGroupContainsNode(g, nodeName) {
			return fmt.Errorf("代理组 %s 不包含节点 %s", groupName, nodeName)
		}
	} else {
		return fmt.Errorf("代理组 %s 不存在", groupName)
	}

	// 2. 可选：校验 GLOBAL 是否包含该节点
	if checkGlobal {
		if g, ok := groups["GLOBAL"].(map[string]interface{}); ok {
			if !proxyGroupContainsNode(g, nodeName) {
				return fmt.Errorf("全局出口 (GLOBAL) 不支持节点 %s", nodeName)
			}
		} else {
			return fmt.Errorf("GLOBAL 组不存在，无法同步")
		}
	}

	return nil
}

func (c *Controller) syncGlobalSelectionOnModeEnter(ctx context.Context, profileID string) {
	// 🚀 核心逻辑：
	// 优先寻找已有的 GLOBAL 持久化记录
	// 如果没有，则寻找一个有效的手动可选组的选择作为兜底同步
	targetNode, exists := c.Offline.Get(profileID, "GLOBAL")

	if !exists {
		// 搜索其他可选组的选择
		selections := c.Offline.Snapshot(profileID)
		for gName, node := range selections {
			if gName == "GLOBAL" {
				continue
			}
			// 如果找到了一个节点，且它在 GLOBAL 中合法，就用它
			if err := c.selectGlobalProxyIfValid(ctx, node); err == nil {
				c.Offline.Mark(profileID, "GLOBAL", node)
				return
			}
		}
		return
	}

	// 执行同步
	if err := c.selectGlobalProxyIfValid(ctx, targetNode); err != nil {
		fmt.Printf("进入全局模式同步失败: %v\n", err)
	}
}

func (c *Controller) SelectOfflineProxyWithModeSync(
	profileID string,
	mode string,
	groupName string,
	proxyName string,
) {
	c.Offline.Mark(profileID, groupName, proxyName)

	if mode == "global" {
		c.Offline.Mark(profileID, "GLOBAL", proxyName)
	}
}

func (c *Controller) selectGlobalProxyIfValid(ctx context.Context, proxyName string) error {
	if proxyName == "" || proxyName == "DIRECT" || proxyName == "REJECT" {
		return fmt.Errorf("无效的全局出口节点: %s", proxyName)
	}

	data, err := clash.GetInitialDataWithContext(ctx)
	if err != nil {
		return err
	}

	groups, ok := data["groups"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("内核代理组数据无效")
	}

	raw, ok := groups["GLOBAL"]
	if !ok {
		return fmt.Errorf("GLOBAL 组不存在")
	}

	globalGroup, ok := raw.(map[string]interface{})
	if !ok {
		return fmt.Errorf("GLOBAL 组数据无效")
	}

	if !proxyGroupContainsNode(globalGroup, proxyName) {
		return fmt.Errorf("GLOBAL 不包含节点: %s", proxyName)
	}

	return clash.SelectProxyWithContext(ctx, "GLOBAL", proxyName)
}

func (c *Controller) applyStoredProxySelections(ctx context.Context, profileID string) {
	selected := c.Offline.Snapshot(profileID)
	if len(selected) == 0 {
		return
	}

	data, err := clash.GetInitialDataWithContext(ctx)
	if err != nil {
		fmt.Printf("读取内核代理组失败，跳过节点选择回放: %v\n", err)
		return
	}

	groups, ok := data["groups"].(map[string]interface{})
	if !ok {
		return
	}

	// 1. 先回放正常可选组
	for groupName, nodeName := range selected {
		if groupName == "GLOBAL" {
			continue
		}

		raw, ok := groups[groupName]
		if !ok {
			continue
		}

		group, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		groupType, _ := group["type"].(string)
		if !isUserSelectableProxyGroupType(groupType) {
			continue
		}

		if !proxyGroupContainsNode(group, nodeName) {
			continue
		}

		_ = clash.SelectProxyWithContext(ctx, groupName, nodeName)
	}

	// 2. 全局模式下回放隐藏 GLOBAL
	behavior := c.Behavior.Get()
	if behavior.ActiveMode == "global" {
		if globalNode, ok := selected["GLOBAL"]; ok && globalNode != "" {
			if err := c.selectGlobalProxyIfValid(ctx, globalNode); err != nil {
				fmt.Printf("回放 GLOBAL 出口失败: %v\n", err)
			}
		}
	}
}

func (c *Controller) SyncProxyStateOnce() {
	if c.proxyState != nil {
		c.proxyState.SyncOnce()
	}
}

func isUserSelectableProxyGroupType(t string) bool {
	switch t {
	case "Selector", "Fallback":
		return true
	default:
		return false
	}
}

func proxyGroupContainsNode(group map[string]interface{}, nodeName string) bool {
	all, ok := group["all"].([]interface{})
	if !ok {
		return false
	}
	for _, v := range all {
		if s, ok := v.(string); ok && s == nodeName {
			return true
		}
	}
	return false
}
