//go:build windows

package appcore

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"goclashz/core/clash"
	"goclashz/core/downloader"
	"goclashz/core/logger"
	"goclashz/core/runtimeassets"
	"goclashz/core/sys"
	"goclashz/core/tasks"
	"goclashz/core/utils"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

var helperReadyGroup singleflight.Group

type LogEntry = logger.LogEntry

type AutoDelayRefreshOptions struct {
	Immediate bool
	Reason    string
}

type BootstrapOptions struct {
	IsStartupLaunch bool
	Silent          bool
}

type Options struct {
	Events  EventSink
	Version string

	// OnStateChange 在每次 SyncState() 时回调，用于 App 层同步托盘等外部 UI
	OnStateChange func()
}

// CoreRuntimeState 核心运行时状态
type CoreRuntimeState struct {
	Running      bool
	PID          int
	Owner        string // direct/helper
	Purpose      string // user/delay/startup
	Tun          bool
	SystemProxy  bool
	ActiveConfig string
	Mode         string
	StartedAt    time.Time
}

// runtimeStateStore 线程安全的核心运行时状态存储
type runtimeStateStore struct {
	mu    sync.RWMutex
	state CoreRuntimeState
}

func (s *runtimeStateStore) Get() CoreRuntimeState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *runtimeStateStore) Set(st CoreRuntimeState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = st
}

func (s *runtimeStateStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = CoreRuntimeState{}
}

// runtimeSatisfiesCore 检查当前运行时状态是否满足期望核心状态
func (c *Controller) runtimeSatisfiesCore(plan RuntimePlan) bool {
	if !clash.IsRunning() {
		return false
	}

	st := c.runtimeState.Get()
	if !st.Running {
		return false
	}
	return st.Purpose == "user" &&
		st.Tun == plan.NeedTun &&
		st.ActiveConfig == plan.ActiveConfig &&
		st.Mode == plan.Mode
}

type Controller struct {
	events        EventSink
	onStateChange func()
	Behavior      *BehaviorStore
	Offline       *OfflineNodeStore
	Tasks         *tasks.Manager
	version       string

	Supervisor *CoreSupervisor
	Desired    *DesiredStateStore
	controls   *ControlCoordinator

	mu                sync.RWMutex
	coreLifecycleMu   sync.Mutex
	componentUpdateMu sync.Mutex
	sysProxyActive    bool
	// tunActive 是 TUN 的「实际」状态：只有 sys.ProbeTunAdapter 成功探测到
	// 真实存在的 Up 适配器时才会被置 true，绝不由 desired.Tun 推导。
	tunActive    bool
	tunProbeInfo sys.TunAdapterInfo // 最近一次成功探测到的适配器快照（只读，诊断用）
	tunProbeOK   bool

	// TUN 就绪静默阶梯：内核以 TUN 模式运行但适配器迟迟未就绪时的升级状态
	// （有界等待 → 一次内核重启 → 非阻塞提示），避免「启动中」永久静默。
	tunLadderSince time.Time
	tunLadderStage int

	// 通知去重：同一条 error 在 notifyDedupWindow 内只发一次。
	// TUN 就绪这类瞬态状态可能被 watchdog / 事务 / 启动恢复多条路径同时上报，
	// 逐条弹窗会变成"重启一次弹好几个框"。
	notifyMu   sync.Mutex
	notifyLast map[string]time.Time

	// 自动测速任务控制
	autoTestQuit chan struct{}
	autoTestMu   sync.Mutex

	appLogger   *LogDispatcher
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

	GeoUpdates  *GeoUpdateManager
	UpdateTasks *ComponentUpdateTaskStore

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

	// 核心运行时状态
	runtimeState runtimeStateStore

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

func NewController(opts Options) *Controller {
	behavior := NewBehaviorStore()
	activeConfig := behavior.Get().ActiveConfig

	c := &Controller{
		events:        opts.Events,
		onStateChange: opts.OnStateChange,
		version:       opts.Version,
		Behavior:      behavior,
		Offline:       NewOfflineNodeStore(activeConfig),
		Tasks:         tasks.NewManager(opts.Events),
		Desired:       NewDesiredStateStore(),
	}

	// 🚀 核心修复：如果用户未开启“开机自启并恢复状态”，则在应用实例化时立即重置代理意图。
	// 这保证了前端在第一时间拉取 GetAppState 时，拿到的是干净的 false 状态，
	// 彻底杜绝每次启动页面都“先显示上次启动状态，然后才复位”的闪烁 Bug。
	if !behavior.Get().RestoreOnStartup {
		d := c.Desired.Get()
		if d.SystemProxy || d.Tun || d.CoreRunning || d.PortProxy {
			d.SystemProxy = false
			d.Tun = false
			d.CoreRunning = false
			d.PortProxy = false
			_ = c.Desired.SetAndSave(d)
		}
	}
	c.appLogger = NewLogDispatcher(opts.Events, func() string {
		return c.Behavior.Get().AppLogLevel
	})
	c.Supervisor = NewCoreSupervisor(c, c.Desired)
	clash.SetMipsFallbackNotifier(func() {
		c.events.Emit("tun-mips-fallback")
	})
	c.controls = NewControlCoordinator(c)
	c.traffic = NewTrafficStreamManager(opts.Events, c.appLogger)
	c.logs = NewLogStreamManager(c.appLogger)
	c.Delay = NewDelayTestManager(opts.Events, c)
	c.proxyState = NewProxyStateMonitor(opts.Events)
	c.connections = NewConnectionMonitorManager(opts.Events)

	c.UpdateTasks = NewComponentUpdateTaskStore(opts.Events)
	c.GeoUpdates = NewGeoUpdateManager(opts.Events, c.updateGeoDatabase, c.UpdateTasks)

	// 提权重启（UAC）会在 core/sys 里直接 os.Exit：退出前先把系统代理/内核收拾干净，
	// 否则注册表里的代理仍指向已退出的内核，用户重开后直接断网。
	// 方向只能是 appcore → sys（sys 不得 import appcore），所以用回调注入。
	sys.SetBeforeElevatedExit(c.cleanupBeforeElevatedExit)

	return c
}

func (c *Controller) logInfo(format string, args ...any) {
	c.appLogger.AddApp(logger.LogEntry{
		Type:    "info",
		Payload: fmt.Sprintf(format, args...),
	})
}

func (c *Controller) logWarn(format string, args ...any) {
	c.appLogger.AddApp(logger.LogEntry{
		Type:    "warn",
		Payload: fmt.Sprintf(format, args...),
	})
}

func (c *Controller) logError(format string, args ...any) {
	c.appLogger.AddApp(logger.LogEntry{
		Type:    "error",
		Payload: fmt.Sprintf(format, args...),
	})
}

func (c *Controller) LogApp(level string, format string, args ...any) {
	if c == nil || c.appLogger == nil {
		return
	}

	c.appLogger.AddApp(logger.LogEntry{
		Type:    logger.NormalizeLevel(level),
		Payload: fmt.Sprintf(format, args...),
	})
}

func (c *Controller) CancelUpdateTask(key string) {
	if isGeoKey(key) {
		c.GeoUpdates.Cancel(key)
	} else {
		c.Tasks.Cancel(key)
	}
}

func (c *Controller) RemoveUpdateTask(key string) {
	c.CancelUpdateTask(key) // 总是先取消再移除
	c.ClearUpdateCache(key) // 清理可能遗留的断点续传临时文件
	c.UpdateTasks.Remove(key)
}

func (c *Controller) ClearUpdateCache(key string) {
	var destPath string
	switch key {
	case "core-update":
		destPath = filepath.Join(utils.GetCoreBinDir(), "clash.update.zip")
	case "driver-install":
		destPath = filepath.Join(utils.GetCoreBinDir(), "wintun.dll.zip")
	default:
		destPath, _ = clash.GeoDBPath(key)
	}

	if destPath != "" {
		_ = os.Remove(destPath + ".tmp")
		_ = os.Remove(destPath + ".tmp.meta.json")
	}
}

func (c *Controller) Bootstrap(ctx context.Context, opts BootstrapOptions) {
	c.ctx = ctx
	CleanLegacyFiles(c.version)

	// 后台预热内核版本快照：GetAppState 只读无锁原子值（F-A2-15），
	// 首次填充放到后台，避免 UI 线程上 spawn clash.exe -v。
	go func() { _ = clash.GetLocalCoreVersion(ctx) }()

	clash.SetOnExitCallback(func(e clash.ExitEvent) {
		if !e.Intentional {
			c.mu.Lock()
			wasCoreRunning := c.userCoreRunning
			wasSysProxy := c.sysProxyActive
			c.sysProxyActive = false
			c.tunActive = false
			c.userCoreRunning = false
			c.coreStartedAt = time.Time{}
			c.mu.Unlock()

			if !wasCoreRunning {
				// 如果在拉起过程（尚未设置为运行态）中遭遇临时挂掉，静默忽略，
				// 交由 ensureCoreRunningWithDesiredState 内部的探活重试逻辑处理，避免 UI 假闪。
				return
			}

			c.runtimeState.Clear()

			if wasSysProxy {
				c.disableOwnedSystemProxyBestEffort()
			}

			c.events.Emit("clash-exited", e.Message)
			c.SyncState()
			c.Supervisor.ReconcileAsync("core-exited")
		}
	})

	c.syncStartupTaskStateSafe()

	c.Supervisor.Start(ctx)

	// 只有自启且配置了恢复状态时，才执行后台恢复任务（异步，带重试，不阻塞 UI）
	if opts.IsStartupLaunch && c.Behavior.Get().RestoreOnStartup {
		// 启动恢复：在后台完成后再启动 auto delay
		go func() {
			c.runStartupRestoreJob(ctx)
			c.RefreshAutoDelayTest(AutoDelayRefreshOptions{
				Immediate: true,
				Reason:    "startup",
			})
		}()
	} else {
		// 普通双击启动，或者关闭了恢复状态的自启：清空期望状态
		desired := c.Desired.Get()
		desired.SystemProxy = false
		desired.Tun = false
		desired.CoreRunning = false
		desired.PortProxy = false
		c.Desired.SetAndSave(desired)

		c.SyncState()

		// 非启动恢复，立即启动 auto delay
		c.RefreshAutoDelayTest(AutoDelayRefreshOptions{
			Immediate: true,
			Reason:    "startup",
		})
	}

	c.RefreshAppAutoUpdate()
}

// runStartupRestoreJob 在后台异步恢复代理状态，带指数退避重试
// 使用同步 Reconcile，结果驱动
func (c *Controller) runStartupRestoreJob(ctx context.Context) {
	backoff := []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second}

	for i, delay := range backoff {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		desired := c.Desired.Get()
		if !desired.CoreRunning && !desired.SystemProxy && !desired.Tun && !desired.PortProxy {
			return
		}

		// 根据 desired 类型设置不同的 timeout。
		// TUN 的预算必须大于内层 tunAdapterReadyTimeout（20s），否则内层永远等不满就被父 ctx 掐断。
		restoreTimeout := 8 * time.Second
		if desired.Tun {
			restoreTimeout = tunAdapterReadyTimeout + 15*time.Second
		}

		attemptCtx, cancel := context.WithTimeout(ctx, restoreTimeout)
		err := c.Supervisor.Reconcile(attemptCtx, "startup-restore")
		cancel()

		if err == nil {
			c.logInfo("启动恢复成功 (attempt %d)", i+1)
			return
		}

		switch {
		case errors.Is(err, ErrNoActiveConfig):
			c.notify("error", "STARTUP_RESTORE_NO_CONFIG", "启动恢复失败：尚未添加配置", "startup")
			return
		case errors.Is(err, ErrHelperInstallRequired):
			// 服务根本未安装（例如纯绿色解压版），直接通知引导
			c.notify("error", "STARTUP_RESTORE_HELPER_REQUIRED", "TUN 启动恢复需要初始化后台服务", "startup")
			return
		default:
			// 对于 ErrHelperRepairRequired、ErrWintunMissing 或开机 SCM 忙等瞬态延迟：
			// 在退避重试次数未耗尽前继续下一次尝试，避免因开机瞬时卡顿提前弹出错误
			if i < len(backoff)-1 {
				c.logWarn("启动恢复尝试 (%d/%d) 暂未就绪: %v，将在 %v 后重试", i+1, len(backoff), err, backoff[i+1])
				continue
			}

			// 5 次重试全部耗尽后，作为最终失败通知用户。
			// 例外：TUN 适配器只是"晚一点才 Up"（内核已运行、驱动与助手都正常），
			// 这种情况由 supervisor 的周期探测接管即可 —— 弹阻塞式模态框只会让用户以为程序坏了，
			// 且开机时几乎每次都会出现（实测冷启动 wintun 经常 10s 以上才就绪）。
			if errors.Is(err, ErrHelperRepairRequired) {
				c.notify("error", "STARTUP_RESTORE_HELPER_REQUIRED", "TUN 启动恢复需要初始化后台服务", "startup")
			} else if errors.Is(err, ErrWintunMissing) {
				c.notify("error", "STARTUP_RESTORE_WINTUN_MISSING", "TUN 启动恢复失败：缺少 Wintun 驱动", "startup")
			} else if isTunAdapterPendingErr(err) {
				c.logWarn("TUN 启动恢复尚未就绪，转为后台探测（不再弹窗）: %v", err)
			} else {
				c.notify("error", "STARTUP_RESTORE_FAILED", fmt.Sprintf("TUN 启动恢复失败: %v", err), "startup")
			}
		}
	}

	c.logWarn("启动恢复在 5 次尝试后仍未成功")
}

