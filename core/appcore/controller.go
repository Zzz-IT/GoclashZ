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

type Options struct {
	Events       EventSink
	Version      string
	RunDelayTest func() // 🚀 新增：自动测速回调
}

type Controller struct {
	events       EventSink
	Behavior     *BehaviorStore
	Offline      *OfflineNodeStore
	Tasks        *tasks.Manager
	version      string
	runDelayTest func()

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
}

func NewController(opts Options) *Controller {
	c := &Controller{
		events:       opts.Events,
		version:      opts.Version,
		runDelayTest: opts.RunDelayTest,
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
	c.RefreshAutoDelayTest()

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
			c.traffic.Start(c.ctx, clash.APIURL("/traffic"))
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

	c.SyncState()
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
func (c *Controller) RefreshAutoDelayTest() {
	c.autoTestMu.Lock()
	defer c.autoTestMu.Unlock()

	if c.autoTestQuit != nil {
		close(c.autoTestQuit)
		c.autoTestQuit = nil
	}

	behavior := c.Behavior.Get()
	if !behavior.AutoDelayTest || behavior.AutoDelayTestInterval <= 0 {
		return
	}

	c.autoTestQuit = make(chan struct{})
	go func(quit chan struct{}, intervalMin int) {
		ticker := time.NewTicker(time.Duration(intervalMin) * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				if clash.IsRunning() && c.runDelayTest != nil {
					// 🚀 核心修复：连通自动测速回调
					c.runDelayTest()
				}
			}
		}
	}(c.autoTestQuit, behavior.AutoDelayTestInterval)
}

func (c *Controller) SyncTrafficStream(ctx context.Context) {
	state := c.GetAppState()
	if state.IsRunning {
		c.traffic.Start(ctx, clash.APIURL("/traffic"))
	} else {
		c.traffic.Stop()
	}
}

func (c *Controller) StopTrafficStream() {
	c.traffic.Stop()
}

func (c *Controller) RestartTrafficStream(ctx context.Context) {
	c.traffic.Restart(ctx, clash.APIURL("/traffic"))
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
