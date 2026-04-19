package main

import (
	"context"
	"fmt"
	"goclashz/core/clash"
	"goclashz/core/sys"
	"goclashz/core/traffic"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx           context.Context
	cancelTraffic context.CancelFunc
	cancelLogs    context.CancelFunc
	logRunning    bool
	mu            sync.Mutex
	activeConfig  string
	activeMode    string            // 👈 新增：存储当前路由模式 (rule, global, direct)
	offlineNodes  map[string]string // 👈 新增：存储离线状态下选中的节点
	sysProxyActive bool             // 👈 替换：系统代理是否开启
	tunActive      bool             // 👈 替换：TUN 模式是否开启
}

// ProxyStatus 新增给前端返回的双重状态结构
type ProxyStatus struct {
	SystemProxy bool `json:"systemProxy"`
	Tun         bool `json:"tun"`
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
func getBaseDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return "."
	}
	// ⚠️ 核心修复：识别 Go / Wails 的开发模式临时目录，强制回退到当前工作目录
	if strings.Contains(exePath, "go-build") || strings.Contains(os.TempDir(), filepath.Dir(exePath)) || strings.Contains(exePath, "wails-dev") {
		wd, err := os.Getwd()
		if err == nil {
			return wd
		}
	}
	return filepath.Dir(exePath)
}

// 记录当前选中的配置文件名到本地
func (a *App) saveActiveConfig(fileName string) {
	baseDir := getBaseDir()
	activeFile := filepath.Join(baseDir, "core", "bin", "active_config.txt")
	os.WriteFile(activeFile, []byte(fileName), 0644)
}

// 启动时读取上次选中的配置文件名
func (a *App) loadActiveConfig() string {
	baseDir := getBaseDir()
	activeFile := filepath.Join(baseDir, "core", "bin", "active_config.txt")
	data, err := os.ReadFile(activeFile)
	if err == nil && len(data) > 0 {
		return string(data)
	}
	return ""
}

// 记录当前选中的模式到本地
func (a *App) saveActiveMode(mode string) {
	baseDir := getBaseDir()
	activeFile := filepath.Join(baseDir, "core", "bin", "active_mode.txt")
	os.WriteFile(activeFile, []byte(mode), 0644)
}

// 启动时读取上次选中的模式
func (a *App) loadActiveMode() string {
	baseDir := getBaseDir()
	activeFile := filepath.Join(baseDir, "core", "bin", "active_mode.txt")
	data, err := os.ReadFile(activeFile)
	if err == nil && len(data) > 0 {
		return string(data)
	}
	return "rule" // 默认规则模式
}

// --- 状态获取辅助方法（新增） ---

func (a *App) getActiveConfig() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	// 如果内存为空，则从本地文件读取并缓存到内存
	if a.activeConfig == "" {
		a.activeConfig = a.loadActiveConfig()
	}
	return a.activeConfig
}

func (a *App) getActiveMode() string {
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

	// ⚠️ 核心修复 2：将异步改为同步阻塞，等待内核 HTTP API 真正就绪
	apiReady := false
	for i := 0; i < 20; i++ { // 最长等待 2 秒
		time.Sleep(100 * time.Millisecond)
		// 用获取初始数据作为 API 就绪的探针
		if _, err := clash.GetInitialData(); err == nil {
			apiReady = true
			break
		}
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
	clash.Stop()
	a.StopTrafficStream()
}

// ==========================================
// --- 暴露给前端的 API ---
// ==========================================

// GetProxyStatus 获取当前双轨状态
func (a *App) GetProxyStatus() ProxyStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// 读取真实配置作为校验
	tunCfg, _ := clash.GetTunConfig()
	realTun := tunCfg != nil && tunCfg.Enable && clash.IsRunning()

	return ProxyStatus{
		SystemProxy: a.sysProxyActive,
		Tun:         realTun,
	}
}

// ToggleSystemProxy 开关 1：系统代理
func (a *App) ToggleSystemProxy(enable bool) error {
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
		// 2. 开启 Windows 系统代理
		bypass := "localhost;127.*;10.*;172.16.*;192.168.*;<local>"
		return sys.EnableSystemProxy("127.0.0.1", 7890, bypass)
	} else {
		// 1. 关闭 Windows 系统代理
		sys.DisableSystemProxy()
		// 2. 如果虚拟网卡也没开，那就彻底关闭内核节约资源
		if !needCore {
			a.stopCoreService()
		}
		return nil
	}
}

// ToggleTunMode 开关 2：虚拟网卡 (TUN)
func (a *App) ToggleTunMode(enable bool) error {
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
		time.Sleep(300 * time.Millisecond) // 等待旧端口释放
		return a.ensureCoreRunning()
	}
	return nil
}

