package main

import (
	"context"
	_ "embed" // 👈 1. 新增：导入 embed 包用于打包图标
	"encoding/json"
	"fmt"
	"goclashz/core/clash"
	"goclashz/core/logger"
	"goclashz/core/sys"
	"goclashz/core/traffic"
	"goclashz/core/utils"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"net/http"

	"github.com/getlantern/systray" // 👈 2. 新增：引入托盘库
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/windows/icon.ico
var iconData []byte // 👈 3. 新增：将图标编译进二进制文件中给托盘使用

type App struct {
	ctx            context.Context
	cancelTraffic  context.CancelFunc
	cancelLogs     context.CancelFunc
	logGen         int // 🚀 新增：日志流版本号，用于区分新旧协程
	logRunning     bool
	mu             sync.RWMutex
	activeConfig   string
	activeMode     string
	offlineNodes   map[string]string
	offlineMu      sync.RWMutex // 🚀 新增：专门保护 offlineNodes 的读写锁
	sysProxyActive bool
	tunActive      bool

	// 👇 新增：用来控制托盘打勾状态的指针
	mSysProxy *systray.MenuItem
	mTun      *systray.MenuItem

	// 👇 新增：用来控制出站路由托盘打勾状态的指针
	mModeRule   *systray.MenuItem
	mModeGlobal *systray.MenuItem
	mModeDirect *systray.MenuItem

	// 👈 新增：专用于应用行为配置的内存缓存及读写锁
	behaviorCache AppBehavior
	behaviorMu    sync.RWMutex
	updateMu      sync.Mutex // 👈 新增：全局统一的组件更新锁，防止任何更新互相冲突

	// 👇 新增：内核启停锁，防止并发操作导致端口抢占
	coreLifecycleMu sync.Mutex

	// 🚀 新增：主题缓存，终结高频 I/O
	themeCache string

	// 🔐 新增：专属 IO 锁，确保磁盘文件原子性写入，防止并发保存导致数据损坏
	behaviorIOMu sync.Mutex

	testCancel context.CancelFunc // 👈 测速任务的取消句柄
	testMu     sync.Mutex         // 👈 保护取消句柄的锁
}

// AppBehavior 定义应用行为设置
type AppBehavior struct {
	SilentStart bool   `json:"silentStart"` // 静默启动 (不弹窗，直接进托盘)
	CloseToTray bool   `json:"closeToTray"` // 点击关闭时隐藏到托盘
	LogLevel    string `json:"logLevel"`    // 日志等级
	HideLogs    bool   `json:"hideLogs"`
	SubUA       string `json:"subUA"` // 订阅更新 User-Agent
	// 新增：统持久化字段
	ActiveConfig string `json:"activeConfig"`
	ActiveMode   string `json:"activeMode"`
	// 👇 新增：规则数据库的下载链接持久化
	GeoIpLink   string `json:"geoIpLink"`
	GeoSiteLink string `json:"geoSiteLink"`
	MmdbLink    string `json:"mmdbLink"`
	AsnLink     string `json:"asnLink"`
}

// 1. 获取离线节点记忆文件的路径
func (a *App) getOfflineNodesPath() string {
	return filepath.Join(utils.GetDataDir(), "offline_nodes.json")
}

// 2. 将内存中的节点选择持久化到磁盘
func (a *App) saveOfflineNodes() {
	a.offlineMu.RLock()
	data, err := json.MarshalIndent(a.offlineNodes, "", "  ")
	a.offlineMu.RUnlock()
	if err == nil {
		os.WriteFile(a.getOfflineNodesPath(), data, 0644)
	}
}

// 3. 启动时从磁盘读取记忆
func (a *App) loadOfflineNodes() {
	data, err := os.ReadFile(a.getOfflineNodesPath())
	if err == nil {
		a.offlineMu.Lock()
		if a.offlineNodes == nil {
			a.offlineNodes = make(map[string]string)
		}
		json.Unmarshal(data, &a.offlineNodes)
		a.offlineMu.Unlock()
	}
}

// MarkNodeOffline 安全地标记节点（或离线选择状态）
func (a *App) MarkNodeOffline(groupName string, nodeName string) {
	a.offlineMu.Lock()
	defer a.offlineMu.Unlock()
	if a.offlineNodes == nil {
		a.offlineNodes = make(map[string]string)
	}
	a.offlineNodes[groupName] = nodeName
}

// ClearOfflineNodes 安全地清空离线记录（切换配置时调用）
func (a *App) ClearOfflineNodes() {
	a.offlineMu.Lock()
	defer a.offlineMu.Unlock()
	a.offlineNodes = make(map[string]string)
}

// IsNodeOffline 安全地查询离线选择状态
func (a *App) IsNodeOffline(groupName string) (bool, string) {
	a.offlineMu.RLock()
	defer a.offlineMu.RUnlock()
	if a.offlineNodes == nil {
		return false, ""
	}
	node, exists := a.offlineNodes[groupName]
	return exists, node
}

// 内部初始化缓存的方法，在 startup 中调用
func (a *App) initBehaviorCache() {
	defaultConfig := AppBehavior{
		SilentStart:  false,
		CloseToTray:  false,
		LogLevel:     "error",
		HideLogs:     true,
		SubUA:        "clash-verge",
		ActiveConfig: "1776940878659",
		ActiveMode:   "rule",
		GeoIpLink:    "https://ghproxy.net/https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb",
		GeoSiteLink:  "https://ghproxy.net/https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat",
		MmdbLink:     "https://ghproxy.net/https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb",
		AsnLink:      "https://ghproxy.net/https://github.com/xishang0128/geoip/releases/download/latest/GeoLite2-ASN.mmdb",
	}

	// 自动处理读取、合并和生成默认文件
	cfg, _ := utils.LoadSetting("behavior", defaultConfig)

	a.behaviorMu.Lock()
	a.behaviorCache = *cfg
	a.behaviorMu.Unlock()
}

// GetAppBehavior 供前端获取当前设置 (Wails 绑定方法)
func (a *App) GetAppBehavior() AppBehavior {
	a.behaviorMu.RLock()
	defer a.behaviorMu.RUnlock()
	return a.behaviorCache
}

// 修改保存逻辑，写盘的同时更新缓存
// SaveAppBehavior 修改保存逻辑，锁内更新快照，锁外异步写盘
func (a *App) SaveAppBehavior(config AppBehavior) error {
	// 1. 锁内更新内存缓存，并深拷贝一份用于安全的写盘
	a.behaviorMu.Lock()
	oldLogLevel := a.behaviorCache.LogLevel // 👈 新增：记录旧的日志等级
	a.behaviorCache = config
	behaviorToSave := a.behaviorCache
	a.behaviorMu.Unlock()

	// 2. 🔐 使用专属 IO 锁确保文件原子性串行写入，防止快照被旧携程覆盖
	a.behaviorIOMu.Lock()
	err := utils.SaveSetting("behavior", &behaviorToSave)
	a.behaviorIOMu.Unlock()

	// 3. 广播与同步
	runtime.EventsEmit(a.ctx, "behavior-changed", behaviorToSave)

	active := a.getActiveConfig()
	if active != "" {
		mode := a.getActiveMode()
		clash.BuildRuntimeConfig(active, mode, behaviorToSave.LogLevel)
		if clash.IsRunning() {
			clash.ReloadConfig()
		}
	}

	// 👇 核心修复：如果日志等级发生变化，且当前前端正在查看日志（流正在运行），立刻重启日志流！
	if oldLogLevel != behaviorToSave.LogLevel {
		a.mu.Lock()
		isLogging := a.logRunning
		a.mu.Unlock()
		if isLogging {
			a.StopStreamingLogs()
			// 稍微休眠，确保底层网络请求句柄已经彻底关闭
			time.Sleep(50 * time.Millisecond)
			a.StartStreamingLogs()
		}
	}

	a.SyncState()
	return err
}

// GetLocalConfigs 获取订阅列表
func (a *App) GetLocalConfigs() []clash.SubIndexItem {
	clash.IndexLock.RLock()
	defer clash.IndexLock.RUnlock()
	return clash.SubIndex
}

// ProxyStatus 新增给前端返回的双重状态结构

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
	// 🚀 新增：让前端实时知道当前在跑哪个配置
	ActiveConfig     string `json:"activeConfig"`
	ActiveConfigName string `json:"activeConfigName"`
	ActiveConfigType string `json:"activeConfigType"`
}

// 1. 在 app.go 任意位置新增这个辅助方法，用于将离线缓存合并到数据源
func (a *App) mergeOfflineNodes(data map[string]interface{}) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if groups, ok := data["groups"].(map[string]interface{}); ok {
		for gName, groupData := range groups {
			if gMap, ok2 := groupData.(map[string]interface{}); ok2 {
				// 优先使用离线选择
				if exists, selNode := a.IsNodeOffline(gName); exists {
					gMap["now"] = selNode
				}
				// 兜底：没有当前选中项，默认选中第一项
				if gMap["now"] == "" {
					if lenRaw, has := gMap["all"]; has {
						if allArr, ok3 := lenRaw.([]string); ok3 && len(allArr) > 0 {
							gMap["now"] = allArr[0]
						}
					}
				}
			}
		}
	}
}