func (c *Controller) syncStartupTaskStateSafe() {
	info, err := sys.CheckStartupTask()
	if err != nil {
		// 该事件在前端没有订阅者（注册但无人消费），补一条日志避免整条链路静默（F-A5-23）。
		c.logWarn("开机自启状态检查失败: %v", err)
		c.events.Emit("startup-task-check-warning", err.Error())
		return
	}

	behavior := c.Behavior.Get()

	// ① 任务根本不存在 → 确实需要同步为 false
	if !info.Exists {
		if behavior.StartupWithOS {
			behavior.StartupWithOS = false
			_ = c.Behavior.SetAndSave(behavior)
		}
		return
	}

	// ② 任务存在，但用户在任务计划器里手动关掉了
	//    （SchedulerEnabled=false 且无错误，说明是用户主动禁用而非健康检查失败）
	if !info.SchedulerEnabled && info.LastError == "" {
		if behavior.StartupWithOS {
			behavior.StartupWithOS = false
			_ = c.Behavior.SetAndSave(behavior)
		}
		return
	}

	// ③ 任务存在且 SchedulerEnabled=true，但健康检查失败
	//    （路径/参数不匹配、缺少 --silent、旧任务等）
	//    → 如果用户期望开机自启，自动执行静默自愈修复
	if !info.IsHealthy {
		if behavior.StartupWithOS && info.ExpectedPath != "" {
			c.logInfo("检测到开机自启项不健康或参数不全，自动自愈修复注册表: %s", info.LastError)
			if err := sys.CreateStartupTask(info.ExpectedPath); err == nil {
				c.logInfo("✅ 自动自愈修复开机自启注册表成功")
				return
			} else {
				c.logWarn("自动修复开机自启注册表失败: %v", err)
			}
		}
		// 同上：事件无消费者，补日志留痕（F-A5-23）。
		c.logWarn("开机自启项不健康: %s", info.LastError)
		c.events.Emit("startup-task-unhealthy", info.LastError)
		return
	}

	// ④ 任务存在且健康 → 双向同步（防止外部改动漏同步）
	if behavior.StartupWithOS != info.SchedulerEnabled {
		behavior.StartupWithOS = info.SchedulerEnabled
		_ = c.Behavior.SetAndSave(behavior)
	}
}

func (c *Controller) GetEvents() EventSink {
	return c.events
}

// AppState 定义全局状态同步结构
type AppState struct {
	IsRunning   bool   `json:"isRunning"`
	IsAdmin     bool   `json:"isAdmin"`
	Mode        string `json:"mode"`
	Theme       string `json:"theme"`
	HideLogs    bool   `json:"hideLogs"`
	LogLevel    string `json:"logLevel"`
	AppLogLevel string `json:"appLogLevel"`
	// 用户意图（卡片数据源）
	DesiredSystemProxy bool `json:"desiredSystemProxy"`
	DesiredTun         bool `json:"desiredTun"`
	DesiredPortProxy   bool `json:"desiredPortProxy"`
	// 后端实际 runtime 状态（诊断/IP 检测用）
	ActualSystemProxy bool   `json:"actualSystemProxy"`
	ActualTun         bool   `json:"actualTun"`
	ControlRevision   uint64 `json:"controlRevision"`
	// 兼容旧前端
	SystemProxy bool   `json:"systemProxy"`
	Tun         bool   `json:"tun"`
	Version     string `json:"version"`
	AppVersion  string `json:"appVersion"`
	// 让前端实时知道当前在跑哪个配置
	ActiveConfig     string `json:"activeConfig"`
	ActiveConfigName string `json:"activeConfigName"`
	ActiveConfigType string `json:"activeConfigType"`
	// 延迟保留相关
	DelayRetention     bool   `json:"delayRetention"`
	DelayRetentionTime string `json:"delayRetentionTime"`

	// 应用更新相关
	UpdateReady      bool   `json:"updateReady"`
	NewAppVersion    string `json:"newAppVersion"`
	UpdateDownloaded bool   `json:"updateDownloaded"`
	DownloadedPath   string `json:"downloadedPath"`

	// 仅端口代理模式（从 behavior 读取，传给前端以驱动按钮文字和行为）
	PortProxyMode bool `json:"portProxyMode"`

	// 界面语言 ("zh-CN", "zh-TW", "en-US")
	Language string `json:"language"`
}