func NewApp() *App {
	return &App{
		offlineNodes: make(map[string]string),
		activeMode:   "", // 留空，待 loadActiveMode 加载
		sysProxyActive: false,
		tunActive:      false,
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	// ⚠️ 核心逻辑：退出时强制恢复网络环境
	fmt.Println("正在关闭 GoclashZ，正在清理网络代理设置...")

	_ = a.ToggleSystemProxy(false) // 关闭系统代理
	_ = a.ToggleTunMode(false)    // 关闭虚拟网卡
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
	return nil
}

// 注意：此方法名与新 GetProxyStatus 冲突，我已在上方实现了返回 ProxyStatus 结构体的新方法。
// 为了兼容 App.vue 的布尔值判断，我们保留一个简单的 IsCoreRunning 逻辑或者让前端适配。
// 这里我们将旧的 GetProxyStatus 逻辑合并到 New API 中。

// --- 配置与测速 ---

func (a *App) GetInitialData() (map[string]interface{}, error) {
	// 👈 核心修复：通过安全方法获取准确的上下文
	activeConfig := a.getActiveConfig()
	mode := a.getActiveMode()

	if !clash.IsRunning() {
		data, err := clash.GetOfflineData(activeConfig)
		if err != nil {
			return map[string]interface{}{"mode": mode, "groups": make(map[string]interface{})}, nil
		}
		
		a.mergeOfflineNodes(data) 

		data["activeConfig"] = activeConfig
		data["mode"] = mode
		data["isOffline"] = true
		return data, nil
	}

	data, err := clash.GetInitialData()
	if err != nil {
		// API 宕机/未就绪时触发降级
		fallbackData, _ := clash.GetOfflineData(activeConfig)
		if fallbackData != nil {
			a.mergeOfflineNodes(fallbackData)
			fallbackData["activeConfig"] = activeConfig
			fallbackData["isOffline"] = true
			fallbackData["mode"] = mode // ✅ 使用准确的模式
			return fallbackData, nil
		}
		return map[string]interface{}{"mode": "rule", "groups": make(map[string]interface{})}, nil
	}

	// ✅ 核心修复：直接使用准确的局部变量，不要再用可能为空的 a.activeConfig
	data["activeConfig"] = activeConfig
	data["mode"] = mode
	data["isOffline"] = false

	// 注入节点组原始排序
	baseDir := getBaseDir()
	configPath := filepath.Join(baseDir, "core", "bin", activeConfig)
	if activeConfig == "" || activeConfig == "config.yaml" {
		configPath = filepath.Join(baseDir, "core", "bin", "config.yaml")
	}
	if yamlData, err := os.ReadFile(configPath); err == nil {
		data["groupOrder"] = clash.ExtractGroupOrder(yamlData)
	}

	return data, nil
}

func (a *App) TestAllProxies(nodeNames []string) {
	if !clash.IsRunning() {
		if err := clash.Start(a.ctx); err != nil {
			// ⚠️ 修复：不要静默 return，通知前端测速异常结束
			runtime.EventsEmit(a.ctx, "proxy-test-finished", "内核启动失败，无法测速")
			return 
		}
		time.Sleep(1 * time.Second)
	}

	go func() {
		concurrency := 8
		semaphore := make(chan struct{}, concurrency)
		var wg sync.WaitGroup
		
		// ⚠️ 修复：为整个测速任务设置一个最高 15 秒的超时 Context，防止 HTTP 卡死
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		for _, name := range nodeNames {
			wg.Add(1)
			
			go func(nName string) {
				defer wg.Done()
				
				select {
				case semaphore <- struct{}{}: // 获取令牌
				case <-ctx.Done(): // 整体超时，直接退出
					return 
				}
				defer func() { <-semaphore }()

				// 假设 GetProxyDelay 内部也有超时控制，否则这里依然需要配合 ctx 改造
				delay, err := clash.GetProxyDelay(nName)
				if err != nil || delay <= 0 {
					delay = -1 
				}

				runtime.EventsEmit(a.ctx, "proxy-delay-update", map[string]interface{}{
					"name":  nName,
					"delay": delay,
				})
			}(name)
		}

		wg.Wait()
		runtime.EventsEmit(a.ctx, "proxy-test-finished", "测速完成")
	}()
}

func (a *App) UpdateClashMode(mode string) error {
	// 1. 持久化到本地和内存
	a.mu.Lock()
	a.activeMode = mode
	a.saveActiveMode(mode)
	isRunning := clash.IsRunning()
	a.mu.Unlock()

	// 2. 如果内核在运行，动态下发指令
	if isRunning {
		return clash.UpdateMode(mode)
	}

	// 3. 如果内核没运行，只改文件
	activeCfg := a.getActiveConfig() // 👈 替换为安全获取

	if activeCfg != "" {
		return clash.BuildRuntimeConfig(activeCfg, mode)
	}
	return nil
}

func (a *App) SelectProxy(groupName, nodeName string) error {
	// ⚠️ 核心修复 1：无论内核是否运行，都将用户的选择同步记录到离线缓存中
	// 防止在线时切换了节点，重启内核后又被还原为老节点
	a.mu.Lock()
	if a.offlineNodes == nil {
		a.offlineNodes = make(map[string]string)
	}
	a.offlineNodes[groupName] = nodeName
	a.mu.Unlock()

	if !clash.IsRunning() {
		return nil
	}
	return clash.SwitchProxy(groupName, nodeName)
}

func (a *App) UpdateSub(url string) error {
	return clash.UpdateSubscription(url)
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
	a.logRunning = true
	// 👈 创建独立的子 Context 控制日志
	logCtx, cancel := context.WithCancel(a.ctx)
	a.cancelLogs = cancel
	a.mu.Unlock()

	// 调用 api_client.go 中定义的 FetchLogs，传入受控的 logCtx
	go clash.FetchLogs(logCtx)
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


// 3. 提供给前端：安装驱动
func (a *App) InstallTunDriver() error {
	// 直接调用重构后的方法，它已内部集成了专属的 ZIP 解析逻辑
	return sys.InstallWintun()
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
	return clash.GetConnections()
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
	return filepath.Join(getBaseDir(), "core", "bin")
}

// 修改 GetLocalConfigs，排除 config.yaml
func (a *App) GetLocalConfigs() ([]string, error) {
	dir := a.getProfilesDir()
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var configs []string
	for _, file := range files {
		// ⚠️ 关键：排除 config.yaml，它不是用户的订阅源，只是运行时副本
		if !file.IsDir() && file.Name() != "config.yaml" &&
			(strings.HasSuffix(file.Name(), ".yaml") || strings.HasSuffix(file.Name(), ".yml")) {
			configs = append(configs, file.Name())
		}
	}
	return configs, nil
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

	// ⚠️ 修复：如果正在重命名当前处于活动状态的配置，必须先停止代理内核释放文件锁
	a.mu.Lock()
	isActiveConfig := (a.activeConfig == oldName)
	a.mu.Unlock()
	
	if isActiveConfig && clash.IsRunning() {
		clash.Stop()
		time.Sleep(200 * time.Millisecond) // 等待文件句柄释放
	}

	renameFunc := func() error {
		if strings.EqualFold(oldName, newName) && oldName != newName {
			tempPath := newPath + ".tmp"
			if err := os.Rename(oldPath, tempPath); err != nil { return err }
			return os.Rename(tempPath, newPath)
		}
		return os.Rename(oldPath, newPath)
	}

	err := renameFunc()

	// ⚠️ 修复：如果刚才停掉了内核，重命名完成后需要重启
	if isActiveConfig {
		a.mu.Lock()
		a.activeConfig = newName // 更新内部记录
		a.saveActiveConfig(newName)
		a.mu.Unlock()
		
		// 👈 核心修复：使用 getActiveMode 防止状态为空
		mode := a.getActiveMode() 
		clash.BuildRuntimeConfig(newName, mode)
		a.ensureCoreRunning()
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

	baseDir := getBaseDir()
	destPath := filepath.Join(baseDir, "core", "bin", "config.yaml")

	// 写入一个最基础的空结构，防止 Clash 内核解析时直接崩溃
	emptyConfig := "mode: rule\nproxies: []\nproxy-groups: []\nrules: []\n"
	return os.WriteFile(destPath, []byte(emptyConfig), 0644)
}

// 替换1：切换本地配置时，使用流水线生成机制
func (a *App) SelectLocalConfig(fileName string) error {
	fileName = filepath.Base(fileName)

	a.mu.Lock()
	a.activeConfig = fileName
	a.saveActiveConfig(fileName)
	wasActive := a.sysProxyActive || a.tunActive
	
	// ⚠️ 核心修复 3：切换配置时清空前一个配置的离线记忆，防止策略组名称冲突
	a.offlineNodes = make(map[string]string) 
	
	a.mu.Unlock()

	a.stopCoreService()
	sys.DisableSystemProxy()

	a.mu.Lock()
	mode := a.activeMode
	a.mu.Unlock()
	if err := clash.BuildRuntimeConfig(fileName, mode); err != nil {
		return fmt.Errorf("生成运行时配置失败: %v", err)
	}

	if wasActive {
		if err := a.ensureCoreRunning(); err != nil {
			return err
		}
		// 如果刚刚关掉前系统代理是开的，由于前面的 DisableSystemProxy，现在需要重新挂上
		a.mu.Lock()
		sysProxy := a.sysProxyActive
		a.mu.Unlock()
		if sysProxy {
			bypass := "localhost;127.*;10.*;172.16.*;192.168.*;<local>"
			sys.EnableSystemProxy("127.0.0.1", 7890, bypass)
		}
	} else {
		// 如果只是离线切换，稍微让文件系统缓一下即可
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

// 获取存储主题配置的路径
func getThemeConfigPath() string {
	configDir, _ := os.UserConfigDir()
	appDir := filepath.Join(configDir, "GoclashZ")
	os.MkdirAll(appDir, 0755)
	return filepath.Join(appDir, "theme_setting.txt")
}

// SaveThemePreference 供前端调用，保存主题模式
func (a *App) SaveThemePreference(isDark bool) {
	theme := "light"
	if isDark {
		theme = "dark"
	}
	_ = os.WriteFile(getThemeConfigPath(), []byte(theme), 0644)
}