// saveActiveConfig 记录当前选中的配置文件名到本地
func (a *App) saveActiveConfig(fileName string) {
	// 🚀 1. 将读取、判断、修改全部放入锁闭环，杜绝 Read-Modify-Write 竞态漏洞
	a.behaviorMu.Lock()

	// ⚡ 防抖：如果没变，直接释放锁并返回
	if a.behaviorCache.ActiveConfig == fileName {
		a.behaviorMu.Unlock()
		return
	}

	a.behaviorCache.ActiveConfig = fileName
	// 🚀 2. 深拷贝一份结构体用于写盘
	behaviorToSave := a.behaviorCache

	// 🚀 3. 立即释放锁，不要让缓慢的磁盘 IO 阻塞其他协程
	a.behaviorMu.Unlock()

	// 4. 🔐 串行化 IO，杜绝并发写文件
	a.behaviorIOMu.Lock()
	utils.SaveSetting("behavior", &behaviorToSave)
	a.behaviorIOMu.Unlock()
}

// 启动时读取上次选中的配置文件名
func (a *App) loadActiveConfig() string {
	return a.GetAppBehavior().ActiveConfig
}

// saveActiveMode 记录当前选中的模式到本地
func (a *App) saveActiveMode(mode string) {
	a.behaviorMu.Lock()

	if a.behaviorCache.ActiveMode == mode {
		a.behaviorMu.Unlock()
		return
	}

	a.behaviorCache.ActiveMode = mode
	behaviorToSave := a.behaviorCache

	a.behaviorMu.Unlock()

	// 🔐 使用专属 IO 锁确保文件原子性串行写入
	a.behaviorIOMu.Lock()
	utils.SaveSetting("behavior", &behaviorToSave)
	a.behaviorIOMu.Unlock()
}

// 启动时读取上次选中的模式
func (a *App) loadActiveMode() string {
	mode := a.GetAppBehavior().ActiveMode
	if mode == "" {
		return "rule"
	}
	return mode
}

// --- 状态获取辅助方法（新增） ---

