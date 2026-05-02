package appcore

import (
	"context"
	"goclashz/core/clash"
	"goclashz/core/sys"
	"goclashz/core/tasks"
	"sync"
	"time"
)

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

	mu              sync.RWMutex
	coreLifecycleMu sync.Mutex
	sysProxyActive  bool
	tunActive       bool

	// 自动测速任务控制
	autoTestQuit chan struct{}
	autoTestMu   sync.Mutex
}

func NewController(opts Options) *Controller {
	return &Controller{
		events:   opts.Events,
		version:  opts.Version,
		Behavior: NewBehaviorStore(),
		Offline:  NewOfflineNodeStore(),
		Tasks:    tasks.NewManager(opts.Events),
	}
}

func (c *Controller) Startup(ctx context.Context) {
	CleanLegacyFiles(c.version)
	c.RefreshAutoDelayTest()
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
		AppVersion:         c.version,
		ActiveConfig:       activeConfig,
		DelayRetention:     behavior.DelayRetention,
		DelayRetentionTime: behavior.DelayRetentionTime,
		HideLogs:           behavior.HideLogs,
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
	c.events.Emit(EventStateSync, c.GetAppState())
}

// EnsureCoreRunning 确保内核已启动并就绪
func (c *Controller) EnsureCoreRunning(ctx context.Context) error {
	c.coreLifecycleMu.Lock()
	defer c.coreLifecycleMu.Unlock()

	if clash.IsRunning() {
		return nil
	}

	behavior := c.Behavior.Get()
	activeConfig := behavior.ActiveConfig
	if activeConfig == "" {
		return nil
	}

	err := clash.BuildRuntimeConfig(activeConfig, behavior.ActiveMode, behavior.LogLevel)
	if err != nil {
		return err
	}

	if err := clash.Start(ctx); err != nil {
		return err
	}

	// 探针等待 API 就绪
	for i := 0; i < 20; i++ {
		if _, err := clash.GetInitialData(); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

// StopCoreService 停止内核并清理状态
func (c *Controller) StopCoreService() {
	c.coreLifecycleMu.Lock()
	defer c.coreLifecycleMu.Unlock()

	clash.Stop()
	c.mu.Lock()
	c.sysProxyActive = false
	c.tunActive = false
	c.mu.Unlock()
}

// ToggleSystemProxy 开关：系统代理
func (c *Controller) ToggleSystemProxy(ctx context.Context, enable bool) error {
	c.mu.Lock()
	if c.sysProxyActive == enable {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	if enable {
		if err := c.EnsureCoreRunning(ctx); err != nil {
			return err
		}
		
		netCfg, err := clash.GetNetworkConfig()
		if err != nil {
			return err
		}
		port := netCfg.MixedPort
		if port == 0 {
			port = netCfg.Port
		}
		
		if err := sys.EnableSystemProxy("127.0.0.1", port, "localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*;<local>"); err != nil {
			return err
		}
	} else {
		_ = sys.DisableSystemProxy()
	}

	c.mu.Lock()
	c.sysProxyActive = enable
	c.mu.Unlock()
	c.SyncState()
	return nil
}

// ToggleTunMode 开关：TUN 模式
func (c *Controller) ToggleTunMode(ctx context.Context, enable bool) error {
	c.mu.Lock()
	if c.tunActive == enable {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	if enable {
		// TUN 模式需要内核重新以管理员权限启动或配置变更
		// 简化逻辑：如果是开启，确保内核在跑
		if err := c.EnsureCoreRunning(ctx); err != nil {
			return err
		}
	} else {
		// 关闭 TUN 模式通常需要重启内核以卸载 Wintun 网卡
		// 这里简化处理
	}

	c.mu.Lock()
	c.tunActive = enable
	c.mu.Unlock()
	c.SyncState()
	return nil
}

// RestartCore 重启内核
func (c *Controller) RestartCore(ctx context.Context) error {
	c.StopCoreService()
	return c.EnsureCoreRunning(ctx)
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
		c.events.Emit(EventNotifyError, "模式持久化保存失败: "+err.Error())
	}

	// 2. 即刻同步 UI
	c.SyncState()

	// 3. 通知内核或预置配置
	if clash.IsRunning() {
		if err := clash.UpdateMode(mode); err != nil {
			// 如果内核正在运行但更新模式失败，可能是 API 断连
		}
	} else {
		activeCfg := behavior.ActiveConfig
		if activeCfg != "" {
			_ = clash.BuildRuntimeConfig(activeCfg, mode, behavior.LogLevel)
		}
	}

	// 再次同步以确认
	c.SyncState()
	return nil
}

// RefreshAutoDelayTest 刷新定时测速任务
func (c *Controller) RefreshAutoDelayTest() {
	c.autoTestMu.Lock()
	defer c.autoTestMu.Unlock()

	// 1. 停止旧任务
	if c.autoTestQuit != nil {
		close(c.autoTestQuit)
		c.autoTestQuit = nil
	}

	// 2. 读取配置
	behavior := c.Behavior.Get()
	if !behavior.AutoDelayTest || behavior.AutoDelayTestInterval <= 0 {
		return
	}

	// 3. 开启新任务
	c.autoTestQuit = make(chan struct{})
	go func(quit chan struct{}, intervalMin int) {
		ticker := time.NewTicker(time.Duration(intervalMin) * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				if clash.IsRunning() {
					// 触发静默测速
					// 注意：这里需要通过 events 通知前端开始测速
				}
			}
		}
	}(c.autoTestQuit, behavior.AutoDelayTestInterval)
}