// GetAppState 获取应用运行状态快照
func (c *Controller) GetAppState() AppState {
	behavior := c.Behavior.Get()
	activeConfig := behavior.ActiveConfig
	desired := c.Desired.Get()

	c.mu.RLock()
	sysProxy := c.sysProxyActive
	tunActive := c.tunActive
	userRunning := c.userCoreRunning
	c.mu.RUnlock()

	// 将内核物理运行状态与 UI 业务接管状态解耦
	logicalIsRunning := clash.IsRunning() && userRunning

	state := AppState{
		IsRunning:          logicalIsRunning,
		IsAdmin:            sys.CheckAdmin(),
		Mode:               behavior.ActiveMode,
		DesiredSystemProxy: desired.SystemProxy,
		DesiredTun:         desired.Tun,
		DesiredPortProxy:   desired.PortProxy,
		ActualSystemProxy:  sysProxy,
		ActualTun:          tunActive,
		SystemProxy:        sysProxy,
		Tun:                tunActive,
		Theme:              ResolveThemeName(behavior),
		Version:            clash.CachedCoreVersion(), // 不 spawn、不持锁（F-A2-15）
		AppVersion:         c.version,
		ActiveConfig:       activeConfig,
		DelayRetention:     behavior.DelayRetention,
		DelayRetentionTime: behavior.DelayRetentionTime,
		HideLogs:           behavior.HideLogs,
		LogLevel:           behavior.LogLevel,
		AppLogLevel:        behavior.AppLogLevel,
		UpdateReady:        c.updateReady,
		NewAppVersion:      c.newAppVersion,
		UpdateDownloaded:   c.downloadedUpdatePath != "",
		DownloadedPath:     c.downloadedUpdatePath,
		PortProxyMode:      behavior.PortProxyMode,
		Language:           behavior.Language,
	}
	if c.controls != nil {
		state.ControlRevision = c.controls.Revision()
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

	if c.onStateChange != nil {
		c.onStateChange()
	}

	// 🚀 核心修复：Controller 自助管理流量流
	if c.ctx != nil {
		if state.IsRunning {
			behavior := c.Behavior.Get()
			c.traffic.Start(c.ctx, clash.APIURL("/traffic?interval=900"), behavior.ProxyTrafficOnly)
			c.proxyState.Start(c.ctx)
		} else {
			c.traffic.Stop()
			c.proxyState.Stop()
		}
	}
}

// --- 内部方法 (需要提前持有 coreLifecycleMu) ---

// assetReady 判断某个资产在当前状态里是否已就绪；缺失条目按「未就绪」处理，
// 保证判定不会因为状态缺项而放宽。
func assetReady(status runtimeassets.RuntimeAssetStatus, key runtimeassets.AssetKey) bool {
	h, ok := status.Assets[key]
	return ok && h.Ready
}

// fastAssetBlocked 判断快检结论是否「明确不可用」（不存在 / 是目录 / PE 损坏）。
//
// 快检（runtimeassets.GetStatusFast）只做 stat + PE 头 + 版本资源，不跑 clash.exe -v、
// 不算 SHA256。它给出的 Ready=false 只有在「明确」时才允许阻断用户动作；
// 其余模糊情形交给修复/启动路径里的全量 EnsureReady 裁决（F-A2-02）。
func fastAssetBlocked(h runtimeassets.AssetHealth) bool {
	return !h.Ready && !h.Valid
}

// fastRequirementBlocked 判断快检结果是否不满足某个资产要求。
func fastRequirementBlocked(status runtimeassets.RuntimeAssetStatus, req runtimeassets.Requirement) bool {
	if req.NeedCore {
		h, ok := status.Assets[runtimeassets.AssetCore]
		if !ok || fastAssetBlocked(h) {
			return true
		}
	}
	if req.NeedWintun {
		h, ok := status.Assets[runtimeassets.AssetWintun]
		if !ok || fastAssetBlocked(h) {
			return true
		}
	}
	if req.NeedGeo {
		for _, key := range []runtimeassets.AssetKey{
			runtimeassets.AssetGeoIP,
			runtimeassets.AssetGeoSite,
			runtimeassets.AssetMMDB,
			runtimeassets.AssetASN,
		} {
			if h, ok := status.Assets[key]; ok && !h.Ready && !h.Exists {
				return true
			}
		}
	}
	return false
}

// fastHelperPing 在给定预算内做一次 helper 可达性探测（Ping 自身预算约 350ms）。
// 返回 false 表示没能在预算内确认可达——调用方应转为后台唤醒，而不是阻塞用户动作。
func fastHelperPing(client *sys.HelperClient, budget time.Duration) bool {
	done := make(chan struct{})
	var ok bool

	go func() {
		defer close(done)
		ok = client.Ping() == nil
	}()

	timer := time.NewTimer(budget)
	defer timer.Stop()

	select {
	case <-done:
		return ok
	case <-timer.C:
		return false
	}
}

func (c *Controller) ensureCoreRunningWithDesiredState(ctx context.Context, desired DesiredState) error {
	// 检查当前运行时状态是否已满足期望
	if c.runtimeSatisfiesCore(RuntimePlan{
		NeedTun:      desired.Tun,
		ActiveConfig: desired.ActiveConfig,
		Mode:         desired.Mode,
	}) {
		// 运行时已满足：仍需用真实适配器校正一次 ActualTun，
		// 避免「上一次探测失败」导致实际状态永久失真（这里不重启内核）。
		if desired.Tun {
			if info, err := c.probeTunActual(); err != nil {
				// 内核已在跑 TUN，只是适配器还没 Up 或探测这一次没命中：
				// watchdog 的 repairTunActualIfNeeded 会自动校正，不需要打扰用户。
				c.logWarn("TUN 适配器尚未就绪（内核已运行，不重启，等待后台校正）: %v", err)
				c.SyncState()
			} else {
				c.logInfo("TUN 适配器就绪: %s", info.Summary())
			}
		} else {
			c.clearTunActual()
		}
		return nil
	}

	// 启动核心前兜底：从 seed 修补缺失/损坏的 core/wintun（在 activeConfig 检查之前）。
	//
	// 快检优先（F-A2-02）：stat + PE 头就足以回答「能不能启动」，
	// 不再在每次起内核时先跑 95MB 哈希 + clash.exe -v；只有快检明确不可用时
	// 才进入全量检查/修复分支（修复本身就要求全量结论）。
	assetReq := runtimeassets.RequireCoreOnly
	if desired.Tun {
		assetReq = runtimeassets.RequireTun
	}
	if fastStatus := runtimeassets.GetStatusFast(ctx); fastRequirementBlocked(fastStatus, assetReq) {
		if assetStatus, assetErr := runtimeassets.EnsureReady(ctx, assetReq, runtimeassets.RepairInvalid); assetErr != nil {
			for _, asset := range assetStatus.Assets {
				if !asset.Ready && asset.Error != "" {
					c.logWarn("运行组件不可用: %s path=%s error=%s", asset.Key, asset.Path, asset.Error)
				}
			}

			// 资产锁被并发占用（另一个修复/迁移流程正在跑）属于 A 类瞬时可重试，
			// 绝不能升级成「开关失败」——那会把"稍后再试"变成一次用户可见的失败（F-A1-13）。
			if errors.Is(assetErr, runtimeassets.ErrAssetLocked) {
				c.logWarn("运行组件修复被并发占用（瞬时可重试，继续启动，失败会由后台重试）: %v", assetErr)
			} else {
				// 修复瑕疵不再升级成「开关失败」：一次 tmp 写失败、rename 被占用、版本探测超时，
				// 只要内核 / wintun 的文件本身仍判定 Ready，就不该阻断本次启动。
				// 只有真正 !Ready（文件缺失、PE 损坏）才保留硬失败。
				coreBlocked := assetReq.NeedCore && !assetReady(assetStatus, runtimeassets.AssetCore)
				wintunBlocked := assetReq.NeedWintun && !assetReady(assetStatus, runtimeassets.AssetWintun)
				if coreBlocked || wintunBlocked {
					return fmt.Errorf("运行组件不可用: %w", assetErr)
				}

				c.logWarn("运行组件修复未完全成功，但所需资产已就绪，继续启动: %v", assetErr)
			}
		}
	}

	if desired.ActiveConfig == "" {
		return ErrNoActiveConfig
	}

	// TUN 模式：确保 helper 服务就绪（可被调用方 ctx 打断，见 F-A2-09）
	if desired.Tun {
		if err := c.EnsureHelperReadyWithContext(ctx, "tun-start"); err != nil {
			return err
		}
	}

	behavior := c.Behavior.Get()

	err := clash.BuildRuntimeConfig(desired.ActiveConfig, desired.Mode, behavior.LogLevel, desired.Tun)
	if err != nil {
		return err
	}

	// geo 补齐是网络下载（最多 4×200MB），绝不能挂在用户同步路径上（F-A2-14）：
	// 转后台并用自己的长预算，失败只记日志，不阻断本次启动。
	c.ensureRuntimeAssetsAsync(behavior)

	// 调用方预算已耗尽时不要再去 spawn -t：那只会把"没时间了"伪装成"配置错误"。
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("启动已取消: %w", err)
	}

	// 启动内核前校验生成的运行时配置（秒级拦截语法错误、不支持的协议栈等）。
	// 配置内容未变时复用上次校验结论，避免每次起内核都 spawn 一次 clash.exe -t（F-A0-06）。
	if err := clash.ValidateConfigCached(ctx, utils.GetRuntimeConfigPath()); err != nil {
		c.logError("运行时配置预检未通过: %v", err)
		return fmt.Errorf("配置预检未通过: %w", err)
	}

	if err := clash.Start(ctx, desired.Tun); err != nil {
		return err
	}

	apiReady := false
	var lastProbeErr error
	for i := 0; i < 100; i++ {
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

		if !clash.IsRunning() {
			// 如果在探活期间内核进程挂掉（如遭遇残存的 TIME_WAIT 端口冲突），
			// 避免原地等待 10 秒，而是等待缓冲后自动原地重试拉起。
			clash.Stop()
			// 内核已异常退出，立即提取真实错误原因快速失败，杜绝盲等超时
			if valErr := clash.ValidateConfig(ctx, utils.GetRuntimeConfigPath()); valErr != nil {
				c.mu.Lock()
				c.userCoreRunning = false
				c.coreStartedAt = time.Time{}
				c.mu.Unlock()
				return fmt.Errorf("内核启动异常退出: %w", valErr)
			}
			if i < 90 {
				time.Sleep(150 * time.Millisecond)
				if err := clash.Start(ctx, desired.Tun); err != nil {
					return err
				}
				continue
			}
		}

		if err := clash.PingAPIWithContext(ctx); err == nil {
			apiReady = true
			break
		} else {
			lastProbeErr = err
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
		clash.Stop()
		c.mu.Lock()
		c.userCoreRunning = false
		c.coreStartedAt = time.Time{}
		c.mu.Unlock()
		return fmt.Errorf("内核启动失败: %s", classifyProbeError(lastProbeErr))
	}

	c.mu.Lock()
	c.userCoreRunning = true
	// ⚠️ 绝不在此处把 desired.Tun 写成 ActualTun：
	// TUN 的真实状态必须由后面的适配器探测确认。
	if !desired.Tun {
		c.tunActive = false
		c.tunProbeOK = false
		c.tunProbeInfo = sys.TunAdapterInfo{}
	}
	c.coreStartedAt = time.Now()
	c.mu.Unlock()

	// 设置运行时状态
	owner := "direct"
	if desired.Tun {
		owner = "helper"
	}
	c.runtimeState.Set(CoreRuntimeState{
		Running:      true,
		Owner:        owner,
		Purpose:      "user",
		Tun:          desired.Tun,
		SystemProxy:  c.sysProxyActive,
		ActiveConfig: desired.ActiveConfig,
		Mode:         desired.Mode,
		StartedAt:    time.Now(),
	})

	// TUN：内核进程健康不等于 TUN 可用，必须以真实适配器（描述 Meta Tunnel / 设备名）
	// 探测成功为准；失败则置错误状态并保留实际值（ActualTun 保持 false）。
	if desired.Tun {
		if err := c.waitForTunAdapter(ctx); err != nil {
			if isTunAdapterPendingErr(err) {
				// 内核已成功起来，只是适配器还没 Up（含"这次没等到"）：
				// 交给 watchdog 的 repairTunActualIfNeeded 收敛，不弹错误框、也不回滚启动。
				c.logWarn("TUN 适配器尚未就绪，转为后台探测: %v", err)
				c.SyncState()
			} else {
				c.setLastErrorCode("TUN_ADAPTER_NOT_READY", "TUN 适配器未就绪: "+err.Error())
				c.logError("TUN 适配器未就绪: %v", err)
				c.SyncState()
				return err
			}
		}
	} else {
		c.clearTunActual()
	}

	// 内核已有反应，同步状态
	c.SyncState()

	// 完整代理数据后台处理，不阻塞开关返回
	go func() {
		bgCtx, cancel := context.WithTimeout(c.ctx, 8*time.Second)
		defer cancel()
		c.applyStoredProxySelections(bgCtx, desired.ActiveConfig)
		c.SyncProxyStateOnce()
	}()

	return nil
}

// ensureRuntimeAssetsAsync 在后台补齐 geo 等运行期资产（F-A2-14）。
// 独立长预算（10 分钟），失败只记日志——绝不阻塞用户动作或内核启动。
func (c *Controller) ensureRuntimeAssetsAsync(behavior AppBehavior) {
	base := c.ctx
	if base == nil {
		base = context.Background()
	}

	go func() {
		bgCtx, cancel := context.WithTimeout(base, 10*time.Minute)
		defer cancel()

		if err := EnsureRuntimeAssets(bgCtx, behavior); err != nil {
			c.logWarn("后台补齐运行期资产失败（不阻断启动）: %v", err)
		}
	}()
}

// stopCoreProcessLocked 停止内核并清理运行态；返回 error 表示"停止未被确认"。
// 停止失败时保留 userCoreRunning / ActualTun，避免主进程与真实内核状态分叉（F-A2-06）。
func (c *Controller) stopCoreProcessLocked() error {
	if err := clash.Stop(); err != nil {
		c.logWarn("停止内核未确认，保留运行态待重试: %v", err)
		return err
	}
	c.mu.Lock()
	c.userCoreRunning = false
	// TUN routing ceases the moment the core stops, even though the Wintun
	// adapter may linger in the Windows network stack for a few seconds.
	// Clearing tunActive here avoids emitting a stale actualTun=true snapshot
	// during restart/shutdown, which would mislead the frontend into showing
	// "启动中..." when TUN is actually winding down.
	c.tunActive = false
	c.tunProbeOK = false
	c.tunProbeInfo = sys.TunAdapterInfo{}
	c.coreStartedAt = time.Time{}
	c.mu.Unlock()
	c.runtimeState.Clear()
	return nil
}

// classifyProbeError 将探针错误分类为用户可读的诊断信息
func classifyProbeError(err error) string {
	if err == nil {
		return "API 未能在预期时间内就绪"
	}

	msg := err.Error()

	// 端口未监听：内核未绑定 API 端口
	if strings.Contains(msg, "connection refused") {
		return "端口未监听，内核未绑定 API 端口"
	}

	// 超时
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded") {
		return "内核响应超时，可能配置文件过大或系统资源不足"
	}

	// HTTP 状态码非 2xx
	if strings.Contains(msg, "failed: HTTP") || strings.Contains(msg, "HTTP ") {
		return "端口被其他程序占用，收到非预期响应"
	}

	// 响应非合法 JSON
	if strings.Contains(msg, "unexpected end") || strings.Contains(msg, "invalid character") ||
		strings.Contains(msg, "cannot decode") || strings.Contains(msg, "JSON") {
		return "端口被其他程序占用，响应内容非内核格式"
	}

	// 连接重置 / 连接断开
	if strings.Contains(msg, "connection reset") || strings.Contains(msg, "broken pipe") {
		return "连接被重置，内核可能已崩溃"
	}

	// 其他 net.OpError
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return "网络连接异常"
	}

	return "API 探针异常，请检查内核配置"
}