func (a *App) getActiveConfig() string {
	a.mu.RLock()
	cfg := a.activeConfig
	a.mu.RUnlock()

	if cfg != "" {
		return cfg
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeConfig == "" { // 👈 二次校验，防止多协程重复读文件
		a.activeConfig = a.loadActiveConfig()
	}
	return a.activeConfig
}

func (a *App) getActiveMode() string {
	a.mu.RLock()
	mode := a.activeMode
	a.mu.RUnlock()

	if mode != "" {
		return mode
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeMode == "" {
		a.activeMode = a.loadActiveMode()
	}
	return a.activeMode
}

// --- 底层：确保内核运行 ---
func (a *App) ensureCoreRunning() error {
	if clash.IsRunning() {
		return nil
	}

	// 👈 核心修复：使用安全方法获取并初始化状态，不再需要手动加锁
	mode := a.getActiveMode()
	activeCfg := a.getActiveConfig()

	// 👈 核心：利用已有的配置生成流水线，直接根据模板重写 config.yaml (含模式、TUN、端口等)
	if activeCfg != "" {
		if err := clash.BuildRuntimeConfig(activeCfg, mode, a.GetAppBehavior().LogLevel); err != nil {
			fmt.Printf("生成运行时配置警告: %v\n", err)
		}
	}

	// 启动内核
	if err := clash.Start(a.ctx); err != nil {
		return err
	}

	// 🚀 新增：内核一经启动（即使 API 还没就绪），立刻同步状态，让前端的“接管中”秒级点亮！
	a.SyncState()

	// ⚠️ 核心修复 2：优化探针逻辑，先检查再等待
	apiReady := false
	for i := 0; i < 20; i++ { // 最长等待 2 秒
		// 1. 探针：尝试请求一次数据，如果成功说明 API 已就绪
		if _, err := clash.GetInitialData(); err == nil {
			apiReady = true
			break
		}

		// 2. 关键：检查内核进程是否还在运行
		if !clash.IsRunning() {
			return fmt.Errorf("内核启动后意外停止。请检查端口(7890/9090)是否被占用，或查看日志以获取详细配置错误。")
		}

		// 3. 只有失败了才等待
		time.Sleep(100 * time.Millisecond)
	}

	// API 就绪后，立刻下发离线选择的节点
	if apiReady {
		a.mu.Lock()
		if len(a.offlineNodes) > 0 {
			for g, n := range a.offlineNodes {
				_ = clash.SelectProxy(g, n)
			}
		}
		a.mu.Unlock()
	}

	// 3. 启动流量监控
	go a.StartTrafficStream()
	return nil
}

// --- 底层：停止内核 ---
func (a *App) stopCoreService() {
	if clash.IsRunning() {
		if data, err := clash.GetInitialData(); err == nil {
			// 🎯 精准修复：使用 map[string]interface{} 正确解析 JSON 对象
			if groups, ok := data["groups"].(map[string]interface{}); ok {
				for gName, gData := range groups {
					if gMap, ok2 := gData.(map[string]interface{}); ok2 {
						if now, ok3 := gMap["now"].(string); ok3 && now != "" {
							a.MarkNodeOffline(gName, now) // 安全写入离线记录
						}
					}
				}
				a.saveOfflineNodes() // 存入磁盘
			}
		}
	}

	clash.Stop()
	a.StopTrafficStream() // 👈 上一步修改的函数，这里会自动触发流量归零

	// 🚀 新增：内核一经停止，立刻同步状态，消除前端“服务停止”状态的延迟感！
	a.SyncState()
}

// ==========================================
// --- 暴露给前端的 API ---
// ==========================================

// ToggleSystemProxy 开关 1：系统代理
func (a *App) ToggleSystemProxy(enable bool) error {
	defer a.SyncState()              // 🚀 无论成功失败，退出函数时强制刷新 UI 状态，防止前端卡死在错误位置
	a.coreLifecycleMu.Lock()         // 🔒 加锁
	defer a.coreLifecycleMu.Unlock() // 🔓 退出时自动解锁

	a.mu.Lock()
	a.sysProxyActive = enable
	needCore := a.sysProxyActive || a.tunActive
	a.mu.Unlock()

	if enable {
		// 1. 确保底层内核在运行
		if err := a.ensureCoreRunning(); err != nil {
			a.mu.Lock()
			a.sysProxyActive = false
			a.mu.Unlock()
			return err
		}

		// ✅ 核心修复：动态读取真实端口
		proxyPort := 7890 // 默认兜底端口
		if netCfg, err := clash.GetNetworkConfig(); err == nil && netCfg != nil {
			if netCfg.MixedPort != 0 {
				proxyPort = netCfg.MixedPort
			} else if netCfg.Port != 0 {
				proxyPort = netCfg.Port
			}
		}

		// 2. 开启 Windows 系统代理
		bypass := "localhost;127.*;10.*;172.16.*;192.168.*;<local>"
		err := sys.EnableSystemProxy("127.0.0.1", proxyPort, bypass)
		a.SyncState()
		return err
	} else {
		// 1. 关闭 Windows 系统代理
		sys.DisableSystemProxy()
		// 2. 如果虚拟网卡也没开，那就彻底关闭内核节约资源
		if !needCore {
			a.stopCoreService()
		}
		a.SyncState()
		return nil
	}
}

// ToggleTunMode 开关 2：虚拟网卡 (TUN)
func (a *App) ToggleTunMode(enable bool) error {
	defer a.SyncState()              // 🚀 防御性同步：确保 UI 状态始终回滚到真实后端状态
	a.coreLifecycleMu.Lock()         // 🔒 加锁
	defer a.coreLifecycleMu.Unlock() // 🔓 退出时自动解锁

	if enable {
		if !sys.IsWintunInstalled() {
			return fmt.Errorf("缺失 Wintun 驱动，请先在设置中安装")
		}
		if !sys.CheckAdmin() {
			errContext := "TUN 模式必须以管理员身份运行。请右键 GoclashZ 图标，选择「以管理员身份运行」。"
			// 1. 发送通知给前端弹出 Error Toast
			runtime.EventsEmit(a.ctx, "notify-error", errContext)
			// 2. 强制同步一次正确状态给前端
			a.SyncState()
			return fmt.Errorf("permission denied")
		}
	}

	// 修改配置文件的 TUN 状态
	tunCfg, _ := clash.GetTunConfig()
	if tunCfg == nil {
		tunCfg = &clash.TunConfig{Stack: "gvisor", AutoRoute: true, StrictRoute: true}
	}
	tunCfg.Enable = enable
	if err := clash.UpdateTunConfig(tunCfg); err != nil {
		return err
	}

	a.mu.Lock()
	a.tunActive = enable
	needCore := a.sysProxyActive || a.tunActive
	a.mu.Unlock()

	// TUN 模式的改变必须重启内核才能生效
	a.stopCoreService()

	if needCore {
		err := a.ensureCoreRunning()
		a.SyncState()
		return err
	}
	a.SyncState()
	return nil
}

// RestartCore 供前端调用：安全地重启底层代理内核
func (a *App) RestartCore() error {
	// 🔒 加锁：防止在重启过程中，用户又点击了托盘或界面的其他开关
	a.coreLifecycleMu.Lock()
	defer a.coreLifecycleMu.Unlock()

	// 1. 停止当前运行的内核与流量监控
	a.stopCoreService()

	// 2. 读取当前应用的接管状态
	a.mu.RLock()
	needCore := a.sysProxyActive || a.tunActive
	a.mu.RUnlock()

	// 3. 如果系统代理或 TUN 至少开了一个，则重新启动内核
	if needCore {
		if err := a.ensureCoreRunning(); err != nil {
			a.SyncState() // 即使失败也要推一次状态
			return fmt.Errorf("内核重启失败: %v", err)
		}
	}

	// 4. 同步最新状态给前端
	a.SyncState()
	return nil
}

func NewApp() *App {
	app := &App{
		offlineNodes:   make(map[string]string),
		activeMode:     "", // 留空，待 loadActiveMode 加载
		sysProxyActive: false,
		tunActive:      false,
	}
	app.loadOfflineNodes() // 👈 新增：启动时加载离线选择记录
	return app
}

// 清理早期版本遗留的废弃配置文件
func (a *App) cleanLegacyFiles() {
	binDir := utils.GetCoreBinDir()
	_ = os.Remove(filepath.Join(binDir, "active_config.txt"))
	_ = os.Remove(filepath.Join(binDir, "active_mode.txt"))

	// 🚀 启动时静默清理上次内核更新产生的 .old 垃圾文件
	_ = os.Remove(filepath.Join(binDir, "mihomo-windows-amd64.exe.old"))
	_ = os.Remove(filepath.Join(binDir, "clash.exe.old"))
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	clash.LoadIndex() // 🚀 初始化加载订阅索引

	// 🚀 核心修复：静默兜底清理
	// 专门对付上次应用意外崩溃、蓝屏、被强制结束，导致注册表代理未关闭而断网的极端情况
	go func() {
		_ = sys.ClearSystemProxy()
	}()

	a.cleanLegacyFiles()  // 🚀 新增：静默扫除历史垃圾文件
	a.initBehaviorCache() // 👈 新增：初始化配置缓存

	// 🚀 初始化主题缓存
	themeData, err := os.ReadFile(getThemeConfigPath())
	if err == nil && len(themeData) > 0 {
		a.themeCache = strings.TrimSpace(string(themeData))
	} else {
		a.themeCache = "dark" // 🚀 默认黑色模式
	}

	// 读取行为配置
	config := a.GetAppBehavior()

	// 判断是否静默启动
	if !config.SilentStart {
		runtime.WindowShow(ctx)
	} else {
		// 👈 核心修复：如果是静默启动，明确强制调用隐藏，防止组件初始化闪烁或暴露
		runtime.WindowHide(ctx)
	}

	// 初始化完成后推一次状态给前端
	a.SyncState()

	// 启动后台守护任务 (Goroutine)
	go a.startDaemonTasks()

	// 不要在这里直接执行 systray.Run
	// 利用 Goroutine 和极短的延迟，避开 WebView2 最消耗资源的初始化瞬间
	go func() {
		time.Sleep(500 * time.Millisecond)
		a.SetupSystray()
	}()
}

func (a *App) startDaemonTasks() {
	// 设定定时器：比如每 12 小时更新订阅，每 30 分钟测速
	subTicker := time.NewTicker(12 * time.Hour)
	speedTicker := time.NewTicker(30 * time.Minute)

	defer func() {
		subTicker.Stop()
		speedTicker.Stop()
	}()

	for {
		select {
		case <-subTicker.C:
			// 增加安全锁：每次后台静默更新，最多允许执行 2 分钟，防止 HTTP 卡死
			updateCtx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)

			// 开启一个 Goroutine 执行，并配合 Context
			go func(ctx context.Context) {
				defer cancel()
				// 执行订阅更新逻辑，透传 ctx 以便在超时或关机时能立刻中断 HTTP
				err := a.updateAllSubs(ctx)
				if err == nil {
					runtime.EventsEmit(a.ctx, "subs-background-updated")
				}
			}(updateCtx)

		case <-speedTicker.C:
			// 预留后台测速占位

		case <-a.ctx.Done(): // 收到软件完全退出信号
			return // 立刻退出守护协程，绝不拖泥带水
		}
	}
}

func (a *App) shutdown(ctx context.Context) {
	// 1. 关闭系统代理 (优雅退出，还网于民)
	_ = sys.ClearSystemProxy()

	// 2. 停止内核
	clash.Stop()

	// 3. 停止流量监控
	a.StopTrafficStream()

	// 🚀 修复 1：彻底消灭 Wails 重启或退出时留下的“点不动的幽灵图标”
	systray.Quit()
}

// --- 代理核心控制 ---

// CheckTunEnv 提供给前端：检查 TUN 模式环境（驱动 + 权限）
func (a *App) CheckTunEnv() map[string]bool {
	return map[string]bool{
		"isAdmin":   sys.CheckAdmin(),
		"hasWintun": sys.IsWintunInstalled(),
	}
}

// ElevatePrivileges 提供给前端：自动提权并重启应用
func (a *App) ElevatePrivileges() error {
	return sys.RequestAdmin() // 将会呼出 UAC 窗口并重启软件
}

// --- 配置与测速 ---

func (a *App) GetInitialData() (map[string]interface{}, error) {
	activeConfig := a.getActiveConfig()
	mode := a.getActiveMode()

	var data map[string]interface{}

	// 1. 获取基础数据
	if !clash.IsRunning() {
		offlineData, err := clash.GetOfflineData(activeConfig)
		if err != nil {
			data = map[string]interface{}{"mode": mode, "groups": make(map[string]interface{}), "activeConfig": activeConfig, "isOffline": true}
		} else {
			a.mergeOfflineNodes(offlineData)
			offlineData["activeConfig"] = activeConfig
			offlineData["mode"] = mode
			offlineData["isOffline"] = true
			data = offlineData
		}
	} else {
		apiData, err := clash.GetInitialData()
		if err != nil {
			fallbackData, _ := clash.GetOfflineData(activeConfig)
			if fallbackData != nil {
				a.mergeOfflineNodes(fallbackData)
				fallbackData["activeConfig"] = activeConfig
				fallbackData["isOffline"] = true
				fallbackData["mode"] = mode
				data = fallbackData
			} else {
				data = map[string]interface{}{"mode": mode, "groups": make(map[string]interface{}), "activeConfig": activeConfig, "isOffline": true}
			}
		} else {
			apiData["activeConfig"] = activeConfig
			apiData["mode"] = mode
			apiData["isOffline"] = false
			data = apiData
		}
	}

	// 2. 🚀 核心修复：无论内核是否运行，统一下发配置名称和类型！
	clash.IndexLock.RLock()
	for _, item := range clash.SubIndex {
		if item.ID == activeConfig {
			data["activeConfigName"] = item.Name
			data["activeConfigType"] = item.Type
			break
		}
	}
	clash.IndexLock.RUnlock()

	// 3. 统一下发排序信息
	configPath := filepath.Join(utils.GetSubscriptionsDir(), activeConfig+".yaml")
	if activeConfig == "" || activeConfig == "config.yaml" {
		configPath = clash.GetConfigPath() // 👈 复用标准方法，消除硬编码
	}
	if yamlData, err := os.ReadFile(configPath); err == nil {
		data["groupOrder"] = clash.ExtractGroupOrder(yamlData)
	}

	return data, nil
}

func (a *App) TestAllProxies(nodeNames []string) {
	// 🚀 优化：掐断历史遗留的、还在疯狂测速的旧任务
	a.testMu.Lock()
	if a.testCancel != nil {
		a.testCancel()
	}
	// 创建仅针对本轮测速的独立 Context
	ctx, cancel := context.WithCancel(a.ctx)
	a.testCancel = cancel
	a.testMu.Unlock()

	a.coreLifecycleMu.Lock()

	isSilentTest := false

	// 1. 如果内核没有运行，执行【后台静默启动】
	if !clash.IsRunning() {
		mode := a.getActiveMode()
		activeCfg := a.getActiveConfig()
		if activeCfg != "" {
			clash.BuildRuntimeConfig(activeCfg, mode, a.GetAppBehavior().LogLevel)
		}

		// 直接调用底层 Start，不调用 ensureCoreRunning，避免触发流量监控和 UI 同步
		if err := clash.Start(a.ctx); err != nil {
			a.coreLifecycleMu.Unlock()
			runtime.EventsEmit(a.ctx, "proxy-test-finished", "后台静默启动内核失败，无法测速")
			return
		}

		// 探针等待 API 就绪
		for i := 0; i < 20; i++ {
			if _, err := clash.GetInitialData(); err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		isSilentTest = true
	}
	a.coreLifecycleMu.Unlock() // 尽早释放锁，不阻塞并发测速

	go func() {
		defer func() {
			a.testMu.Lock()
			if a.testCancel != nil { // 👈 修正：由于 defer 在函数退出时执行，直接判断并清理
				a.testCancel = nil
			}
			a.testMu.Unlock()
			cancel()
		}()

		// 2. 测速地址获取提到循环外部，彻底消除锁竞争
		testUrl := "http://www.gstatic.com/generate_204"
		if netCfg, err := clash.GetNetworkConfig(); err == nil && netCfg != nil && netCfg.TestURL != "" {
			testUrl = netCfg.TestURL
		}

		concurrency := 16
		semaphore := make(chan struct{}, concurrency)
		var wg sync.WaitGroup

		for _, name := range nodeNames {
			runtime.EventsEmit(a.ctx, "proxy-test-start", name)

			wg.Add(1)
			go func(nName string) {
				defer wg.Done()

				select {
				case semaphore <- struct{}{}:
				case <-ctx.Done():
					runtime.EventsEmit(a.ctx, "proxy-delay-update", map[string]interface{}{
						"name":   nName,
						"delay":  0,
						"status": "timeout",
					})
					return
				}
				defer func() { <-semaphore }()

				// 4. 为每个节点分配绝对独立的 5 秒专属超时时间
				reqCtx, reqCancel := context.WithTimeout(ctx, 5*time.Second)
				defer reqCancel()

				delay, err := clash.GetProxyDelay(reqCtx, nName, testUrl)

				if err != nil || delay <= 0 {
					runtime.EventsEmit(a.ctx, "proxy-delay-update", map[string]interface{}{
						"name":   nName,
						"delay":  0,
						"status": "timeout",
					})
				} else {
					runtime.EventsEmit(a.ctx, "proxy-delay-update", map[string]interface{}{
						"name":   nName,
						"delay":  delay,
						"status": "success",
					})
				}
			}(name)
		}

		wg.Wait()
		runtime.EventsEmit(a.ctx, "proxy-test-finished", "测速完成")

		// 5. 【用完即焚】：如果内核是专门为了测速启动的，测完后无痕关闭
		if isSilentTest {
			a.coreLifecycleMu.Lock()
			a.mu.RLock()
			stillInactive := !a.sysProxyActive && !a.tunActive
			a.mu.RUnlock()

			// 如果测速期间，用户没有手动开启系统代理或 TUN，就静默关闭内核节约内存
			if stillInactive && clash.IsRunning() {
				clash.Stop()
			}
			a.coreLifecycleMu.Unlock()
		}
	}()
}

// UpdateClashMode 切换 Clash 路由模式并全自动同步 UI
func (a *App) UpdateClashMode(mode string) error {
	a.mu.Lock()
	// ⚡ 防抖机制：如果意图切换的模式与当前一致，直接拦截，拒绝冗余通信
	if a.activeMode == mode {
		a.mu.Unlock()
		return nil
	}
	// 1. 修改内存的绝对真实状态
	a.activeMode = mode
	isRunning := clash.IsRunning()
	a.mu.Unlock()

	// 2. 🚀 关键：立刻推送状态！
	// 此时底层网络还没切过去，但内存已经变了。我们先让前端和托盘瞬间打上勾，消除用户的 UI 延迟感。
	a.SyncState()

	// 3. 将耗时的磁盘 IO 和 HTTP 通信放入独立协程
	go func(targetMode string, coreRunning bool) {
		// 写磁盘保存记忆
		a.saveActiveMode(targetMode)

		// 通知内核切换
		if coreRunning {
			err := clash.UpdateMode(targetMode)
			if err != nil {
				fmt.Printf("警告: 内核模式切换失败 (可能内核已断开): %v\n", err)
			}
		} else {
			// 内核未运行时，只去修改底层的 Yaml 预备配置
			activeCfg := a.getActiveConfig()
			if activeCfg != "" {
				clash.BuildRuntimeConfig(activeCfg, targetMode, a.GetAppBehavior().LogLevel)
			}
		}

		// 最后做一次对齐
		a.SyncState()
	}(mode, isRunning)

	return nil
}

func (a *App) SelectProxy(groupName, nodeName string) error {
	a.MarkNodeOffline(groupName, nodeName)

	a.saveOfflineNodes() // 👈 核心修复 1：立刻将选择写入硬盘

	if !clash.IsRunning() {
		return nil
	}

	err := clash.SelectProxy(groupName, nodeName)
	if err != nil {
		// 👈 核心修复 2：如果底层抛出拒绝连接的错误(假在线)，直接忽略它
		// 这样前端就不会弹报错，等到真在线时，确保机制会自动应用离线选择
		fmt.Printf("API切换节点失败(已作为离线记录保存): %v\n", err)
		return nil
	}
	a.SyncState() // 👈 补回：确保前端和托盘状态同步更新
	return nil
}

// UpdateSub 导出给前端
func (a *App) UpdateSub(name, url string) error {
	return a.updateSub(a.ctx, name, url)
}

// updateSub 内部实现
func (a *App) updateSub(ctx context.Context, name, url string) error {
	ua := a.GetAppBehavior().SubUA
	// 1. 下载订阅 (自动生成 ID)
	id, err := clash.DownloadSub(ctx, name, url, "", ua)
	if err != nil {
		return err
	}

	// 2. 如果更新的是当前正在使用的配置，触发一次内核重载
	if a.getActiveConfig() == id && clash.IsRunning() {
		mode := a.getActiveMode()
		clash.BuildRuntimeConfig(id, mode, a.GetAppBehavior().LogLevel)
		clash.ReloadConfig()
	}

	return nil
}

// UpdateSingleSub 导出给前端
func (a *App) UpdateSingleSub(id string) error {
	return a.updateSingleSub(a.ctx, id)
}

// updateSingleSub 内部实现
func (a *App) updateSingleSub(ctx context.Context, id string) error {
	clash.IndexLock.RLock()
	var url string
	var name string
	for _, item := range clash.SubIndex {
		if item.ID == id {
			url = item.URL
			name = item.Name
			break
		}
	}
	clash.IndexLock.RUnlock()

	if url == "" {
		return fmt.Errorf("未找到该订阅的链接")
	}

	ua := a.GetAppBehavior().SubUA
	_, err := clash.DownloadSub(ctx, name, url, id, ua)
	if err != nil {
		return err
	}

	// 如果更新的是当前正在使用的配置，触发重载
	if a.getActiveConfig() == id && clash.IsRunning() {
		mode := a.getActiveMode()
		clash.BuildRuntimeConfig(id, mode, a.GetAppBehavior().LogLevel)
		clash.ReloadConfig()
	}

	return nil
}

// UpdateAllSubs 导出给前端
func (a *App) UpdateAllSubs() error {
	return a.updateAllSubs(a.ctx)
}

// updateAllSubs 内部实现
func (a *App) updateAllSubs(ctx context.Context) error {
	clash.IndexLock.RLock()
	// 复制一份索引以防长时间占锁
	items := make([]clash.SubIndexItem, len(clash.SubIndex))
	copy(items, clash.SubIndex)
	clash.IndexLock.RUnlock()

	ua := a.GetAppBehavior().SubUA
	for _, item := range items {
		if item.URL != "" && item.Type == "remote" {
			// 将 ctx 透传给底层的下载函数
			_, _ = clash.DownloadSub(ctx, item.Name, item.URL, item.ID, ua)
		}
	}

	// 更新完成后，如果当前活动配置在其中，触发一次重载
	active := a.getActiveConfig()
	if active != "" && clash.IsRunning() {
		mode := a.getActiveMode()
		clash.BuildRuntimeConfig(active, mode, a.GetAppBehavior().LogLevel)
		clash.ReloadConfig()
	}

	return nil
}

func (a *App) StartTrafficStream() {
	a.mu.Lock()
	if a.cancelTraffic != nil {
		a.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelTraffic = cancel
	a.mu.Unlock()

	// ⚠️ 修复：移除 Ticker，改用长连接流式读取，彻底解决连接断开/失效问题
	go func() {
		traffic.StreamTraffic(ctx, func(up, down string) {
			runtime.EventsEmit(a.ctx, "traffic-data", map[string]string{"up": up, "down": down})
		})

		// 如果流异常断开，自动清理上下文以便后续可重新启动
		a.mu.Lock()
		if a.cancelTraffic != nil {
			a.cancelTraffic()
			a.cancelTraffic = nil
		}
		a.mu.Unlock()
	}()
}

func (a *App) StopTrafficStream() {
	a.mu.Lock()
	if a.cancelTraffic != nil {
		a.cancelTraffic()
		a.cancelTraffic = nil
	}
	a.mu.Unlock()

	// 🚀 新增：彻底停止流的同时，立刻向前端发射归零数据，防止前端数值卡死在最后一秒
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "traffic-data", map[string]string{"up": "0 B", "down": "0 B"})
	}
}

func (a *App) StartStreamingLogs() {
	a.mu.Lock()
	if a.logRunning {
		a.mu.Unlock()
		return
	}

	// ✅ 防御性清理残留
	if a.cancelLogs != nil {
		a.cancelLogs()
		a.cancelLogs = nil
	}

	a.logRunning = true
	a.logGen++ // 🚀 递增版本号，确保每个协程有唯一身份
	currentGen := a.logGen
	logCtx, cancel := context.WithCancel(a.ctx)
	a.cancelLogs = cancel

	logLevel := a.GetAppBehavior().LogLevel
	a.mu.Unlock()

	go func() {
		defer func() {
			a.mu.Lock()
			// 🚀 核心修复：只清理属于当前版本号（当前协程）的状态！
			if a.logGen == currentGen {
				a.logRunning = false
				a.cancelLogs = nil
			}
			a.mu.Unlock()
			cancel() // 释放当前独立的 Context
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
		a.cancelLogs()     // 👈 真正发送取消信号给后台协程
		a.cancelLogs = nil // 清空
		a.logRunning = false
		a.logGen++ // 🚀 使旧协程在清理时“认不清身份”，防止其覆盖新状态
	}
}

// GetRecentLogs 供前端拉取最近的日志记录 (Wails 绑定方法)
func (a *App) GetRecentLogs() []logger.LogEntry {
	return logger.AppLogs.GetAll()
}

// SearchLogs 供前端搜索日志 (Wails 绑定方法)
func (a *App) SearchLogs(keyword string) []logger.LogEntry {
	return logger.AppLogs.Search(keyword)
}

// --- 系统工具 ---

func (a *App) FixUWPNetwork() error {
	if !sys.CheckAdmin() {
		return fmt.Errorf("Need Admin Privileges")
	}
	return sys.ExemptAllUWP()
}

// SaveThemePreference 供前端调用，保存主题模式
func (a *App) SaveThemePreference(isDark bool) {
	theme := "light"
	if isDark {
		theme = "dark"
	}

	a.mu.Lock()
	if a.themeCache == theme {
		a.mu.Unlock()
		return // 状态一致，防抖拦截
	}
	a.themeCache = theme
	a.mu.Unlock()

	// 🚀 异步写盘，绝不阻塞 UI 和后续代码
	go os.WriteFile(getThemeConfigPath(), []byte(theme), 0644)

	// 触发全局同步
	a.SyncState()
}

// SyncState 统一推送当前应用状态给前端
func (a *App) SyncState() {
	behavior := a.GetAppBehavior()

	// 🚀 核心修复：从内存缓存直接提取所有状态，移除一切 os.ReadFile
	a.mu.RLock()
	sysProxy := a.sysProxyActive
	tunActive := a.tunActive
	theme := a.themeCache
	a.mu.RUnlock()

	if theme == "" {
		theme = "dark"
	}

	activeId := a.getActiveConfig()
	activeName := ""
	activeType := ""
	clash.IndexLock.RLock()
	for _, item := range clash.SubIndex {
		if item.ID == activeId {
			activeName = item.Name
			activeType = item.Type
			break
		}
	}
	clash.IndexLock.RUnlock()

	// 🚀 核心修改：将内核物理运行状态与 UI 业务接管状态解耦
	// 只有系统代理或 TUN 开启时，才向前端汇报 true，屏蔽静默测速引发的闪烁
	logicalIsRunning := clash.IsRunning() && (sysProxy || tunActive)

	// 统一组装当前真实状态
	state := AppState{
		IsRunning:        logicalIsRunning, // 👈 修改了这里
		Mode:             a.getActiveMode(),
		Theme:            theme,
		HideLogs:         behavior.HideLogs,
		SystemProxy:      sysProxy,
		Tun:              tunActive,
		Version:          a.GetCoreVersion(),
		ActiveConfig:     activeId,
		ActiveConfigName: activeName,
		ActiveConfigType: activeType,
	}

	// 推送给前端（唯一通道）
	runtime.EventsEmit(a.ctx, "app-state-sync", state)

	// 👇 追加：同步更新系统托盘的 UI 勾选状态
	if a.mSysProxy != nil {
		if sysProxy {
			a.mSysProxy.Check()
		} else {
			a.mSysProxy.Uncheck()
		}
	}
	if a.mTun != nil {
		if tunActive {
			a.mTun.Check()
		} else {
			a.mTun.Uncheck()
		}
	}

	// 👇 新增：同步出站路由的托盘单选状态
	if a.mModeRule != nil {
		// 先全部取消勾选
		a.mModeRule.Uncheck()
		a.mModeGlobal.Uncheck()
		a.mModeDirect.Uncheck()

		// 再根据当前真实状态单独勾选
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

// GetCoreVersion 返回内核版本
func (a *App) GetCoreVersion() string {
	// 1. 优先尝试从本地物理文件直接读取（无论内核是否在运行都能成功）
	binDir := utils.GetCoreBinDir()
	exePath := filepath.Join(binDir, "clash.exe")
	localVer := getLocalCoreVersion(exePath)

	if localVer != "" {
		return localVer
	}

	// 2. 如果本地文件读取失败，兜底尝试从 API 获取
	apiVer := clash.GetVersion()
	if apiVer != "" {
		return apiVer
	}

	// 3. 如果都失败了，说明还没下载内核
	return "未安装"
}

func (a *App) GetTunConfig() (*clash.TunConfig, error) {
	return clash.GetTunConfig()
}

// 替换2：保存 TUN 配置并触发内核热重启
func (a *App) SaveTunConfig(cfg *clash.TunConfig) error {
	err := clash.UpdateTunConfig(cfg)

	a.mu.Lock()
	isActive := a.sysProxyActive || a.tunActive
	a.mu.Unlock()

	if err == nil && isActive {
		// 🚀 修复点：加入生命周期锁，防止热重启瞬间用户点击 UI 触发并发启动
		a.coreLifecycleMu.Lock()
		defer a.coreLifecycleMu.Unlock()

		a.stopCoreService()
		a.ensureCoreRunning()
	}
	return err
}

// RenameConfig 重命名配置文件
func (a *App) RenameConfig(id, newName string) error {
	a.mu.Lock()
	isActiveConfig := (a.activeConfig == id)
	mode := a.activeMode
	wasActive := a.sysProxyActive || a.tunActive
	a.mu.Unlock()

	if isActiveConfig && clash.IsRunning() {
		a.coreLifecycleMu.Lock()
		a.stopCoreService()
		a.coreLifecycleMu.Unlock()
	}

	err := clash.RenameConfig(id, newName)

	if isActiveConfig {
		if err != nil {
			clash.BuildRuntimeConfig(id, mode, a.GetAppBehavior().LogLevel)
			if wasActive {
				a.ensureCoreRunning()
				a.SyncState()
			}
			return fmt.Errorf("文件重命名失败: %v", err)
		}

		if wasActive {
			a.ensureCoreRunning()
			a.SyncState()
		}
	}

	return err
}

// 3. 提供给前端：安装驱动 (加入安全回滚与防占用机制)
func (a *App) InstallTunDriver(force bool) (string, error) {
	binDir := utils.GetCoreBinDir()
	dllPath := filepath.Join(binDir, "wintun.dll")
	backupPath := filepath.Join(binDir, "wintun_backup.dll")

	// 🎯 核心修复：如果是检查更新模式且已存在，直接返回，不触发备份逻辑
	if !force && sys.IsWintunInstalled() {
		return "ALREADY_LATEST", nil
	}

	// 1. 如果当前正在使用 TUN 模式，必须先关闭...
	a.updateMu.Lock()         // 👈 加上全局组件更新锁
	defer a.updateMu.Unlock() // 👈 加上全局组件更新锁

	a.mu.RLock()
	wasTunActive := a.tunActive
	a.mu.RUnlock()

	if wasTunActive {
		_ = a.ToggleTunMode(false)
		time.Sleep(300 * time.Millisecond) // 等待系统解除对 DLL 的文件锁定
	}

	// 2. 如果存在旧驱动，先将其重命名为备份文件
	hasOldDll := false
	if _, err := os.Stat(dllPath); err == nil {
		os.Remove(backupPath) // 确保备份档无残留
		if err := os.Rename(dllPath, backupPath); err == nil {
			hasOldDll = true
		}
	}

	// 3. 执行真正的底层下载/解压安装逻辑 (此时 force 必然为 true 或者文件原本就不存在)
	status, err := sys.InstallWintun(force)

	// 4. 灾难恢复与回滚
	if err != nil {
		// 安装失败 -> 摧毁可能损坏的残留文件 -> 还原备份
		os.Remove(dllPath)
		if hasOldDll {
			_ = os.Rename(backupPath, dllPath)
		}

		// 恢复原有的 TUN 运行状态
		if wasTunActive {
			_ = a.ToggleTunMode(true)
		}
		return "", fmt.Errorf("Wintun 驱动安装失败，已安全还原旧版本: %v", err)
	}

	// 5. 更新彻底成功，过河拆桥销毁备份
	os.Remove(backupPath)

	// 如果用户更新前开着 TUN，更新完帮他自动无缝切回去
	if wasTunActive {
		_ = a.ToggleTunMode(true)
	}

	return status, nil
}
func (a *App) GetDNSConfig() (*clash.DNSConfig, error) {
	return clash.GetDNSConfig()
}

// 替换3：保存 DNS 配置并触发内核热重启
func (a *App) SaveDNSConfig(cfg *clash.DNSConfig) error {
	err := clash.UpdateDNSConfig(cfg)

	a.mu.Lock()
	isActive := a.sysProxyActive || a.tunActive
	a.mu.Unlock()

	if err == nil && isActive {
		a.coreLifecycleMu.Lock()
		defer a.coreLifecycleMu.Unlock()

		a.stopCoreService()
		a.ensureCoreRunning()
	}
	return err
}

// 获取基础网络设置
func (a *App) GetNetworkConfig() (*clash.NetworkConfig, error) {
	return clash.GetNetworkConfig()
}

// 保存基础网络设置并重启服务
func (a *App) SaveNetworkConfig(cfg *clash.NetworkConfig) error {
	err := clash.UpdateNetworkConfig(cfg)

	a.mu.Lock()
	isActive := a.sysProxyActive || a.tunActive
	a.mu.Unlock()

	// 这些设置直接影响内核底层行为，需要重启内核生效
	if err == nil && isActive {
		a.coreLifecycleMu.Lock()
		defer a.coreLifecycleMu.Unlock()

		a.stopCoreService()
		a.ensureCoreRunning()
	}
	return err
}

// --- 连接管理 (新增) ---

func (a *App) GetConnections() (map[string]interface{}, error) {
	rawBytes, err := clash.GetConnectionsRaw() // 👈 使用优化后的 Raw 方法
	if err != nil {
		return nil, err
	}

	var data struct {
		Connections []traffic.RawConnection `json:"connections"`
	}

	// 🚀 直接从字节流解析，省去 map 中转和二次 Marshal 的损耗
	if err := json.Unmarshal(rawBytes, &data); err != nil {
		return nil, err
	}

	vos := traffic.ProcessConnections(data.Connections)

	return map[string]interface{}{
		"connections": vos,
	}, nil
}

func (a *App) CloseConnection(id string) error {
	return clash.CloseConnection(id)
}

func (a *App) CloseAllConnections() error {
	return clash.CloseAllConnections()
}

// StartConnectionMonitor 供前端调用：开启连接监控
func (a *App) StartConnectionMonitor() error {
	return clash.StartConnectionMonitor(a.ctx)
}

// StopConnectionMonitor 供前端调用：关闭连接监控
func (a *App) StopConnectionMonitor() {
	clash.StopConnectionMonitor()
}

// ==========================================
// --- 本地配置文件管理 (新增) ---
// ==========================================

func (a *App) getProfilesDir() string {
	return utils.GetProfilesDir()
}

type SelectedFile struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// SelectLocalFile 1. 选择本地文件并预提取名称
func (a *App) SelectLocalFile() (*SelectedFile, error) {
	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择本地配置文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "YAML 配置", Pattern: "*.yaml;*.yml"},
		},
	})
	if err != nil || filePath == "" {
		return nil, err
	}
	// 提取不带后缀的文件名
	name := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	return &SelectedFile{Path: filePath, Name: name}, nil
}

