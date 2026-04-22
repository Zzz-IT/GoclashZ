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

	"github.com/getlantern/systray" // 👈 2. 新增：引入托盘库
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/windows/icon.ico
var iconData []byte // 👈 3. 新增：将图标编译进二进制文件中给托盘使用

type App struct {
	ctx            context.Context
	cancelTraffic  context.CancelFunc
	cancelLogs     context.CancelFunc
	logRunning     bool
	mu             sync.RWMutex
	activeConfig   string
	activeMode     string
	offlineNodes   map[string]string
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

// 获取配置文件的存放路径
func (a *App) getAppBehaviorPath() string {
	return filepath.Join(utils.GetDataDir(), "app_behavior.json")
}

// 1. 获取离线节点记忆文件的路径
func (a *App) getOfflineNodesPath() string {
	return filepath.Join(utils.GetDataDir(), "offline_nodes.json")
}

// 2. 将内存中的节点选择持久化到磁盘
func (a *App) saveOfflineNodes() {
	a.mu.RLock()
	data, err := json.MarshalIndent(a.offlineNodes, "", "  ")
	a.mu.RUnlock()
	if err == nil {
		os.WriteFile(a.getOfflineNodesPath(), data, 0644)
	}
}

// 3. 启动时从磁盘读取记忆
func (a *App) loadOfflineNodes() {
	data, err := os.ReadFile(a.getOfflineNodesPath())
	if err == nil {
		a.mu.Lock()
		if a.offlineNodes == nil {
			a.offlineNodes = make(map[string]string)
		}
		json.Unmarshal(data, &a.offlineNodes)
		a.mu.Unlock()
	}
}

// 内部初始化缓存的方法，在 startup 中调用
func (a *App) initBehaviorCache() {
	defaultConfig := AppBehavior{
		SilentStart: false,
		CloseToTray: true,
		LogLevel:    "info",
		HideLogs:    false,
		// 预设好权威且带加速的默认链接
		GeoIpLink:   "https://ghproxy.net/https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb",
		GeoSiteLink: "https://ghproxy.net/https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat",
		MmdbLink:    "https://ghproxy.net/https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb",
		AsnLink:     "https://ghproxy.net/https://github.com/xishang0128/geoip/releases/download/latest/GeoLite2-ASN.mmdb",
	}

	data, err := os.ReadFile(a.getAppBehaviorPath())
	if err == nil {
		if err := json.Unmarshal(data, &defaultConfig); err != nil {
			fmt.Println("行为配置解析失败，使用默认值")
		}
	}

	if defaultConfig.LogLevel == "" {
		defaultConfig.LogLevel = "info"
	}

	a.behaviorMu.Lock()
	a.behaviorCache = defaultConfig
	a.behaviorMu.Unlock()
}

// GetAppBehavior 供前端获取当前设置 (Wails 绑定方法)
func (a *App) GetAppBehavior() AppBehavior {
	a.behaviorMu.RLock()
	defer a.behaviorMu.RUnlock()
	return a.behaviorCache
}

// 修改保存逻辑，写盘的同时更新缓存
func (a *App) SaveAppBehavior(config AppBehavior) error {
	// 1. 写入磁盘
	data, _ := json.MarshalIndent(config, "", "  ")
	err := os.WriteFile(a.getAppBehaviorPath(), data, 0644)

	// 2. 更新内存缓存
	if err == nil {
		a.behaviorMu.Lock()
		a.behaviorCache = config
		a.behaviorMu.Unlock()
	}

	// 3. 广播与同步
	runtime.EventsEmit(a.ctx, "behavior-changed", config)

	active := a.getActiveConfig()
	if active != "" {
		mode := a.getActiveMode()
		clash.BuildRuntimeConfig(active, mode)
		if clash.IsRunning() {
			clash.ReloadConfig()
		}
	}
	a.SyncState()
	return err
}

// SubRecord 用于记录文件名与订阅链接的映射
type SubRecord struct {
	URL      string `json:"url"`
	Upload   int64  `json:"upload"`   // 👈 新增
	Download int64  `json:"download"` // 👈 新增
	Total    int64  `json:"total"`    // 👈 新增
	Expire   int64  `json:"expire"`   // 👈 新增
}

// 获取订阅信息存储路径
func (a *App) getSubsRecordPath() string {
	return filepath.Join(utils.GetDataDir(), "subs_history.json")
}

// 读取所有订阅记录
func (a *App) readSubRecords() map[string]SubRecord {
	records := make(map[string]SubRecord)
	data, err := os.ReadFile(a.getSubsRecordPath())
	if err == nil {
		json.Unmarshal(data, &records)
	}
	return records
}

// 保存订阅记录
func (a *App) saveSubRecord(filename string, url string, info *clash.SubInfo) {
	records := a.readSubRecords()
	record := SubRecord{URL: url}

	// 保留历史流量记录
	if old, exists := records[filename]; exists {
		record.Upload = old.Upload
		record.Download = old.Download
		record.Total = old.Total
		record.Expire = old.Expire
	}
	// 如果本次请求有新流量数据，则覆盖
	if info != nil {
		record.Upload = info.Upload
		record.Download = info.Download
		record.Total = info.Total
		record.Expire = info.Expire
	}

	records[filename] = record
	data, _ := json.MarshalIndent(records, "", "  ")
	os.WriteFile(a.getSubsRecordPath(), data, 0644)
}

// GetSubRecords 供前端获取订阅记录映射
func (a *App) GetSubRecords() map[string]SubRecord {
	return a.readSubRecords()
}

// 1. 获取配置排序记忆文件的路径
func (a *App) getSubsOrderPath() string {
	return filepath.Join(utils.GetDataDir(), "subs_order.json")
}

// 2. 供前端调用的保存顺序 API (Wails 绑定方法)
func (a *App) SaveConfigsOrder(order []string) error {
	data, _ := json.MarshalIndent(order, "", "  ")
	return os.WriteFile(a.getSubsOrderPath(), data, 0644)
}

// ProxyStatus 新增给前端返回的双重状态结构
type ProxyStatus struct {
	SystemProxy bool `json:"systemProxy"`
	Tun         bool `json:"tun"`
}

// AppState 定义全局状态同步结构
type AppState struct {
	IsRunning   bool   `json:"isRunning"`
	Mode        string `json:"mode"`
	Theme       string `json:"theme"`
	HideLogs    bool   `json:"hideLogs"`
	// 👇 新增以下字段，统一接管 UI
	SystemProxy bool   `json:"systemProxy"`
	Tun         bool   `json:"tun"`
	Version     string `json:"version"`
}

// 1. 在 app.go 任意位置新增这个辅助方法，用于将离线缓存合并到数据源
func (a *App) mergeOfflineNodes(data map[string]interface{}) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if groups, ok := data["groups"].(map[string]interface{}); ok {
		for gName, groupData := range groups {
			if gMap, ok2 := groupData.(map[string]interface{}); ok2 {
				// 优先使用离线选择
				if a.offlineNodes != nil {
					if selNode, exists := a.offlineNodes[gName]; exists {
						gMap["now"] = selNode
					}
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

// 1. 在 app.go 任意位置新增一个获取程序真实绝对路径的辅助方法

// 记录当前选中的配置文件名到本地
func (a *App) saveActiveConfig(fileName string) {
	behavior := a.GetAppBehavior()
	behavior.ActiveConfig = fileName
	data, _ := json.MarshalIndent(behavior, "", "  ")
	os.WriteFile(a.getAppBehaviorPath(), data, 0644)
}

// 启动时读取上次选中的配置文件名
func (a *App) loadActiveConfig() string {
	return a.GetAppBehavior().ActiveConfig
}

// 记录当前选中的模式到本地
func (a *App) saveActiveMode(mode string) {
	behavior := a.GetAppBehavior()
	behavior.ActiveMode = mode
	data, _ := json.MarshalIndent(behavior, "", "  ")
	os.WriteFile(a.getAppBehaviorPath(), data, 0644)
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
		if err := clash.BuildRuntimeConfig(activeCfg, mode); err != nil {
			fmt.Printf("生成运行时配置警告: %v\n", err)
		}
	}

	// 启动内核
	if err := clash.Start(a.ctx); err != nil {
		return err
	}

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
				clash.SwitchProxy(g, n)
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
	// 👇 核心修复 3：在彻底停机前，反向抓取内核中真实的节点状态存入离线缓存
	if clash.IsRunning() {
		if data, err := clash.GetInitialData(); err == nil {
			if groups, ok := data["groups"].([]interface{}); ok { // 👈 注意：clash.GetInitialData 返回的 groups 是切片
				a.mu.Lock()
				if a.offlineNodes == nil {
					a.offlineNodes = make(map[string]string)
				}
				for _, g := range groups {
					if gMap, ok2 := g.(map[string]interface{}); ok2 {
						gName, _ := gMap["name"].(string)
						now, _ := gMap["now"].(string)
						if gName != "" && now != "" {
							a.offlineNodes[gName] = now
						}
					}
				}
				a.mu.Unlock()
				a.saveOfflineNodes() // 存入磁盘
			}
		}
	}

	clash.Stop()
	a.StopTrafficStream()
}

// ==========================================
// --- 暴露给前端的 API ---
// ==========================================

// GetProxyStatus 获取当前双轨状态
func (a *App) GetProxyStatus() ProxyStatus {
	a.mu.RLock()
	sysProxy := a.sysProxyActive
	// ✅ 优化：不再调用 clash.GetTunConfig() 去读文件，直接读取内存中的 tunActive 状态
	realTun := a.tunActive && clash.IsRunning()
	a.mu.RUnlock()

	return ProxyStatus{
		SystemProxy: sysProxy,
		Tun:         realTun,
	}
}

// ToggleSystemProxy 开关 1：系统代理
func (a *App) ToggleSystemProxy(enable bool) error {
	defer a.SyncState() // 🚀 无论成功失败，退出函数时强制刷新 UI 状态，防止前端卡死在错误位置
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
	defer a.SyncState() // 🚀 防御性同步：确保 UI 状态始终回滚到真实后端状态
	a.coreLifecycleMu.Lock()         // 🔒 加锁
	defer a.coreLifecycleMu.Unlock() // 🔓 退出时自动解锁

	if enable {
		if !sys.IsWintunInstalled() {
			return fmt.Errorf("缺失 Wintun 驱动，请先在设置中安装")
		}
		if !sys.CheckAdmin() {
			return fmt.Errorf("开启虚拟网卡需要管理员权限，请以管理员身份重启软件，或在设置中点击提权")
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
		time.Sleep(150 * time.Millisecond) // 等待旧端口释放
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

	// 2. 短暂等待，确保底层的端口释放、TUN 虚拟网卡卸载干净
	time.Sleep(300 * time.Millisecond)

	// 3. 读取当前应用的接管状态
	a.mu.RLock()
	needCore := a.sysProxyActive || a.tunActive
	a.mu.RUnlock()

	// 4. 如果系统代理或 TUN 至少开了一个，则重新启动内核
	if needCore {
		if err := a.ensureCoreRunning(); err != nil {
			a.SyncState() // 即使失败也要推一次状态
			return fmt.Errorf("内核重启失败: %v", err)
		}
	}

	// 5. 同步最新状态给前端
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

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.initBehaviorCache() // 👈 新增：初始化配置缓存

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
				// 执行订阅更新逻辑 (目前的方法内部如果是同步阻塞的，至少外部应用关闭时 a.ctx.Done 会立刻响应主循环退出)
				err := a.UpdateAllSubs()
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
	// ⚠️ 核心逻辑：退出时强制恢复网络环境
	fmt.Println("正在关闭 GoclashZ，正在清理网络代理设置...")

	_ = a.ToggleSystemProxy(false) // 关闭系统代理
	_ = a.ToggleTunMode(false)     // 关闭虚拟网卡

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

// --- 代理旧接口兼容 (可选，若前端已全量更新可删除) ---

func (a *App) RunProxy() error {
	return a.ToggleSystemProxy(true)
}

func (a *App) StopProxy() error {
	a.mu.Lock()
	a.sysProxyActive = false
	a.tunActive = false
	a.mu.Unlock()
	sys.DisableSystemProxy()
	a.stopCoreService()

	a.SyncState() // 👈 关键：同步状态
	return nil
}

// 注意：此方法名与新 GetProxyStatus 冲突，我已在上方实现了返回 ProxyStatus 结构体的新方法。
// 为了兼容 App.vue 的布尔值判断，我们保留一个简单的 IsCoreRunning 逻辑或者让前端适配。
// 这里我们将旧的 GetProxyStatus 逻辑合并到 New API 中。

// --- 配置与测速 ---

func (a *App) GetInitialData() (map[string]interface{}, error) {
	activeConfig := a.getActiveConfig()
	mode := a.getActiveMode()

	if !clash.IsRunning() {
		data, err := clash.GetOfflineData(activeConfig)
		if err != nil {
			// 🎯 核心修复 1：即使离线获取失败（如文件损坏），也必须把 activeConfig 传给前端，防止前端丢失“当前选中”的记忆
			return map[string]interface{}{"mode": mode, "groups": make(map[string]interface{}), "activeConfig": activeConfig, "isOffline": true}, nil
		}

		a.mergeOfflineNodes(data)

		data["activeConfig"] = activeConfig
		data["mode"] = mode
		data["isOffline"] = true
		return data, nil
	}

	data, err := clash.GetInitialData()
	if err != nil {
		fallbackData, _ := clash.GetOfflineData(activeConfig)
		if fallbackData != nil {
			a.mergeOfflineNodes(fallbackData)
			fallbackData["activeConfig"] = activeConfig
			fallbackData["isOffline"] = true
			fallbackData["mode"] = mode
			return fallbackData, nil
		}
		// 🎯 核心修复 2：即使 API 失败且降级也失败，依然要将 activeConfig 传出
		return map[string]interface{}{"mode": mode, "groups": make(map[string]interface{}), "activeConfig": activeConfig, "isOffline": true}, nil
	}

	data["activeConfig"] = activeConfig
	data["mode"] = mode
	data["isOffline"] = false

	// 注入节点组原始排序
	configPath := filepath.Join(utils.GetProfilesDir(), activeConfig)
	if activeConfig == "" || activeConfig == "config.yaml" {
		configPath = filepath.Join(utils.GetDataDir(), "config.yaml")
	}
	if yamlData, err := os.ReadFile(configPath); err == nil {
		data["groupOrder"] = clash.ExtractGroupOrder(yamlData)
	}

	return data, nil
}

func (a *App) TestAllProxies(nodeNames []string) {
	if !clash.IsRunning() {
		if err := clash.Start(a.ctx); err != nil {
			runtime.EventsEmit(a.ctx, "proxy-test-finished", "内核启动失败，无法测速")
			return
		}
		time.Sleep(1 * time.Second)
	}

	go func() {
		concurrency := 16 // 稍微提高并发
		semaphore := make(chan struct{}, concurrency)
		var wg sync.WaitGroup

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		for _, name := range nodeNames {
			// 1. 立即发射“开始测速”事件，告诉前端这个节点开始转圈了
			runtime.EventsEmit(a.ctx, "proxy-test-start", name)

			wg.Add(1)
			go func(nName string) {
				defer wg.Done()

				select {
				case semaphore <- struct{}{}:
				case <-ctx.Done():
					// 🚀 核心修复：被 Context 强制取消的节点，必须通知前端停止转圈
					runtime.EventsEmit(a.ctx, "proxy-delay-update", map[string]interface{}{
						"name":   nName,
						"delay":  0,
						"status": "timeout", // 通知前端测速取消
					})
					return
				}
				defer func() { <-semaphore }()

				// 2. 向 Clash 内核请求真实测速
				delay, err := clash.GetProxyDelay(nName)

				// 3. 发射结果
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
				clash.BuildRuntimeConfig(activeCfg, targetMode)
			}
		}
		
		// 最后做一次对齐
		a.SyncState()
	}(mode, isRunning)

	return nil
}

func (a *App) SelectProxy(groupName, nodeName string) error {
	a.mu.Lock()
	if a.offlineNodes == nil {
		a.offlineNodes = make(map[string]string)
	}
	a.offlineNodes[groupName] = nodeName
	a.mu.Unlock()

	a.saveOfflineNodes() // 👈 核心修复 1：立刻将选择写入硬盘

	if !clash.IsRunning() {
		return nil
	}

	err := clash.SwitchProxy(groupName, nodeName)
	if err != nil {
		// 👈 核心修复 2：如果底层抛出拒绝连接的错误(假在线)，直接忽略它
		// 这样前端就不会弹报错，等到真在线时，确保机制会自动应用离线选择
		fmt.Printf("API切换节点失败(已作为离线记录保存): %v\n", err)
		return nil
	}
	a.SyncState() // 👈 补回：确保前端和托盘状态同步更新
	return nil
}

func (a *App) UpdateSub(url string) error {
	ua := a.GetAppBehavior().SubUA
	// 1. 下载订阅
	filename, info, err := clash.UpdateSubscription(url, "", ua)
	if err != nil {
		return err
	}

	// 2. 记录 URL 映射
	a.saveSubRecord(filename, url, info)

	// 3. 如果更新的是当前正在使用的配置，触发重载
	if a.getActiveConfig() == filename && clash.IsRunning() {
		mode := a.getActiveMode()
		clash.BuildRuntimeConfig(filename, mode)
		clash.ReloadConfig()
	}

	return nil
}

// UpdateSingleSub 实装：更新单个文件
func (a *App) UpdateSingleSub(filename string) error {
	records := a.readSubRecords()
	record, ok := records[filename]
	if !ok || record.URL == "" {
		return fmt.Errorf("未找到该文件的订阅链接，请重新导入")
	}

	ua := a.GetAppBehavior().SubUA
	_, info, err := clash.UpdateSubscription(record.URL, filename, ua)
	if err != nil {
		return err
	}

	// 更新流量信息
	a.saveSubRecord(filename, record.URL, info)

	// 如果更新的是当前正在使用的配置，触发重载
	if a.getActiveConfig() == filename && clash.IsRunning() {
		mode := a.getActiveMode()
		clash.BuildRuntimeConfig(filename, mode)
		clash.ReloadConfig()
	}

	return nil
}

// UpdateAllSubs 实装：遍历并更新所有已记录链接的文件
func (a *App) UpdateAllSubs() error {
	records := a.readSubRecords()
	ua := a.GetAppBehavior().SubUA
	for filename, record := range records {
		if record.URL != "" {
			// 忽略错误继续更新下一个
			_, info, _ := clash.UpdateSubscription(record.URL, filename, ua)
			if info != nil {
				a.saveSubRecord(filename, record.URL, info)
			}
		}
	}

	// 更新完成后，如果当前活动配置在其中，触发一次重载
	active := a.getActiveConfig()
	if active != "" && clash.IsRunning() {
		if _, exists := records[active]; exists {
			mode := a.getActiveMode()
			clash.BuildRuntimeConfig(active, mode)
			clash.ReloadConfig()
		}
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
}

func (a *App) StartStreamingLogs() {
	a.mu.Lock()
	if a.logRunning {
		a.mu.Unlock()
		return
	}

	// ✅ 核心加固：防御性清理残留的 cancel 函数，防止 Goroutine 泄露
	if a.cancelLogs != nil {
		a.cancelLogs()
		a.cancelLogs = nil
	}

	a.logRunning = true
	// 👈 创建独立的子 Context 控制日志
	logCtx, cancel := context.WithCancel(a.ctx)
	a.cancelLogs = cancel
	a.mu.Unlock()

	// 调用 api_client.go 中定义的 FetchLogs，传入受控的 logCtx 和回调
	go func() {
		// 🚀 核心修复：确保即使 HTTP 长连接断开（内核重启），也能重置状态
		defer func() {
			a.mu.Lock()
			a.logRunning = false
			if a.cancelLogs != nil {
				a.cancelLogs()
				a.cancelLogs = nil
			}
			a.mu.Unlock()
		}()

		clash.FetchLogs(logCtx, func(data interface{}) {
			// 解析为 LogEntry 并存入 Buffer
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

// 在 app.go 底部添加以下方法

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
		a.stopCoreService()
		time.Sleep(200 * time.Millisecond)
		a.ensureCoreRunning()
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
		a.stopCoreService()
		time.Sleep(200 * time.Millisecond)
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
		a.stopCoreService()
		time.Sleep(200 * time.Millisecond)
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

// 修改 GetLocalConfigs，让它结合物理文件和用户的自定义排序
func (a *App) GetLocalConfigs() ([]string, error) {
	dir := a.getProfilesDir()
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	// 1. 获取实际存在的文件
	var actualConfigs []string
	actualMap := make(map[string]bool)
	for _, file := range files {
		if !file.IsDir() && file.Name() != "config.yaml" &&
			(strings.HasSuffix(file.Name(), ".yaml") || strings.HasSuffix(file.Name(), ".yml")) {
			actualConfigs = append(actualConfigs, file.Name())
			actualMap[file.Name()] = true
		}
	}

	// 2. 读取之前保存的排序
	orderPath := a.getSubsOrderPath()
	data, err := os.ReadFile(orderPath)
	var savedOrder []string
	if err == nil {
		json.Unmarshal(data, &savedOrder)
	}

	// 3. 重建有序列表
	var finalConfigs []string
	seen := make(map[string]bool)

	// 先按保存的顺序推入（且确保文件确实存在）
	for _, name := range savedOrder {
		if actualMap[name] {
			finalConfigs = append(finalConfigs, name)
			seen[name] = true
		}
	}

	// 把新下载/新导入、还不在排序记录里的文件追加到末尾
	for _, name := range actualConfigs {
		if !seen[name] {
			finalConfigs = append(finalConfigs, name)
		}
	}

	return finalConfigs, nil
}

// ImportLocalConfig 导入本地配置文件
func (a *App) ImportLocalConfig() error {
	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择本地配置文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "YAML 配置", Pattern: "*.yaml;*.yml"},
		},
	})
	if err != nil || filePath == "" {
		return err
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	fileName := filepath.Base(filePath)
	destDir := a.getProfilesDir()
	os.MkdirAll(destDir, 0755)
	destPath := filepath.Join(destDir, fileName)

	return os.WriteFile(destPath, content, 0644)
}

// RenameConfig 重命名配置文件
func (a *App) RenameConfig(oldName, newName string) error {
	oldName = filepath.Base(oldName)
	newName = filepath.Base(newName)

	if !strings.HasSuffix(newName, ".yaml") && !strings.HasSuffix(newName, ".yml") {
		newName += ".yaml"
	}
	oldPath := filepath.Join(a.getProfilesDir(), oldName)
	newPath := filepath.Join(a.getProfilesDir(), newName)

	a.mu.Lock()
	isActiveConfig := (a.activeConfig == oldName)
	mode := a.activeMode // 提前取出 mode
	a.mu.Unlock()

	if isActiveConfig && clash.IsRunning() {
		clash.Stop()
		time.Sleep(200 * time.Millisecond) // 等待文件句柄释放
	}

	renameFunc := func() error {
		if strings.EqualFold(oldName, newName) && oldName != newName {
			tempPath := newPath + ".tmp"
			if err := os.Rename(oldPath, tempPath); err != nil {
				return err
			}
			return os.Rename(tempPath, newPath)
		}
		return os.Rename(oldPath, newPath)
	}

	err := renameFunc()

	if isActiveConfig {
		// ✅ 优化：增加错误回退处理，如果重命名失败，必须用旧配置将内核救回来
		if err != nil {
			clash.BuildRuntimeConfig(oldName, mode)
			a.ensureCoreRunning()
			return fmt.Errorf("文件重命名失败，已恢复原状态: %v", err)
		}

		// ✅ 核心修复：重新加锁执行 CAS 状态校验
		a.mu.Lock()
		// 仅当 activeConfig 仍是刚才重命名的文件时，才予以更新
		if a.activeConfig == oldName {
			a.activeConfig = newName
			a.mu.Unlock() // 尽早释放锁

			// 耗时的磁盘操作放在锁外
			a.saveActiveConfig(newName)

			clash.BuildRuntimeConfig(newName, mode)
			a.ensureCoreRunning()
		} else {
			a.mu.Unlock() // 如果已经被其他协程修改，则放弃覆盖
		}
	}

	return err
}

// OpenConfigFile 使用系统默认应用打开配置文件
func (a *App) OpenConfigFile(fileName string) error {
	fileName = filepath.Base(fileName) // 净化
	path := filepath.Join(a.getProfilesDir(), fileName)
	var cmd *exec.Cmd
	switch stdruntime.GOOS {
	case "windows":
		// ⚠️ 修复：避免使用 cmd /c，防止 Shell 元字符注入
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
func (a *App) DeleteConfig(fileName string) error {
	fileName = filepath.Base(fileName) // 👈 净化
	path := filepath.Join(a.getProfilesDir(), fileName)

	// 🚀 核心修复：如果删掉的是当前活动配置，必须重置环境
	a.mu.Lock()
	if a.activeConfig == fileName {
		a.activeConfig = ""
		a.mu.Unlock()
		
		a.saveActiveConfig("") // 清空本地记忆
		// 清空离线记录，防止切到空配置时报错
		a.mu.Lock()
		a.offlineNodes = make(map[string]string)
		a.mu.Unlock()
		a.saveOfflineNodes()
	} else {
		a.mu.Unlock()
	}

	return os.Remove(path)
}

// ClearBaseConfig 清空基础配置（当所有订阅被删除时调用）
func (a *App) ClearBaseConfig() error {
	a.mu.Lock()
	a.activeConfig = ""
	a.saveActiveConfig("") // 清空本地记忆

	// ⚠️ 核心修复 4：所有配置清空时，将离线选择一并清除
	a.offlineNodes = make(map[string]string)

	a.mu.Unlock()
	a.saveOfflineNodes()

	// ✅ 改写到安全的数据目录
	destPath := filepath.Join(utils.GetDataDir(), "config.yaml")

	// 写入一个最基础的空结构，防止 Clash 内核解析时直接崩溃
	emptyConfig := "mode: rule\nproxies: []\nproxy-groups: []\nrules: []\n"
	return os.WriteFile(destPath, []byte(emptyConfig), 0644)
}

// 替换1：切换本地配置时，使用流水线生成机制
func (a *App) SelectLocalConfig(fileName string) error {
	fileName = filepath.Base(fileName)

	a.mu.Lock()
	a.activeConfig = fileName
	mode := a.activeMode // 顺便把 mode 一起取出来，后面不用再加锁
	wasActive := a.sysProxyActive || a.tunActive
	a.offlineNodes = make(map[string]string)
	a.mu.Unlock() // ✅ 立即释放锁
	a.saveOfflineNodes()

	// 耗时的磁盘 I/O 放在锁外执行
	a.saveActiveConfig(fileName)

	a.stopCoreService()
	sys.DisableSystemProxy()

	// 使用刚刚在锁内提取出的 mode
	if err := clash.BuildRuntimeConfig(fileName, mode); err != nil {
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
			// ✅ 动态获取端口
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
	} else {
		time.Sleep(200 * time.Millisecond)
	}

	runtime.EventsEmit(a.ctx, "config-changed", fileName)
	return nil
}

// --- 规则管理 (新增) ---

// GetRules 供前端获取规则列表及权限
func (a *App) GetRules() (clash.RuleInfo, error) {
	return clash.GetRules(a.getActiveConfig())
}

// AddRule 增加一条规则到最前面
func (a *App) AddRule(ruleStr string) error {
	info, err := clash.GetRules(a.getActiveConfig())
	if err != nil {
		return err
	}
	if !info.IsEditable {
		return fmt.Errorf("当前配置只读，无法修改规则")
	}

	// 新规则置于顶部
	newRules := append([]string{ruleStr}, info.Rules...)
	if err := clash.SaveRules(a.getActiveConfig(), newRules); err != nil {
		return err
	}

	// 同步到内核：重构 config.yaml
	mode := a.getActiveMode()
	clash.BuildRuntimeConfig(a.getActiveConfig(), mode)

	// 如果内核运行中，触发热重载
	if clash.IsRunning() {
		return clash.UpdateMode(mode)
	}
	return nil
}

// DeleteRule 根据索引删除规则
func (a *App) DeleteRule(index int) error {
	info, err := clash.GetRules(a.getActiveConfig())
	if err != nil {
		return err
	}
	if !info.IsEditable {
		return fmt.Errorf("当前配置只读，无法修改规则")
	}

	if index < 0 || index >= len(info.Rules) {
		return fmt.Errorf("规则索引越界")
	}

	// 移除指定索引的规则
	newRules := append(info.Rules[:index], info.Rules[index+1:]...)

	if err := clash.SaveRules(a.getActiveConfig(), newRules); err != nil {
		return err
	}

	// 同步到内核
	mode := a.getActiveMode()
	clash.BuildRuntimeConfig(a.getActiveConfig(), mode)
	if clash.IsRunning() {
		return clash.UpdateMode(mode)
	}
	return nil
}

// 获取主题配置路径
func getThemeConfigPath() string {
	return filepath.Join(utils.GetDataDir(), "theme_setting.txt")
}

// SaveThemePreference 供前端调用，保存主题模式
func (a *App) SaveThemePreference(isDark bool) {
	theme := "light"
	if isDark {
		theme = "dark"
	}
	_ = os.WriteFile(getThemeConfigPath(), []byte(theme), 0644)
	// 触发全局同步
	a.SyncState()
}

// GetAppState 供前端初始化时主动拉取应用状态
func (a *App) GetAppState() AppState {
	behavior := a.GetAppBehavior()
	theme := "light"
	data, err := os.ReadFile(getThemeConfigPath())
	if err == nil {
		theme = strings.TrimSpace(string(data))
	}
	return AppState{
		IsRunning:   clash.IsRunning(),
		Mode:        a.getActiveMode(),
		Theme:       theme,
		HideLogs:    behavior.HideLogs,
		SystemProxy: a.sysProxyActive,   // 👈 真实系统代理状态
		Tun:         a.tunActive,        // 👈 真实虚拟网卡状态
		Version:     a.GetCoreVersion(), // 👈 当前内核版本
	}
}

// SyncState 统一推送当前应用状态给前端
func (a *App) SyncState() {
	behavior := a.GetAppBehavior()
	theme := "light"
	data, err := os.ReadFile(getThemeConfigPath())
	if err == nil {
		theme = strings.TrimSpace(string(data))
	}

	// 🚀 修复 1：必须加锁读取并发敏感的布尔值
	a.mu.RLock()
	sysProxy := a.sysProxyActive
	tunActive := a.tunActive
	a.mu.RUnlock()

	// 统一组装当前真实状态
	state := AppState{
		IsRunning:   clash.IsRunning(),
		Mode:        a.getActiveMode(), // 这个方法内部自带了安全锁，没问题
		Theme:       theme,
		HideLogs:    behavior.HideLogs,
		SystemProxy: sysProxy,           // 使用安全读取的变量
		Tun:         tunActive,          // 使用安全读取的变量
		Version:     a.GetCoreVersion(),
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
func (a *App) GetAllRules(keyword string) (PagedRules, error) {
	info, err := clash.GetRules(a.getActiveConfig())
	if err != nil {
		return PagedRules{}, err
	}

	var filtered []RuleItem
	keyword = strings.ToLower(keyword)

	// Go 语言层面的极速过滤
	for i, r := range info.Rules {
		if keyword == "" || strings.Contains(strings.ToLower(r), keyword) {
			filtered = append(filtered, RuleItem{Index: i, Text: r})
		}
	}

	return PagedRules{
		Total:      len(filtered),
		Items:      filtered,
		IsEditable: info.IsEditable,
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

// GetCoreVersion 供前端获取当前本地的内核版本
func (a *App) GetCoreVersion() string {
	binDir := utils.GetCoreBinDir()
	exePath := filepath.Join(binDir, "clash.exe")
	v := getLocalCoreVersion(exePath)
	if v == "" {
		return "未知"
	}
	return v
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
	a.stopCoreService()
	time.Sleep(300 * time.Millisecond) // 等待 Windows 彻底释放旧文件占用锁

	backupPath := filepath.Join(binDir, "clash_backup.exe")
	os.Remove(backupPath) // 确保历史备份档为空

	// 6. 重命名：旧版 -> 备份
	renameErr := os.Rename(exePath, backupPath)
	if renameErr != nil && !os.IsNotExist(renameErr) {
		// 锁定旧文件失败(极低概率)，立刻取消操作，恢复运行
		os.Remove(newExePath)
		if wasActive {
			a.ensureCoreRunning()
		}
		return "", fmt.Errorf("内核文件被锁定无法替换: %v", renameErr)
	}

	// 7. 重命名：新版 -> 正式服
	err = os.Rename(newExePath, exePath)
	if err != nil {
		// 灾难恢复：新版更名失败，立刻把老版换回来！
		os.Remove(newExePath)
		_ = os.Rename(backupPath, exePath)
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
			os.Remove(exePath)                 // 摧毁损坏的新内核
			_ = os.Rename(backupPath, exePath) // 复活老内核
			a.ensureCoreRunning()              // 重新启动
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
	a.stopCoreService()
	time.Sleep(200 * time.Millisecond)

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
			_ = os.Rename(targetPath, backupPath)
		}

		if err := os.Rename(tempPath, targetPath); err != nil {
			os.Remove(tempPath)
			_ = os.Rename(backupPath, targetPath) // 失败则秒级回滚
			errs = append(errs, fmt.Sprintf("[%s] 部署失败: %v", task.key, err))
		} else {
			os.Remove(backupPath) // 成功则过河拆桥
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