// --- 导出方法 ---

// EnsureCoreRunning 确保内核已启动并就绪 (带锁包装)
func (c *Controller) EnsureCoreRunning(ctx context.Context) error {
	c.coreLifecycleMu.Lock()
	defer c.coreLifecycleMu.Unlock()

	desired := c.Desired.Get()
	if desired.ActiveConfig == "" {
		behavior := c.Behavior.Get()
		desired.ActiveConfig = behavior.ActiveConfig
		desired.Mode = behavior.ActiveMode
	}
	return c.ensureCoreRunningWithDesiredState(ctx, desired)
}

// StopCoreService 停止内核并清理所有运行时状态（通常用于程序退出）
func (c *Controller) StopCoreService() {
	c.DisableAll()
}

func (c *Controller) reconcileCoreProcess(ctx context.Context, plan RuntimePlan) error {
	if c.runtimeSatisfiesCore(plan) {
		return nil
	}

	if plan.RestartRequired {
		// supervisor 已按 reconcileMu 串行化，直接使用内部实现，
		// 避免重入 ControlCoordinator 的串行锁。
		if err := c.restartCoreWithReason(ctx, plan.Reason); err != nil {
			return fmt.Errorf("restart core failed: %w", err)
		}
	} else {
		if err := c.EnsureCoreRunning(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) reconcileSystemProxy(plan RuntimePlan) error {
	if plan.NeedSysProxy {
		return c.ensureSystemProxyEnabled()
	}
	return c.ensureSystemProxyDisabled()
}

func (c *Controller) ensureSystemProxyEnabled() error {
	port := clash.GetProxyPort()
	current, err := sys.GetSystemProxyState()
	// Windows 10/11 有时将注册表代理写成 "http=127.0.0.1:7890;https=..." 格式，
	// 严格相等会误判失效；用 Contains 兼容两种格式。
	expectedAddr := fmt.Sprintf("127.0.0.1:%d", port)
	if err == nil && current.Enabled && strings.Contains(current.Server, expectedAddr) {
		c.mu.Lock()
		c.sysProxyActive = true
		c.mu.Unlock()
		return nil
	}

	err = sys.EnableSystemProxy(
		"127.0.0.1",
		port,
		"localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*;<local>",
	)
	if err != nil {
		c.setLastErrorCode("SYSTEM_PROXY_SET_FAILED", "设置 Windows 系统代理失败: "+err.Error())
		c.mu.Lock()
		c.sysProxyActive = false
		c.mu.Unlock()
		return err
	}
	c.mu.Lock()
	c.sysProxyActive = true
	c.mu.Unlock()
	return nil
}

func (c *Controller) ensureSystemProxyDisabled() error {
	current, err := sys.GetSystemProxyState()
	if err != nil {
		return fmt.Errorf("读取 Windows 系统代理状态失败: %w", err)
	}

	if current.Enabled {
		// 只关闭「可判定为本程序写入」的代理：注册表里若是别的工具写的，
		// 关掉它会直接打断用户的其它代理（F-A2-07）。
		if !c.systemProxyOwnedByUs(current) {
			c.logWarn("系统代理当前指向其它程序(%s)，跳过关闭", current.Server)
			c.mu.Lock()
			c.sysProxyActive = false
			c.mu.Unlock()
			return nil
		}
		if err := sys.DisableSystemProxy(); err != nil {
			return fmt.Errorf("关闭 Windows 系统代理失败: %w", err)
		}
		sys.UnmarkSystemProxyOwned()
	}

	c.mu.Lock()
	c.sysProxyActive = false
	c.mu.Unlock()
	return nil
}

// systemProxyOwnedByUs 判断注册表里的系统代理是否指向本程序的本地端口。
// 用于「关闭意图」时的安全判断：不是我们写的就不要动（F-A2-07）。
func (c *Controller) systemProxyOwnedByUs(state sys.SystemProxyState) bool {
	if c.systemProxyActiveNow() {
		return true
	}
	expected := fmt.Sprintf("127.0.0.1:%d", clash.GetProxyPort())
	return strings.Contains(state.Server, expected) ||
		strings.Contains(state.Server, fmt.Sprintf("localhost:%d", clash.GetProxyPort()))
}

// disableOwnedSystemProxyBestEffort 尽力关闭「本程序写入」的系统代理，绝不误伤其它工具。
// 用于退出 / 关闭代理等收尾路径（F-A2-07）。
func (c *Controller) disableOwnedSystemProxyBestEffort() {
	if c.systemProxyActiveNow() {
		if err := sys.DisableSystemProxy(); err != nil {
			c.logWarn("关闭系统代理失败: %v", err)
		}
		sys.UnmarkSystemProxyOwned()
	} else {
		// 状态未知时只在「所有权文件 + 当前注册表」双匹配时才清（proxy_guard 内已判定）。
		sys.ClearOwnedSystemProxy()
	}
	c.mu.Lock()
	c.sysProxyActive = false
	c.mu.Unlock()
}

// setRuntimeStateFromPlan 提交「由 plan 可直接推导」的运行时字段。
//
// ⚠️ 这里刻意不再写 tunActive = plan.NeedTun：
// 历史实现以内核进程健康即认为 TUN 已生效，导致 ActualTun 由 desired 伪造，
// 实际适配器未创建时前端/托盘仍显示 TUN 已开启（问题根因之一）。
// 现在 ActualTun 只能来自 probeTunActual/watchForTunAdapter 的真实探测结果；
// plan 只负责在「不再需要 TUN」时把实际状态归零。
func (c *Controller) setRuntimeStateFromPlan(plan RuntimePlan) {
	c.mu.Lock()
	if !plan.NeedTun {
		c.tunActive = false
		c.tunProbeOK = false
		c.tunProbeInfo = sys.TunAdapterInfo{}
	}
	c.userCoreRunning = plan.NeedCore
	c.mu.Unlock()
}

// --- TUN 实际状态 / 运行时事务辅助 ---

// isTunAdapterPendingErr 判断错误是否属于「TUN 适配器还没 Up」这类**可自愈**的情况。
//
// 这类情况的特点是：内核已经在以 TUN 模式运行，只是 wintun 适配器还没被创建/还没 Up。
// watchdog 每 5s 的 repairTunActualIfNeeded 会在适配器真正 Up 之后把 ActualTun 自动校正为 true，
// 所以它不该被当成"故障"弹给用户。
//
// 三个来源都要覆盖：
//  1) 适配器不存在 / 存在但没 Up（真正的 pending）；
//  2) 等待过程中调用方上下文到期（watchdog / 协调器只给 8s，比 TUN 等待上限 20s 短）；
//  3) 文案被上层再包一层后的形态。
func isTunAdapterPendingErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sys.ErrTunAdapterNotFound) || errors.Is(err, sys.ErrTunAdapterNotReady) {
		return true
	}
	msg := err.Error()
	// 等待 TUN 被 ctx 掐断：属于"这次没等到"，不是适配器坏了
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return strings.Contains(msg, "TUN")
	}
	return (strings.Contains(msg, "TUN 适配器") && strings.Contains(msg, "未就绪")) ||
		strings.Contains(msg, "等待 TUN 适配器")
}