// DoLocalImport 2. 执行最终导入动作
func (a *App) DoLocalImport(path, name string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	id := fmt.Sprintf("%d", time.Now().UnixMilli())
	destPath := filepath.Join(utils.GetSubscriptionsDir(), id+".yaml")
	if err := os.WriteFile(destPath, content, 0644); err != nil {
		return err
	}

	// 初始化伴生规则文件 (导入时立刻截取 YAML 规则，存入 JSON 中)
	rules, err := clash.GetOriginalRules(id)
	if err != nil || len(rules) == 0 {
		rules = []string{"MATCH,DIRECT"}
	}
	clash.SaveCustomRules(id, rules)

	// 更新全局索引
	clash.IndexLock.Lock()
	clash.SubIndex = append(clash.SubIndex, clash.SubIndexItem{
		ID:      id,
		Name:    name,
		URL:     "",
		Type:    "local",
		Updated: time.Now().Unix(),
	})
	clash.IndexLock.Unlock()

	return clash.SaveIndex()
}

// SyncRules 从配置源文件同步覆盖当前自定义规则
func (a *App) SyncRules(id string) error {
	return clash.SyncRulesFromYaml(id)
}

// OpenConfigFile 使用系统默认应用打开配置文件
func (a *App) OpenConfigFile(id string) error {
	path := filepath.Join(utils.GetSubscriptionsDir(), id+".yaml")
	var cmd *exec.Cmd
	switch stdruntime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

// DeleteConfig 删除配置文件
func (a *App) DeleteConfig(id string) error {
	a.mu.Lock()
	if a.activeConfig == id {
		a.activeConfig = ""
		a.mu.Unlock()

		a.saveActiveConfig("") // 清空本地记忆
		a.ClearOfflineNodes()
		a.saveOfflineNodes()
	} else {
		a.mu.Unlock()
	}

	return clash.DeleteConfig(id)
}

// ClearBaseConfig 清空基础配置（当所有订阅被删除时调用）
func (a *App) ClearBaseConfig() error {
	a.mu.Lock()
	a.activeConfig = ""
	a.saveActiveConfig("") // 清空本地记忆

	a.ClearOfflineNodes()
	a.mu.Unlock()
	a.saveOfflineNodes()

	// ✅ 改写到安全的数据目录
	destPath := clash.GetConfigPath() // 👈 复用标准方法

	// 写入一个最基础的空结构，防止 Clash 内核解析时直接崩溃
	emptyConfig := "mode: rule\nproxies: []\nproxy-groups: []\nrules: []\n"
	return os.WriteFile(destPath, []byte(emptyConfig), 0644)
}

// 替换1：切换本地配置时，使用流水线生成机制
// 切换本地配置 (使用 ID)
func (a *App) SelectLocalConfig(id string) error {
	a.mu.Lock()
	a.activeConfig = id
	mode := a.activeMode
	wasActive := a.sysProxyActive || a.tunActive
	a.mu.Unlock()
	a.ClearOfflineNodes()
	a.saveOfflineNodes()

	a.saveActiveConfig(id)

	a.coreLifecycleMu.Lock()
	a.stopCoreService()
	a.coreLifecycleMu.Unlock()

	sys.DisableSystemProxy()

	if err := clash.BuildRuntimeConfig(id, mode, a.GetAppBehavior().LogLevel); err != nil {
		return fmt.Errorf("生成运行时配置失败: %v", err)
	}

	if wasActive {
		if err := a.ensureCoreRunning(); err != nil {
			return err
		}
		a.mu.Lock()
		sysProxy := a.sysProxyActive
		a.mu.Unlock()
		if sysProxy {
			proxyPort := 7890
			if netCfg, err := clash.GetNetworkConfig(); err == nil && netCfg != nil {
				if netCfg.MixedPort != 0 {
					proxyPort = netCfg.MixedPort
				} else if netCfg.Port != 0 {
					proxyPort = netCfg.Port
				}
			}
			bypass := "localhost;127.*;10.*;172.16.*;192.168.*;<local>"
			sys.EnableSystemProxy("127.0.0.1", proxyPort, bypass)
		}
	}

	// 🚀 核心修复：无论内核是否启动，主动把最新状态(含名称、类型)强行推给前端 globalState
	a.SyncState()

	runtime.EventsEmit(a.ctx, "config-changed", id)
	return nil
}

// StartClash 启动配置 (前端兼容接口)
func (a *App) StartClash(id string) error {
	return a.SelectLocalConfig(id)
}

// --- 规则管理 (新增) ---

// 规则读写
// ResetComponentSettings 供前端调用：一键重置指定模块的设置并热重载
// module 支持: "tun", "dns", "network", "behavior"
func (a *App) ResetComponentSettings(module string) error {
	// 1. 删除 user_*.json 触发降级
	utils.ResetSetting(module)

	oldLogLevel := "" // 👈 新增

	// 2. 如果重置的是应用行为，需立刻刷新内存缓存
	if module == "behavior" {
		a.behaviorMu.RLock()
		oldLogLevel = a.behaviorCache.LogLevel // 👈 记录旧等级
		a.behaviorMu.RUnlock()

		a.initBehaviorCache()
	}

	// 3. 触发一次内核配置重塑
	a.mu.Lock()
	isActive := a.sysProxyActive || a.tunActive
	a.mu.Unlock()

	activeId := a.getActiveConfig()
	if activeId != "" {
		mode := a.getActiveMode()
		clash.BuildRuntimeConfig(activeId, mode, a.GetAppBehavior().LogLevel)

		// 重置涉及内核底层参数的模块，需热重启
		if isActive && (module == "tun" || module == "dns" || module == "network") {
			a.coreLifecycleMu.Lock()
			a.stopCoreService()
			a.ensureCoreRunning()
			a.coreLifecycleMu.Unlock()
		} else if clash.IsRunning() {
			clash.ReloadConfig() // 轻量级重载
		}
	}

	// 👇 核心修复：如果恢复默认设置导致日志等级发生了变化，重启日志流
	if module == "behavior" {
		a.behaviorMu.RLock()
		newLogLevel := a.behaviorCache.LogLevel
		a.behaviorMu.RUnlock()

		if oldLogLevel != "" && oldLogLevel != newLogLevel {
			a.mu.Lock()
			isLogging := a.logRunning
			a.mu.Unlock()
			if isLogging {
				a.StopStreamingLogs()
				time.Sleep(50 * time.Millisecond)
				a.StartStreamingLogs()
			}
		}
	}

	a.SyncState()
	return nil
}

func (a *App) GetCustomRules(id string) []string {
	rules, _ := clash.GetCustomRules(id)
	return rules
}

func (a *App) SaveCustomRules(id string, rules []string) error {
	return clash.SaveCustomRules(id, rules)
}

// 获取主题配置路径
func getThemeConfigPath() string {
	return filepath.Join(utils.GetDataDir(), "theme_setting.txt")
}

// GetAppState 供前端初始化时主动拉取应用状态
func (a *App) GetAppState() AppState {
	behavior := a.GetAppBehavior()
	a.mu.RLock()
	sysProxy := a.sysProxyActive
	tunActive := a.tunActive
	theme := a.themeCache
	a.mu.RUnlock()

	if theme == "" {
		theme = "dark"
	}

	return AppState{
		IsRunning:   clash.IsRunning(),
		Mode:        a.getActiveMode(),
		Theme:       theme,
		HideLogs:    behavior.HideLogs,
		SystemProxy: sysProxy,
		Tun:         tunActive,
		Version:     a.GetCoreVersion(),
	}
}

// ==========================================
// --- 系统托盘功能 (新增) ---
// ==========================================

// 🎯 修复托盘消失 Bug：延迟初始化机制
// 不要在 Wails 的 OnStartup 里直接拉起托盘，会和 WebView2 抢线程
// 建议在 Wails 的 OnDomReady 钩子里调用此方法，或者在 OnStartup 里包一层 goroutine 并短暂休眠
func (a *App) SetupSystray() {
	// 使用独立的 Goroutine 隔离 Windows 事件循环
	go systray.Run(a.onTrayReady, a.onTrayExit)
}

func (a *App) onTrayReady() {
	// 设置托盘图标和悬浮提示
	systray.SetIcon(iconData)
	systray.SetTitle("GoclashZ")
	systray.SetTooltip("GoclashZ 代理客户端")

	// --- 严格按照图片的 UI 布局 ---
	mShow := systray.AddMenuItem("显示主界面", "打开 GoclashZ 面板")
	systray.AddSeparator() // ----------------------

	// 👇 新增：创建出站路由的子菜单
	mModeMenu := systray.AddMenuItem("出站路由", "切换流量分流模式")
	a.mModeRule = mModeMenu.AddSubMenuItemCheckbox("规则分流 (Rule)", "", false)
	a.mModeGlobal = mModeMenu.AddSubMenuItemCheckbox("全局代理 (Global)", "", false)
	a.mModeDirect = mModeMenu.AddSubMenuItemCheckbox("直接连接 (Direct)", "", false)

	systray.AddSeparator() // ----------------------

	a.mSysProxy = systray.AddMenuItemCheckbox("系统代理", "全局接管 Windows 流量", false)
	a.mTun = systray.AddMenuItemCheckbox("虚拟网卡", "虚拟网卡底层接管", false)

	systray.AddSeparator() // ----------------------
	mRestart := systray.AddMenuItem("重启内核", "热重启 Mihomo 进程")
	mQuit := systray.AddMenuItem("退出程序", "彻底退出客户端")

	// 初始状态同步
	a.mu.RLock()
	if a.sysProxyActive {
		a.mSysProxy.Check()
	}
	if a.tunActive {
		a.mTun.Check()
	}
	a.mu.RUnlock()

	// ⚡ 调用同步方法，给当前的模式打勾
	a.SyncState()

	// ⚡ 核心：开启常驻监听协程，绝不能阻塞当前函数
	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				// 🚀 修复 2：Windows 11 焦点破解连招
				// 当 StartHidden: true 时，单靠 WindowShow 有时无法抢占系统前台焦点
				runtime.WindowShow(a.ctx)
				runtime.WindowUnminimise(a.ctx)

			// 👇 修复 3：极其致命的修复！【必须】使用 go 关键字异步执行！
			// 既然是状态机，托盘和 Vue 前端一样只负责"发送指令"，绝不阻塞自身
			case <-a.mModeRule.ClickedCh:
				go a.UpdateClashMode("rule")
			case <-a.mModeGlobal.ClickedCh:
				go a.UpdateClashMode("global")
			case <-a.mModeDirect.ClickedCh:
				go a.UpdateClashMode("direct")

			case <-a.mSysProxy.ClickedCh:
				go a.ToggleSystemProxy(!a.sysProxyActive)

			case <-a.mTun.ClickedCh:
				go a.ToggleTunMode(!a.tunActive)

			case <-mRestart.ClickedCh:
				go a.RestartCore()

			case <-mQuit.ClickedCh:
				// 安全退出的标准顺位：先卸载托盘，再通知 Wails 退出
				systray.Quit()
				runtime.Quit(a.ctx)
				return // 退出这个监听死循环

			// 🚀 修复点：新增对 Wails 全局上下文销毁的监听
			case <-a.ctx.Done():
				// 当用户从主界面 [X] 按钮强制退出，或者通过快捷键结束应用时，
				// a.ctx 会发送 Done 信号。此时优雅退出监听协程，防止内存泄露。
				return
			}
		}
	}()
}

func (a *App) onTrayExit() {
	// Windows 下的托盘清理逻辑
}

// ==========================================
// --- 规则分页与搜索支持 (新增) ---
// ==========================================

type RuleItem struct {
	Index int    `json:"index"` // 记录在原始切片中的真实索引，确保删除时准确无误
	Text  string `json:"text"`
}

type PagedRules struct {
	Total      int        `json:"total"`
	Items      []RuleItem `json:"items"`
	IsEditable bool       `json:"isEditable"`
}

// GetAllRules 供前端一次性获取所有匹配规则，配合虚拟滚动实现极致流畅
func (a *App) GetAllRules(id string, keyword string) (PagedRules, error) {
	rules, err := clash.GetCustomRules(id)
	if err != nil {
		return PagedRules{}, err
	}

	var filtered []RuleItem
	keyword = strings.ToLower(keyword)

	for i, r := range rules {
		if keyword == "" || strings.Contains(strings.ToLower(r), keyword) {
			filtered = append(filtered, RuleItem{Index: i, Text: r})
		}
	}

	return PagedRules{
		Total:      len(filtered),
		Items:      filtered,
		IsEditable: true,
	}, nil
}

// GetUwpApps 供前端拉取所有 UWP 应用
func (a *App) GetUwpApps() ([]sys.UwpApp, error) {
	return sys.GetUwpAppList()
}