const (
	// tunAdapterReadyTimeout 是内核启动后等待 TUN 适配器真实就绪的上限。
	// 冷启动（磁盘慢 / 配置大 / 助手服务刚拉起）时 wintun 适配器常常晚于 8s 才 Up，
	// 8s 会导致"每次开机都报 TUN 未就绪"。20s 仍是有限等待，真正的失败会由后面的分支兜住。
	tunAdapterReadyTimeout = 20 * time.Second
	// tunAdapterPollInterval 是 TUN 适配器只读探测的轮询间隔。
	// 每次探测都要 GetAdaptersAddresses + 两次整表路由遍历（F-A2-22），
	// 200ms 意味着 20s 内最多 100 次全表扫描；400ms 减半且不影响 Up 检测的时效。
	tunAdapterPollInterval = 400 * time.Millisecond
	// helperStopConfirmTimeout 是 helper 托管内核的停止确认上限。
	helperStopConfirmTimeout = 3 * time.Second
	// stopConfirmPollInterval 是 helper 停止确认的轮询间隔。
	stopConfirmPollInterval = 200 * time.Millisecond
	// providerReadyTimeout 是内核启动后等待代理组/provider 数据就绪的上限。
	providerReadyTimeout = 10 * time.Second
	// providerPollInterval 是代理组就绪探测的轮询间隔。
	providerPollInterval = 250 * time.Millisecond
)

// tunDeviceName 返回当前 TUN 设置里的设备名。
// 读取失败时退回内置默认值（探测仍可通过适配器描述 "Meta Tunnel" 命中）。
func (c *Controller) tunDeviceName() string {
	if cfg, err := clash.GetTunConfig(); err == nil && cfg != nil {
		if name := strings.TrimSpace(cfg.Device); name != "" {
			return name
		}
	}
	return clash.GetDefaultTunConfig().Device
}

// probeTunActual 执行一次只读探测，成功时把 ActualTun 置为 true 并记录快照。
// 返回 error 表示适配器未就绪（未创建 / 未 Up），此时 ActualTun 保持原值不变。
func (c *Controller) probeTunActual() (sys.TunAdapterInfo, error) {
	info, err := sys.ProbeTunAdapter(c.tunDeviceName())
	if err != nil {
		c.mu.Lock()
		c.tunProbeOK = false
		c.mu.Unlock()
		return info, err
	}

	c.mu.Lock()
	c.tunActive = true
	c.tunProbeInfo = info
	c.tunProbeOK = true
	c.mu.Unlock()
	return info, nil
}

// waitForTunAdapter 在内核就绪后轮询真实 TUN 适配器，只有探测成功才置 ActualTun。
// 失败时保留既有实际值（绝不用 desired.Tun 兜底），并返回错误由上层置错误状态。
// tunWaitBudgetKey 让调用方限制「等待 TUN 适配器就绪」的预算。
// 用于不能把 20s 挂在同步路径上的场景（配置切换 / 后台收敛）。
type tunWaitBudgetKey struct{}

func withTunWaitBudget(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, tunWaitBudgetKey{}, d)
}