// --- 软件更新 (新增) ---

// CheckComponentUpdate 模拟检查更新 (未来可对接 GitHub API)
func (a *App) CheckComponentUpdate() map[string]string {
	return map[string]string{
		"core":   "v1.18.3",
		"wintun": "0.14.1",
	}
}

// GetWintunVersion 供前端获取当前 Wintun 驱动的版本
func (a *App) GetWintunVersion() string {
	dllPath := sys.GetWintunPath()
	v := getWintunVersion(dllPath)
	if v == "" {
		return "未安装"
	}
	return v
}

func getWintunVersion(dllPath string) string {
	if _, err := os.Stat(dllPath); os.IsNotExist(err) {
		return ""
	}
	// 使用 PowerShell 获取文件版本号
	cmd := exec.Command("powershell", "-Command", fmt.Sprintf("(Get-Item '%s').VersionInfo.FileVersion", dllPath))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return "未知"
	}
	return strings.TrimSpace(string(out))
}

// 获取本地内核版本号
func getLocalCoreVersion(exePath string) string {
	if _, err := os.Stat(exePath); os.IsNotExist(err) {
		return ""
	}
	// 执行 clash.exe -v 获取版本 (使用 CombinedOutput 捕获所有输出)
	cmd := exec.Command(exePath, "-v")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}

	str := string(out)
	parts := strings.Fields(str)
	for _, p := range parts {
		// 寻找以 v 开头的版本号，或者形如 alpha-xxx 的字符串
		if strings.HasPrefix(p, "v") && len(p) > 1 {
			return p
		}
		if strings.HasPrefix(p, "alpha-") || strings.HasPrefix(p, "beta-") {
			return p
		}
	}
	return ""
}

// 辅助函数：归一化版本号（移除 v 前缀和空白符）
func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// safeRename 提供带有重试机制的原子替换，对抗 Windows 下杀毒软件短暂锁文件的现象
func safeRename(oldpath, newpath string) error {
	var err error
	// 尝试 5 次，每次间隔 200ms (最多等待 1 秒)
	for i := 0; i < 5; i++ {
		err = os.Rename(oldpath, newpath)
		if err == nil {
			return nil
		}
		// 如果是源文件不存在，属于硬错误，直接返回无需重试
		if os.IsNotExist(err) {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("多次尝试重命名被拒绝(文件可能被锁定): %v", err)
}

// UpdateCoreComponent 触发安全更新机制：无缝下载，原子替换
func (a *App) UpdateCoreComponent() (string, error) {
	binDir := utils.GetCoreBinDir()
	exePath := filepath.Join(binDir, "clash.exe")

	// 1. 获取本地版本
	localVersion := getLocalCoreVersion(exePath)

	// 2. 走 API 获取最新版本和下载链接
	directURL, latestVersion, err := getLatestMihomoAssetURL("windows", "amd64", ".zip")
	if err != nil {
		return "", err
	}

	// 3. 拦截：如果已经是最新，直接返回
	if localVersion != "" && latestVersion != "" && normalizeVersion(localVersion) == normalizeVersion(latestVersion) {
		return "ALREADY_LATEST", nil
	}

	// ==========================================
	// ⚡ 以下是无感下载阶段 (此时代理仍在正常运行！)
	// ==========================================
	tempZip := filepath.Join(binDir, "core_temp.zip")
	newExePath := filepath.Join(binDir, "clash_new.exe")

	// 4. 下载到临时压缩包
	err = downloadFileWithRetry(tempZip, directURL)
	if err != nil {
		return "", fmt.Errorf("下载新内核失败: %v", err)
	}

	// 5. 提取新 .exe 到一旁备用
	err = clash.ExtractKernel(tempZip, newExePath)
	os.Remove(tempZip) // 解压完直接把 zip 删了
	if err != nil {
		os.Remove(newExePath)
		return "", fmt.Errorf("解压内核失败，已清理残留: %v", err)
	}

	// ==========================================
	// ⚡ 以下是原子替换阶段 (仅在此刻产生百毫秒级的断流)
	// ==========================================
	a.updateMu.Lock()         // 👈 加上全局组件更新锁
	defer a.updateMu.Unlock() // 👈 加上全局组件更新锁

	a.mu.RLock()
	wasActive := a.sysProxyActive || a.tunActive
	a.mu.RUnlock()

	// 停机准备换核
	a.coreLifecycleMu.Lock()
	a.stopCoreService()
	a.coreLifecycleMu.Unlock()

	backupPath := filepath.Join(binDir, "clash_backup.exe")
	os.Remove(backupPath) // 确保历史备份档为空

	// 6. 重命名：旧版 -> 备份
	renameErr := safeRename(exePath, backupPath) // 👈 替换这里
	if renameErr != nil && !os.IsNotExist(renameErr) {
		// 锁定旧文件失败(极低概率)，立刻取消操作，恢复运行
		os.Remove(newExePath)
		if wasActive {
			a.ensureCoreRunning()
		}
		return "", fmt.Errorf("内核文件被锁定无法替换: %v", renameErr)
	}

	// 7. 重命名：新版 -> 正式服
	err = safeRename(newExePath, exePath) // 👈 替换这里
	if err != nil {
		// 灾难恢复：新版更名失败，立刻把老版换回来！
		os.Remove(newExePath)
		_ = safeRename(backupPath, exePath)
		if wasActive {
			a.ensureCoreRunning()
		}
		return "", fmt.Errorf("部署新内核失败，已安全回滚: %v", err)
	}

	// 8. 拉起新内核进行可用性验证
	if wasActive {
		if startErr := a.ensureCoreRunning(); startErr != nil {
			// ⚠️ 灾难恢复：新内核架构不对或启动报错，立即执行最终回滚！
			a.stopCoreService()
			os.Remove(exePath)                  // 摧毁损坏的新内核
			_ = safeRename(backupPath, exePath) // 复活老内核
			a.ensureCoreRunning()               // 重新启动
			a.SyncState()
			return "", fmt.Errorf("新内核损坏或不兼容，已自动回滚至稳定版: %v", startErr)
		}
	}

	// 9. 更新彻底成功，过河拆桥销毁备份
	os.Remove(backupPath)
	a.SyncState()

	return "SUCCESS", nil
}

// SaveUwpExemptions 供前端批量保存选中的 SID 列表
func (a *App) SaveUwpExemptions(sids []string) error {
	if !sys.CheckAdmin() {
		return fmt.Errorf("此操作需要管理员权限")
	}
	return sys.SaveUwpExemptions(sids)
}

// FlashWindow 底层闪烁窗口独立方法
func (a *App) FlashWindow() {
	windowName, _ := syscall.UTF16PtrFromString("GoclashZ")
	user32 := syscall.NewLazyDLL("user32.dll")
	procFindWindow := user32.NewProc("FindWindowW")
	procFlashWindowEx := user32.NewProc("FlashWindowEx")

	hwnd, _, _ := procFindWindow.Call(0, uintptr(unsafe.Pointer(windowName)))
	if hwnd != 0 {
		type FLASHWINFO struct {
			CbSize    uint32
			Hwnd      uintptr
			DwFlags   uint32
			UCount    uint32
			DwTimeout uint32
		}
		finfo := FLASHWINFO{
			CbSize:    uint32(unsafe.Sizeof(FLASHWINFO{})),
			Hwnd:      hwnd,
			DwFlags:   0x00000003 | 0x0000000C, // 闪烁标题和任务栏
			UCount:    0,
			DwTimeout: 0,
		}
		procFlashWindowEx.Call(uintptr(unsafe.Pointer(&finfo)))
	}
}

// UpdateGeoDatabase 安全更新单一规则数据库文件
func (a *App) UpdateGeoDatabase(dbType string) error {
	// 逻辑合并到并发更新中去执行，复用代码
	return a.UpdateAllGeoDatabases([]string{dbType})
}

// UpdateAllGeoDatabases 一键并发更新所有数据库，利用 Go 协程极速下载，且仅停机一次内核
func (a *App) UpdateAllGeoDatabases(types []string) error {
	behavior := a.GetAppBehavior()

	type dbTask struct {
		key  string
		url  string
		file string
	}

	var tasks []dbTask
	allTypes := map[string]dbTask{
		"geoip":   {"geoip", behavior.GeoIpLink, "geoip.metadb"},
		"geosite": {"geosite", behavior.GeoSiteLink, "GeoSite.dat"},
		"mmdb":    {"mmdb", behavior.MmdbLink, "Country.mmdb"},
		"asn":     {"asn", behavior.AsnLink, "GeoLite2-ASN.mmdb"},
	}

	// 筛选需要更新的任务（若传入空数组，则默认更新全部4个）
	if len(types) == 0 {
		types = []string{"geoip", "geosite", "mmdb", "asn"}
	}

	// 针对 GeoIP 后缀做特殊兼容处理
	if behavior.GeoIpLink != "" {
		if strings.HasSuffix(behavior.GeoIpLink, ".dat") {
			allTypes["geoip"] = dbTask{"geoip", behavior.GeoIpLink, "geoip.dat"}
		}
	}

	for _, t := range types {
		if task, ok := allTypes[t]; ok && task.url != "" {
			tasks = append(tasks, task)
		}
	}

	if len(tasks) == 0 {
		return fmt.Errorf("没有找到有效的下载链接配置")
	}

	binDir := utils.GetCoreBinDir()
	var wg sync.WaitGroup
	errChan := make(chan error, len(tasks))

	// 1. ⚡ 开启多协程，并发无感下载所有文件
	for _, t := range tasks {
		wg.Add(1)
		go func(task dbTask) {
			defer wg.Done()
			tempPath := filepath.Join(binDir, task.file+".temp")
			if err := downloadFileWithRetry(tempPath, task.url); err != nil {
				errChan <- fmt.Errorf("[%s] 下载失败: %v", task.key, err)
			}
		}(t)
	}

	wg.Wait()
	close(errChan)

	var errs []string
	for err := range errChan {
		errs = append(errs, err.Error())
	}

	// 如果所有文件都下载失败，直接打断，不需要停机
	if len(errs) == len(tasks) {
		return fmt.Errorf("网络异常，文件下载均失败:\n%s", strings.Join(errs, "\n"))
	}

	// 2. 🔒 获取全局组件更新锁，开始原子替换
	a.updateMu.Lock()
	defer a.updateMu.Unlock()

	a.mu.RLock()
	wasActive := a.sysProxyActive || a.tunActive
	a.mu.RUnlock()

	// 无论更新1个还是4个文件，内核只停机 1 次！
	a.coreLifecycleMu.Lock()
	a.stopCoreService()
	a.coreLifecycleMu.Unlock()

	for _, task := range tasks {
		tempPath := filepath.Join(binDir, task.file+".temp")
		targetPath := filepath.Join(binDir, task.file)
		backupPath := filepath.Join(binDir, task.file+".backup")

		// 跳过那些下载失败的文件
		if _, err := os.Stat(tempPath); os.IsNotExist(err) {
			continue
		}

		// 🎯 核心修复：清理 GeoIP 切换后缀时的残留僵尸文件
		if task.key == "geoip" {
			if task.file == "geoip.metadb" {
				os.Remove(filepath.Join(binDir, "geoip.dat"))
			} else {
				os.Remove(filepath.Join(binDir, "geoip.metadb"))
			}
		}

		os.Remove(backupPath)
		if _, err := os.Stat(targetPath); err == nil {
			_ = safeRename(targetPath, backupPath) // 👈 替换这里
		}

		if err := safeRename(tempPath, targetPath); err != nil { // 👈 替换这里
			os.Remove(tempPath)
			_ = safeRename(backupPath, targetPath) // 👈 失败回滚也使用安全替换
			errs = append(errs, fmt.Sprintf("[%s] 部署失败: %v", task.key, err))
		} else {
			os.Remove(backupPath)
		}
	}

	// 3. 🚀 统一重新拉起内核
	if wasActive {
		if startErr := a.ensureCoreRunning(); startErr != nil {
			a.SyncState()
			return fmt.Errorf("文件更新成功，但内核重启引发异常: %v", startErr)
		}
	}

	a.SyncState()

	if len(errs) > 0 {
		return fmt.Errorf("部分文件处理出现警告:\n%s", strings.Join(errs, "\n"))
	}
	return nil
}

// GeoFileInfo 描述数据库文件的物理信息
type GeoFileInfo struct {
	Size    int64 `json:"size"`
	ModTime int64 `json:"modTime"` // Unix 时间戳
	Exists  bool  `json:"exists"`
}

// GetGeoDatabaseInfo 获取所有规则数据库的物理文件状态
func (a *App) GetGeoDatabaseInfo() map[string]GeoFileInfo {
	binDir := utils.GetCoreBinDir()
	results := make(map[string]GeoFileInfo)

	files := map[string]string{
		"geoip":   "geoip.metadb",
		"geosite": "GeoSite.dat",
		"mmdb":    "Country.mmdb",
		"asn":     "GeoLite2-ASN.mmdb",
	}

	for key, name := range files {
		path := filepath.Join(binDir, name)

		// 针对 GeoIP 可能的 .dat 后缀做兼容检查
		if key == "geoip" {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				path = filepath.Join(binDir, "geoip.dat")
			}
		}

		info, err := os.Stat(path)
		if err != nil {
			results[key] = GeoFileInfo{Exists: false}
			continue
		}

		results[key] = GeoFileInfo{
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
			Exists:  true,
		}
	}

	return results
}

// getLatestMihomoAssetURL 走 GitHub API 获取最新内核版本与下载直链
func getLatestMihomoAssetURL(osName, arch, suffix string) (string, string, error) {
	// ⚡ 使用本地定义的超时 Client，防止 GitHub API 卡死整个应用
	client := &http.Client{Timeout: 10 * time.Second}

	url := "https://api.github.com/repos/MetaCubeX/mihomo/releases/latest"
	resp, err := client.Get(url)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", "", err
	}

	for _, asset := range release.Assets {
		name := strings.ToLower(asset.Name)
		// 匹配逻辑：windows + amd64 + .zip
		if strings.Contains(name, osName) && strings.Contains(name, arch) && strings.HasSuffix(name, suffix) {
			return asset.BrowserDownloadURL, release.TagName, nil
		}
	}

	return "", "", fmt.Errorf("未找到匹配的资产文件: %s-%s%s", osName, arch, suffix)
}

// downloadFileWithRetry 带有重试机制的文件下载封装
func downloadFileWithRetry(destPath, url string) error {
	var lastErr error
	for i := 0; i < 3; i++ {
		// 🚀 调用 downloader.go 中的核心下载逻辑
		lastErr = DownloadLargeFile(url, destPath)
		if lastErr == nil {
			return nil
		}
		// 失败后等待 2 秒再重试，给网络恢复留出时间
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("三次尝试均失败: %v", lastErr)
}