func (c *Controller) waitForTunAdapter(ctx context.Context) error {
	timeout := tunAdapterReadyTimeout
	if b, ok := ctx.Value(tunWaitBudgetKey{}).(time.Duration); ok && b > 0 && b < timeout {
		timeout = b
	}
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		timeout = tunAdapterPollInterval
	}
	deadline := time.Now().Add(timeout)

	var lastErr error
	for {
		if info, err := c.probeTunActual(); err == nil {
			c.logInfo("TUN 适配器已就绪: %s", info.Summary())
			return nil
		} else {
			lastErr = err
		}

		if time.Now().After(deadline) {
			c.mu.Lock()
			c.tunProbeOK = false
			c.mu.Unlock()
			if lastErr == nil {
				lastErr = sys.ErrTunAdapterNotFound
			}
			return fmt.Errorf("TUN 适配器在 %s 内未就绪: %w", timeout.Round(tunAdapterPollInterval), lastErr)
		}

		timer := time.NewTimer(tunAdapterPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("等待 TUN 适配器被取消: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// confirmTunActual 做一次只读确认（不轮询等待），用于调和阶段：
// 内核刚启动时 ensureCoreRunningWithDesiredState 已经完成过轮询等待，
// 这里只复核一次，避免 watchdog 每 5s 又触发 8s 的长等待。
func (c *Controller) confirmTunActual() error {
	c.mu.RLock()
	active := c.tunActive
	c.mu.RUnlock()
	if active {
		return nil
	}
	_, err := c.probeTunActual()
	return err
}

// clearTunActual 清空 ActualTun 与探测快照（仅当确定不需要 TUN 时调用）。
func (c *Controller) clearTunActual() {
	c.mu.Lock()
	c.tunActive = false
	c.tunProbeOK = false
	c.tunProbeInfo = sys.TunAdapterInfo{}
	c.mu.Unlock()
}

// TunProbeSnapshot 返回最近一次成功的 TUN 适配器探测快照（只读，供诊断/验收使用）。
func (c *Controller) TunProbeSnapshot() (sys.TunAdapterInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tunProbeInfo, c.tunProbeOK
}

// repairTunActualIfNeeded 在内核以 TUN 模式运行但 ActualTun 为 false 时做一次
// 只读校正。不会重启内核、不会清理任何系统资源，避免 watchdog 抖动。
func (c *Controller) repairTunActualIfNeeded() {
	if !c.coreLaunchedWithTun() || !clash.IsRunning() {
		return
	}

	c.mu.RLock()
	active := c.tunActive
	c.mu.RUnlock()
	if active {
		return
	}

	info, err := sys.ProbeTunAdapter(c.tunDeviceName())
	if err != nil {
		return
	}

	c.mu.Lock()
	c.tunActive = true
	c.tunProbeInfo = info
	c.tunProbeOK = true
	c.mu.Unlock()

	c.logInfo("TUN 实际状态已校正: %s", info.Summary())
	c.SyncState()
}

const (
	// TUN 就绪静默阶梯的阈值（从「内核以 TUN 模式运行且 ActualTun=false」开始计时）。
	tunLadderWaitAfter    = 15 * time.Second // L1：后台做一次有界等待
	tunLadderRestartAfter = 40 * time.Second // L2：后台重启一次内核
	tunLadderNotifyAfter  = 60 * time.Second // L3：非阻塞提示（toast，不弹模态）
	// tunLadderWaitBudget 是 L1 有界等待的预算。
	tunLadderWaitBudget = 5 * time.Second
	// configSwitchTunWaitBudget 是配置切换事务里等待 TUN 适配器的预算：
	// 事务是用户同步路径，不能按默认 20s 死等（没等到交给静默阶梯收敛）。
	configSwitchTunWaitBudget = 5 * time.Second
)

// escalateTunReadinessIfNeeded 在内核已按 TUN 模式运行、但真实适配器迟迟未就绪时，
// 按静默阶梯升级：L1 后台有界等待 → L2 后台重启一次内核 → L3 非阻塞提示。
// 只有走到 L3 才会打扰用户，符合「A 类静默重试 / B 类非阻塞提示 / C 类模态」的分级。
func (c *Controller) escalateTunReadinessIfNeeded() {
	if !c.coreLaunchedWithTun() || !clash.IsRunning() {
		c.resetTunLadder()
		return
	}

	c.mu.RLock()
	active := c.tunActive
	c.mu.RUnlock()
	if active {
		c.resetTunLadder()
		return
	}

	c.mu.Lock()
	if c.tunLadderSince.IsZero() {
		c.tunLadderSince = time.Now()
	}
	elapsed := time.Since(c.tunLadderSince)
	stage := c.tunLadderStage
	c.mu.Unlock()

	switch {
	case stage < 1 && elapsed >= tunLadderWaitAfter:
		c.mu.Lock()
		c.tunLadderStage = 1
		c.mu.Unlock()
		c.logWarn("TUN 适配器 %s 内未就绪，开始后台有界等待（L1）", tunLadderWaitAfter)
		go c.boundedTunWait()
	case stage < 2 && elapsed >= tunLadderRestartAfter:
		c.mu.Lock()
		c.tunLadderStage = 2
		c.mu.Unlock()
		c.logWarn("TUN 适配器 %s 内仍未就绪，尝试后台重启一次内核（L2）", tunLadderRestartAfter)
		if c.Supervisor != nil {
			c.Supervisor.ReconcileAsync("tun-not-ready-restart")
		}
	case stage < 3 && elapsed >= tunLadderNotifyAfter:
		c.mu.Lock()
		c.tunLadderStage = 3
		c.mu.Unlock()
		c.notify("warn", "TUN_NOT_READY", "TUN 已开启但网卡迟迟未就绪，正在后台重试；可在首页重试开关或导出诊断包", "appcore")
	}
}

// boundedTunWait 是静默阶梯 L1：用自己的短预算再等一次适配器，成功即校正状态。
func (c *Controller) boundedTunWait() {
	base := c.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(base, tunLadderWaitBudget)
	defer cancel()

	if err := c.waitForTunAdapter(ctx); err != nil {
		c.logWarn("后台有界等待 TUN 适配器仍未就绪: %v", err)
		return
	}
	c.logInfo("TUN 适配器在后台有界等待中就绪")
	c.resetTunLadder()
	c.SyncState()
}

// resetTunLadder 清空阶梯状态（TUN 已就绪 / 内核已停 / 意图已变时调用）。
func (c *Controller) resetTunLadder() {
	c.mu.Lock()
	c.tunLadderSince = time.Time{}
	c.tunLadderStage = 0
	c.mu.Unlock()
}

// coreLaunchedWithTun 返回「内核进程是以 TUN 模式启动的」这一事实。
// 它来自 runtimeState（启动意图记录），与 ActualTun（适配器探测结果）严格分离：
// 重启判定必须用它，否则适配器探测失败会导致每个 watchdog tick 都误判需要重启。
func (c *Controller) coreLaunchedWithTun() bool {
	st := c.runtimeState.Get()
	return st.Running && st.Purpose == "user" && st.Tun
}

// stopCoreForTransition 停止内核并尽可能拿到「确实已停止」的证据：
//   - helper 托管（TUN 模式）：通过 helper core-status 轮询确认停止（停止确认来自 helper）
//   - 直连模式：clash.Stop() 内部已 Wait 到进程退出，无需额外确认
//
// 注意：TUN 的适配器/路由/DNS 回收一律由 helper 与内核自身负责，
// 主进程在此只做「观测 + 确认」，绝不盲清系统路由或 DNS。
func (c *Controller) stopCoreForTransition(ctx context.Context) error {
	wasHelper := clash.StartedViaHelper()

	if err := c.stopCoreProcessLocked(); err != nil {
		return err
	}

	if !wasHelper {
		return nil
	}

	client := sys.NewHelperClient()
	deadline := time.Now().Add(helperStopConfirmTimeout)
	var lastErr error

	for {
		if status, err := client.CoreStatus(); err == nil {
			if !status.Running {
				return nil
			}
			lastErr = fmt.Errorf("helper 仍报告内核运行中 (pid=%d)", status.PID)
		} else {
			lastErr = err
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("helper 停止确认超时: %w", lastErr)
		}

		timer := time.NewTimer(stopConfirmPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("helper 停止确认被取消: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// observeTunAdapterAfterStop 只读观测 TUN 停止后适配器是否仍在系统中短时残留。
// 仅记录日志，不做任何清理（清理由 helper/内核负责）。
func (c *Controller) observeTunAdapterAfterStop() {
	info, err := sys.ProbeTunAdapter(c.tunDeviceName())
	if err != nil {
		return
	}
	c.logInfo("TUN 停止后适配器仍存在（等待内核/helper 回收）: %s", info.Summary())
}

// systemProxyActiveNow 返回主进程记录的「系统代理实际已开启」状态。
func (c *Controller) systemProxyActiveNow() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sysProxyActive
}

// systemProxySnapshot 是切换事务开始前的系统代理观测结果。
type systemProxySnapshot struct {
	// owned 表示系统代理配置可判定为「本程序写入」，可以安全关闭
	owned bool
	// known 为 false 表示注册表读取失败，状态未知 —— 不得据此盲目开启/关闭
	known bool
}

// captureSystemProxyState 采集系统代理的真实状态（读失败保留 unknown）。
func (c *Controller) captureSystemProxyState() systemProxySnapshot {
	c.mu.RLock()
	active := c.sysProxyActive
	c.mu.RUnlock()

	state, err := sys.GetSystemProxyState()
	if err != nil {
		c.logWarn("读取系统代理状态失败（按 unknown 处理，不会据此盲目开启）: %v", err)
		return systemProxySnapshot{owned: active, known: false}
	}

	owned := active || systemProxyLooksOwned(state)
	return systemProxySnapshot{owned: owned, known: true}
}

// systemProxyLooksOwned 判断注册表中的系统代理是否指向本程序的本地代理端口。
func systemProxyLooksOwned(state sys.SystemProxyState) bool {
	if !state.Enabled {
		return false
	}
	server := strings.ToLower(strings.TrimSpace(state.Server))
	return strings.Contains(server, "127.0.0.1") || strings.Contains(server, "localhost")
}

// waitProxyGroupsReady 等待内核 API 与代理组/provider 数据都就绪。
// 复用现有内核 API 客户端：/configs + /proxies 均成功且返回至少一个代理组才算就绪。
func (c *Controller) waitProxyGroupsReady(ctx context.Context) error {
	timeout := providerReadyTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		timeout = providerPollInterval
	}
	deadline := time.Now().Add(timeout)

	var lastErr error
	for {
		if err := c.proxyGroupsReadyOnce(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}

		if time.Now().After(deadline) {
			if lastErr == nil {
				lastErr = errors.New("内核尚未返回代理组数据")
			}
			return fmt.Errorf("代理组/provider 在 %s 内未就绪: %w", timeout.Round(providerPollInterval), lastErr)
		}

		timer := time.NewTimer(providerPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("等待代理组就绪被取消: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// proxyGroupsReadyOnce 只读探测一次代理组就绪情况。
func (c *Controller) proxyGroupsReadyOnce(ctx context.Context) error {
	data, err := clash.GetInitialDataWithContext(ctx)
	if err != nil {
		return err
	}

	groups, ok := data["groups"].(map[string]interface{})
	if !ok || len(groups) == 0 {
		return errors.New("内核尚未返回任何代理组")
	}
	return nil
}

func (c *Controller) setLastError(msg string) {
	c.setLastErrorCode("RUNTIME_OPERATION_FAILED", msg)
}

// setLastErrorCode 同 setLastError，但带上可编程错误码（F-A2-10）：
// 前端/日志可以按 code 路由，而不是靠文案匹配。
func (c *Controller) setLastErrorCode(code, msg string) {
	c.notify("error", code, msg, "appcore")
}

// notifyDedupWindow 是错误通知的去重时间窗。
const notifyDedupWindow = 30 * time.Second

// shouldSuppressNotify 判断这条 error 通知是否在去重窗口内已经发过。
func (c *Controller) shouldSuppressNotify(code, message string) bool {
	key := code + "|" + message
	now := time.Now()

	c.notifyMu.Lock()
	defer c.notifyMu.Unlock()

	if c.notifyLast == nil {
		c.notifyLast = make(map[string]time.Time)
	}
	if last, ok := c.notifyLast[key]; ok && now.Sub(last) < notifyDedupWindow {
		return true
	}
	c.notifyLast[key] = now

	// 顺手清理过期项，避免长期运行堆积
	if len(c.notifyLast) > 32 {
		for k, t := range c.notifyLast {
			if now.Sub(t) > notifyDedupWindow {
				delete(c.notifyLast, k)
			}
		}
	}
	return false
}

func (c *Controller) notify(level, code, message, source string) {
	if level == "error" && c.shouldSuppressNotify(code, message) {
		// 去重只抑制 UI 通知，不能连日志一起抑制：
		// 否则同一错误在 30s 内第二次出现时时间线直接断掉，复盘时看不到上下文。
		c.logWarn("[通知已去重] %s | code=%s | source=%s", message, code, source)
		return
	}
	if level == "error" {
		c.logError("%s | code=%s | source=%s", message, code, source)
	} else {
		c.logWarn("%s | code=%s | source=%s", message, code, source)
	}
	c.events.Emit(EventNotification, Notification{
		Level:   level,
		Code:    code,
		Message: message,
		Source:  source,
	})
	// F-A5-23：不再额外发一条 legacy "notify-error"——前端只订阅 app-notification，
	// 这条事件既没有消费者（死事件），又让同一条错误在事件流里出现两次（同名双发）。
	// 常量 EventNotifyError 保留，等所有调用方迁移完成后再删（不破坏既有绑定注册）。
}

// DisableAll 彻底清理并关闭所有功能。
// 用于程序退出等「尽快返回」的路径：不做 helper 停止确认，保持既有退出耗时。
func (c *Controller) DisableAll() {
	c.disableAll(context.Background(), false)
}

// disableAllWithConfirmation 用于运行期「关闭 TUN / 系统代理」的路径：
// 停内核后向 helper 确认停止结果（停止确认来自 helper），
// 主进程只清 ActualTun，不对系统路由/DNS 做任何清理。
func (c *Controller) disableAllWithConfirmation(ctx context.Context) {
	c.disableAll(ctx, true)
}

func (c *Controller) disableAll(ctx context.Context, confirmStop bool) {
	c.coreLifecycleMu.Lock()
	defer c.coreLifecycleMu.Unlock()

	// 先撤系统代理、再停内核（F-A5-13）：停内核可能一直阻塞到 stop-core 的完整预算
	// （helper 优雅 20s + 强杀 5s），而外层退出链路只给 5s/8s——一旦硬退，
	// 系统代理就会残留并指向已经死掉的端口，直接导致用户断网。
	// 撤代理是本地注册表操作（毫秒级），把它放到最前面最安全。
	// 只清「可判定为本程序写入」的代理，避免误伤其它代理工具（F-A2-07）。
	c.disableOwnedSystemProxyBestEffort()

	if confirmStop {
		if err := c.stopCoreForTransition(ctx); err != nil {
			// 停止未被确认：不要清运行态、更不要把"内核仍在跑"说成已关闭（F-A2-06）。
			// 保留 ActualTun 状态，由 watchdog 下一轮重试停止。
			c.logWarn("停止内核并确认停止失败，保留运行态待重试: %v", err)
			c.SyncState()
			return
		}
	} else {
		_ = c.stopCoreProcessLocked()
	}

	c.mu.Lock()
	c.sysProxyActive = false
	c.tunActive = false
	c.tunProbeOK = false
	c.tunProbeInfo = sys.TunAdapterInfo{}
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

// cleanupBeforeElevatedExit 是提权重启（os.Exit）前的快速清理。
//
// 只做本地能立刻完成的动作：停内核 + 摘系统代理 + 复位运行态。
// 用 3s 上限兜住偶发拖尾（测速内核、失效 helper），保证提权重启不会被无限期挂住；
// 超时后至少把系统代理摘掉——那是用户断网的直接原因。
func (c *Controller) cleanupBeforeElevatedExit() {
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				c.logWarn("提权退出前清理异常: %v", r)
			}
			close(done)
		}()
		c.disableAll(context.Background(), false)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		c.logWarn("提权退出前清理超时(3s)，直接摘除本程序的系统代理后继续提权")
		c.disableOwnedSystemProxyBestEffort()
	}
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

	timeout := time.NewTimer(10 * time.Second)
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
	// 仅构建运行时 YAML 并启动进程，强制禁用 TUN 以避免影响真实代理状态
	if err := clash.BuildRuntimeConfig(id, behavior.ActiveMode, behavior.LogLevel, false); err != nil {
		return err
	}
	return clash.Start(ctx, false) // delay 测试不走 TUN
}

// StopCoreProcess 仅停止物理进程，保留用户开启意图
func (c *Controller) StopCoreProcess() {
	c.coreLifecycleMu.Lock()
	defer c.coreLifecycleMu.Unlock()
	_ = c.stopCoreProcessLocked()
}

func (c *Controller) fillDesiredTarget(d *DesiredState) {
	b := c.Behavior.Get()
	d.ActiveConfig = b.ActiveConfig
	d.Mode = b.ActiveMode
}

// ToggleSystemProxy 开关：系统代理
func (c *Controller) ToggleSystemProxy(ctx context.Context, enable bool) error {
	return c.controls.SetSystemProxy(ctx, enable)
}

// ToggleSystemProxyFromCurrentDesired is used by the tray so the target is
// calculated under the same lock that commits the command.
func (c *Controller) ToggleSystemProxyFromCurrentDesired(ctx context.Context) error {
	return c.controls.ToggleSystemProxy(ctx)
}

// EnsureHelperReady 确保 helper 服务已安装并运行
// 使用 singleflight 合并并发调用
// EnsureHelperReady 保持历史签名（无 ctx），内部退化为 Background。
func (c *Controller) EnsureHelperReady(reason string) error {
	return c.EnsureHelperReadyWithContext(context.Background(), reason)
}

// EnsureHelperReadyWithContext 在调用方 ctx 到期时立即返回（F-A2-09）。
//
// 历史实现用 singleflight.Group.Do，无法被取消：一次 helper 强修复最坏 ≈45s，
// 会把 UI 的 25s 预算直接吃穿。这里改用 DoChan + select，超时即返回 ctx.Err()，
// 后台唤醒仍会继续（singleflight 是进程内共享的，不会因此丢任务）。
func (c *Controller) EnsureHelperReadyWithContext(ctx context.Context, reason string) error {
	ch := helperReadyGroup.DoChan("helper-ready", func() (any, error) {
		return nil, c.ensureHelperReadySlow(reason)
	})

	select {
	case res := <-ch:
		return res.Err
	case <-ctx.Done():
		return fmt.Errorf("等待后台服务就绪被取消（%s）: %w", reason, ctx.Err())
	}
}

func (c *Controller) ensureHelperReadySlow(reason string) error {
	client := sys.NewHelperClient()

	if err := client.Ping(); err == nil {
		// Helper already reachable — register this process so the watchdog
		// can detect a future abnormal exit.
		_ = client.RegisterParent(os.Getpid())
		return nil
	}

	// 1. 如果 Helper 已安装但处于停止状态，尝试直接通过 SCM 启动（当前用户 SID 已由 DACL 授权 SERVICE_START）
	if err := sys.StartHelperService(); err == nil {
		if err := sys.WaitForHelperReady(25, 200*time.Millisecond); err == nil {
			_ = client.RegisterParent(os.Getpid())
			c.logInfo("Helper 服务已通过 SCM 成功启动并就绪 (reason: %s)", reason)
			return nil
		}
	} else {
		c.logWarn("尝试通过 SCM 启动 Helper 服务未成功: %v (reason: %s)", err, reason)
	}

	// 2. 普通权限：无法直接启动时检查服务是否安装，返回对应引导错误
	if !sys.CheckAdmin() {
		helperStatus := sys.CheckHelperService()
		if !helperStatus.Installed {
			return ErrHelperInstallRequired
		}
		return ErrHelperRepairRequired
	}

	// 3. 管理员权限：执行强修复/安装
	helperExe := filepath.Join(utils.GetAppDir(), "GoclashZHelper.exe")
	sid, err := sys.CurrentUserSID()
	if err != nil {
		return fmt.Errorf("获取当前用户 SID 失败: %w", err)
	}

	if err := sys.RecoverHelperServiceForUser(helperExe, sid); err != nil {
		return fmt.Errorf("修复后台服务失败: %w", err)
	}

	// Register after successful recovery so the watchdog starts immediately.
	_ = client.RegisterParent(os.Getpid())
	c.logInfo("Helper 服务已自愈并就绪 (reason: %s)", reason)
	return nil
}

// ToggleTunMode 开关：TUN 模式
func (c *Controller) ToggleTunMode(ctx context.Context, enable bool) error {
	return c.controls.SetTun(ctx, enable)
}

// prepareTunEnable contains the prerequisites shared by every TUN entry
// point. It intentionally runs inside ControlCoordinator serialization so a
// tray command cannot bypass validation or race a concurrent control change.
func (c *Controller) prepareTunEnable(ctx context.Context) error {
	behavior := c.Behavior.Get()
	if behavior.ActiveConfig == "" {
		return ErrNoActiveConfig
	}

	// 1) helper：只做 ≤800ms 快 ping（F-A2-02）。
	// 探不到就转后台唤醒并继续——真正的「服务就绪 + 内核启动」由随后的事务/Reconcile 收敛，
	// 不再让"服务正在启动"把用户动作卡住几十秒（历史最坏 ≈45s）。
	client := sys.NewHelperClient()
	if fastHelperPing(client, 800*time.Millisecond) {
		_ = client.RegisterParent(os.Getpid())
	} else {
		c.logInfo("Helper 未在 800ms 内就绪，转后台唤醒并继续 TUN 启动流程")
		go func() {
			if err := c.EnsureHelperReady("tun-prepare-bg"); err != nil {
				c.logWarn("后台唤醒 Helper 失败: %v", err)
			}
		}()
	}

	// 2) 资产：只看快检（stat + PE 头）。只有「明确不可用」才拒绝，
	//    其余交给修复路径与适配器探测裁决——这里不再跑 95MB 哈希 + clash.exe -v。
	fastStatus := runtimeassets.GetStatusFast(ctx)
	w := fastStatus.Assets[runtimeassets.AssetWintun]
	core := fastStatus.Assets[runtimeassets.AssetCore]

	if fastAssetBlocked(w) {
		// 文案必须与前端 errors.ts 的分类规则对齐，否则会被归成 unknown（无任何引导）：
		//   "缺少 Wintun 依赖" → wintun_missing：引导安装驱动后自动重试；
		//   "wintun 已损坏"   → wintun_unavailable：只展示，不强行装驱动。
		switch w.ErrorCode {
		case runtimeassets.ErrMissing, runtimeassets.ErrIsDir:
			return fmt.Errorf("缺少 Wintun 依赖: %s (%s)", w.Error, w.Path)
		default:
			return fmt.Errorf("wintun 已损坏（%s）: %s (%s)", w.ErrorCode, w.Error, w.Path)
		}
	}
	if fastAssetBlocked(core) {
		// 内核问题不在这里硬拦：修复分支（EnsureReady + seed 自愈）会处理。
		c.logWarn("内核快检异常（继续尝试启动，修复交给后续流程）: %s path=%s", core.Error, core.Path)
	}

	return nil
}

// ToggleTunFromCurrentDesired is the TUN equivalent of the tray-safe toggle.
func (c *Controller) ToggleTunFromCurrentDesired(ctx context.Context) error {
	return c.controls.ToggleTun(ctx)
}

// RestartCore 重启内核（默认为内部调用原因）。
// 对外统一收敛到 ControlCoordinator —— 它是唯一的 runtime 操作入口。
func (c *Controller) RestartCore(ctx context.Context) error {
	return c.controls.RestartCore(ctx, "internal")
}

// RestartCoreWithReason 是重启内核的对外入口，统一收敛到 ControlCoordinator
// （保留导出的方法名是为了兼容 app.go / 托盘等既有调用点）。
// 真正的重启实现在内部的 restartCoreWithReason：
// supervisor 调和时会直接调用它，避免在已持有协调器锁的事务中重入协调器造成死锁。
func (c *Controller) RestartCoreWithReason(ctx context.Context, reason string) error {
	return c.controls.RestartCore(ctx, reason)
}

// restartCoreWithReason 是重启内核的内部实现（不再作为对外入口）。
// 调用方必须已具备串行化语义：ControlCoordinator 事务，或 supervisor 的调和锁。
func (c *Controller) restartCoreWithReason(ctx context.Context, reason string) error {
	if err := c.guardDisruptiveAction(reason); err != nil {
		c.queuePendingDisruptiveAction(reason)
		return err
	}

	c.coreLifecycleMu.Lock()
	defer c.coreLifecycleMu.Unlock()

	wasRunning := clash.IsRunning()
	// 停止内核（TUN 宿主随内核一起消失），helper 托管时向 helper 确认停止结果；
	// 主进程不做任何路由/适配器清理。
	// 确认失败不阻断重启（helper 可能暂时不可达），仅记录告警，保持既有重试语义。
	if err := c.stopCoreForTransition(ctx); err != nil {
		c.logWarn("停止内核/helper 停止确认失败，继续按重启流程处理: %v", err)
	}

	if wasRunning {
		// 留出 250ms 缓冲期，确保 Windows 操作系统完成对旧内核 9090 和 7890 端口的 TCP TIME_WAIT 状态回收，
		// 避免新内核瞬间拉起时遭遇 bind: address already in use 从而直接崩溃。
		time.Sleep(250 * time.Millisecond)
	}

	desired := c.Desired.Get()
	if desired.ActiveConfig == "" {
		behavior := c.Behavior.Get()
		desired.ActiveConfig = behavior.ActiveConfig
		desired.Mode = behavior.ActiveMode
	}

	needCore := desired.CoreRunning || desired.SystemProxy || desired.Tun

	// 无论是否开启代理，都强行启动内核一次以验证配置正确性
	if err := c.ensureCoreRunningWithDesiredState(ctx, desired); err != nil {
		c.SyncState()
		return err
	}

	if !needCore {
		// 如果原本未开启代理服务，则启动验证成功后立刻关闭，不驻留后台
		_ = c.stopCoreProcessLocked()
	}

	// 发送通用的重启信号
	c.events.Emit("core-restarted", map[string]any{
		"reason": reason,
	})

	// 🛡️ 核心改进：仅在用户明确意图或核心配置变更时，才通知前端清理延迟缓存
	switch reason {
	case "manual", "config-switch", "subscription-update", "restore", "config-save":
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
		c.notify("error", "MODE_SAVE_FAILED", "模式持久化保存失败: "+err.Error(), "appcore")
		c.SyncState()
		return err
	}

	if err := c.Desired.Update(func(d *DesiredState) {
		d.Mode = mode
		c.fillDesiredTarget(d)
	}); err != nil {
		// Behavior is already persisted; a desired-state save failure leaves
		// Behavior, kernel and desired state inconsistent.  Surface the error
		// so the caller can retry rather than silently diverging.
		c.SyncState()
		return fmt.Errorf("模式切换 desired state 保存失败: %w", err)
	}

	// 2. 如果内核正在运行，尝试通过 API 热切换
	if clash.IsRunning() {
		if err := clash.UpdateModeWithContext(ctx, mode); err != nil {
			c.SyncState()
			return fmt.Errorf("内核模式切换失败: %v", err)
		}

		st := c.runtimeState.Get()
		if st.Running && st.Purpose == "user" {
			st.Mode = mode
			c.runtimeState.Set(st)
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

	c.logInfo("Triggering auto delay test, reason: %s", reason)
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

	if old.AppLogLevel != next.AppLogLevel || old.Language != next.Language {
		c.SyncState()
	}

	if old.LogLevel != next.LogLevel {
		c.SyncState()
	}

	if old.ProxyTrafficOnly != next.ProxyTrafficOnly {
		c.traffic.Stop()
		c.traffic.ResetRuntimeState()
		c.events.Emit("traffic-stat-mode-changed", next.ProxyTrafficOnly)
	}

	if old.StartupWithOS != next.StartupWithOS {
		if err := c.handleStartupWithOSChange(next.StartupWithOS); err != nil {
			// 操作失败，回滚内存和持久化配置
			next.StartupWithOS = old.StartupWithOS
			_ = c.Behavior.SetAndSave(next)
			c.SyncState()
			return fmt.Errorf("配置开机自启失败: %w", err)
		}
	}

	// 端口代理模式被关闭时，清零 desired.PortProxy 并触发 reconcile，
	// 防止 desired.PortProxy=true 残留导致内核无法停止
	if old.PortProxyMode && !next.PortProxyMode {
		_ = c.Desired.Update(func(d *DesiredState) {
			d.PortProxy = false
		})
		c.Supervisor.ReconcileAsync("port-proxy-mode-disabled")
	}

	c.SyncState()
	return nil
}

func (c *Controller) handleStartupWithOSChange(enable bool) error {
	exePath, err := os.Executable()
	if err != nil {
		c.events.Emit("app-state-sync", c.GetAppState())
		return fmt.Errorf("无法获取程序执行路径: %w", err)
	}

	if enable {
		if createTaskErr := sys.CreateStartupTask(exePath); createTaskErr != nil {
			c.setLastErrorCode("STARTUP_TASK_CREATE_FAILED", "设置开机自启失败: "+createTaskErr.Error())
			c.events.Emit("app-state-sync", c.GetAppState())
			return createTaskErr
		}
	} else {
		if deleteTaskErr := sys.DeleteStartupTask(); deleteTaskErr != nil {
			c.setLastErrorCode("STARTUP_TASK_DELETE_FAILED", "关闭开机自启失败: "+deleteTaskErr.Error())
			c.events.Emit("app-state-sync", c.GetAppState())
			return deleteTaskErr
		}
	}

	return nil
}

func (c *Controller) StopTrafficStream() {
	c.traffic.Stop()
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

func (c *Controller) GetRecentLogs() []logger.LogEntry {
	return logger.AppLogs.GetAll()
}

func (c *Controller) SearchLogs(keyword string) []logger.LogEntry {
	return logger.AppLogs.Search(keyword)
}

func (c *Controller) ClearLogs() {
	logger.AppLogs.Clear()
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
		c.logError("进入全局模式同步失败: %v", err)
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
		c.logError("读取内核代理组失败，跳过节点选择回放: %v", err)
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
				c.logError("回放 GLOBAL 出口失败: %v", err)
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

// GetDiagnosticInfo returns current diagnostic info
func (c *Controller) GetDiagnosticInfo() DiagnosticInfo {
	return GetDiagnosticInfo()
}

// ExportDiagnosticsWithPath 弹出保存对话框并导出诊断包，返回**实际落盘路径**。
//
// F-A3-15：旧实现（error-only）在"用户取消"与"导出成功"两种情况下都返回 nil，
// 前端无法区分，取消后会提示"导出成功"。这里约定：
//   - 成功：返回最终路径（已规范化为 .zip）+ nil；
//   - 用户取消：返回 "" + nil；
//   - 失败：返回 "" + error。
func (c *Controller) ExportDiagnosticsWithPath() (string, error) {
	path, err := application.Get().Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
		Filename: "goclashz_diagnostics.zip",
		Title:    "导出诊断包",
		Filters: []application.FileFilter{
			{DisplayName: "ZIP Archives (*.zip)", Pattern: "*.zip"},
		},
	}).PromptForSingleSelection()
	if err != nil {
		// v3 在用户取消时返回 cfd.ErrorCancelled（"cancelled by user"），
		// v2 取消时返回 nil；归一化为「取消 = 空路径 + 无错误」。
		if strings.Contains(strings.ToLower(err.Error()), "cancelled by user") {
			return "", nil
		}
		return "", err
	}
	if path == "" {
		return "", nil // cancelled or empty selection
	}

	// ExportDiagnosticsToFile 会在缺省 .zip 时补后缀；这里同步算出最终路径用于回传。
	finalPath := path
	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(finalPath)), ".zip") {
		finalPath += ".zip"
	}

	if err := ExportDiagnosticsToFile(finalPath); err != nil {
		return "", err
	}
	return finalPath, nil
}

// ExportDiagnostics 是保留旧签名的兼容壳（绑定层与 App 层尚未切换，见主审）。
// 需要区分"取消/成功"或拿路径的调用方请改用 ExportDiagnosticsWithPath。
func (c *Controller) ExportDiagnostics() error {
	_, err := c.ExportDiagnosticsWithPath()
	return err
}

// GetConfigDeviations 返回当前设置中「保存了但不会按字面生效」的配置项，module 为空表示全部模块
func (c *Controller) GetConfigDeviations(module string) ([]clash.ConfigDeviation, error) {
	return clash.GetConfigDeviations(module)
}

// ExplainDNS 静态推演某个域名会用哪个 DNS 解析器（不发起真实查询）
func (c *Controller) ExplainDNS(domain string) (*clash.DNSExplain, error) {
	return clash.ExplainDNS(domain)
}

// ApplyUwpExemptionChanges 只按传入的差异增删 UWP 环回豁免，绝不触碰其它已豁免项
func (c *Controller) ApplyUwpExemptionChanges(added []string, removed []string) error {
	return sys.ApplyUwpExemptionChanges(added, removed)
}

// ClearAllUwpExemptions 清空全部 UWP 豁免：只删除本次枚举到的已豁免 SID
func (c *Controller) ClearAllUwpExemptions() error {
	return sys.ClearAllUwpExemptions()
}
