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
	isProxyActive bool              // 👈 新增：记录用户逻辑上的代理开关状态
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

// 在 App 结构体下新增一个内部方法
func (a *App) startProxyService() error {
	tunCfg, _ := clash.GetTunConfig()
	isTunEnabled := tunCfg != nil && tunCfg.Enable

	if isTunEnabled {
		// 环境拦截
		if !sys.IsWintunInstalled() {
			return fmt.Errorf("缺失 Wintun 驱动，请先安装")
		}
		if !sys.CheckAdmin() {
			// 直接触发提权，而不是让用户干瞪眼
			sys.RequestAdmin()
			return fmt.Errorf("正在请求系统管理员权限，请在弹窗中允许...")
		}
	}

	a.mu.Lock()
	mode := a.activeMode
	if mode == "" {
		mode = a.loadActiveMode()
		a.activeMode = mode
	}
	// 获取当前配置文件名
	activeCfg := a.activeConfig
	if activeCfg == "" {
		activeCfg = a.loadActiveConfig()
	}
	a.mu.Unlock()

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

	// 2. 强制设置系统代理
	bypass := "localhost;127.*;10.*;172.16.*;192.168.*;<local>"
	if err := sys.EnableSystemProxy("127.0.0.1", 7890, bypass); err != nil {
		return err
	}

	// 3. 启动流量监控
	go a.StartTrafficStream()
	return nil
}

func NewApp() *App {
	return &App{
		offlineNodes: make(map[string]string),
		activeMode:   "", // 留空，待 loadActiveMode 加载
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(_ context.Context) {
	sys.DisableSystemProxy()
	clash.Stop()
}

// --- 代理核心控制 ---

// 修改 RunProxy
func (a *App) RunProxy() error {
	err := a.startProxyService()
	if err == nil {
		a.mu.Lock()
		a.isProxyActive = true // 👈 记录用户主动开启
		a.mu.Unlock()
	}
	return err
}

// 1. 提供给前端：检查 TUN 模式环境（驱动 + 权限）
func (a *App) CheckTunEnv() map[string]bool {
	return map[string]bool{
		"isAdmin":   sys.CheckAdmin(),
		"hasWintun": sys.IsWintunInstalled(),
	}
}

// 2. 提供给前端：自动提权并重启应用
func (a *App) ElevatePrivileges() error {
	return sys.RequestAdmin() // 将会呼出 UAC 窗口并重启软件
}

func (a *App) StopProxy() error {
	a.mu.Lock()
	a.isProxyActive = false // 👈 记录用户主动关闭
	a.mu.Unlock()

	clash.Stop()
	sys.DisableSystemProxy()
	a.StopTrafficStream()
	return nil
}

func (a *App) GetProxyStatus() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.isProxyActive // 👈 前端 UI 开关状态绑定到这里
}

// --- 配置与测速 ---

func (a *App) GetInitialData() (map[string]interface{}, error) {
	a.mu.Lock()
	activeConfig := a.activeConfig
	if activeConfig == "" {
		activeConfig = a.loadActiveConfig()
	}
	mode := a.activeMode
	if mode == "" {
		mode = a.loadActiveMode()
	}
	a.mu.Unlock()

	if !clash.IsRunning() {
		data, err := clash.GetOfflineData(activeConfig)
		if err != nil {
			return map[string]interface{}{"mode": mode, "groups": make(map[string]interface{})}, nil
		}
		
		a.mergeOfflineNodes(data) // 👈 使用辅助方法合并

		data["activeConfig"] = activeConfig
		data["mode"] = mode
		data["isOffline"] = true
		return data, nil
	}

	data, err := clash.GetInitialData()
	if err != nil {
		// API 宕机/未就绪时触发降级，同样需要合并离线节点，防止 UI 重置跳变
		fallbackData, _ := clash.GetOfflineData(activeConfig)
		if fallbackData != nil {
			a.mergeOfflineNodes(fallbackData)
			fallbackData["activeConfig"] = activeConfig
			fallbackData["isOffline"] = true

			// ✅ 修复：把内存中最新的 mode 覆盖给 fallback，防止从 YAML 读到旧数据导致界面跳变
			a.mu.Lock()
			if a.activeMode != "" {
				fallbackData["mode"] = a.activeMode
			}
			a.mu.Unlock()

			return fallbackData, nil
		}
		return map[string]interface{}{"mode": "rule", "groups": make(map[string]interface{})}, nil
	}

	a.mu.Lock()
	data["activeConfig"] = a.activeConfig
	data["mode"] = a.activeMode
	a.mu.Unlock()
	data["isOffline"] = false
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

	// 3. 如果内核没运行，只改文件（防止下次启动时丢失）
	a.mu.Lock()
	activeCfg := a.activeConfig
	if activeCfg == "" {
		activeCfg = a.loadActiveConfig()
	}
	a.mu.Unlock()

	if activeCfg != "" {
		// 直接利用现成的流水线重构 config.yaml
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

	// ⚠️ 核心修复：将阻塞的轮询放入后台 Goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				up, down := traffic.GetTraffic()
				runtime.EventsEmit(a.ctx, "traffic-data", map[string]string{"up": up, "down": down})
			}
		}
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
	isActive := a.isProxyActive
	a.mu.Unlock()

	if err == nil && isActive { // 👈 核心修复
		// TUN 模式的开启和关闭必须重启内核才能生效
		clash.Stop()
		a.startProxyService()
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
	isActive := a.isProxyActive
	a.mu.Unlock()

	if err == nil && isActive { // 👈 核心修复
		// 监听端口的改变和 fake-ip 的劫持需要重启内核
		clash.Stop()
		a.startProxyService()
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
	isActive := a.isProxyActive
	a.mu.Unlock()

	// 这些设置直接影响内核底层行为，需要重启内核生效
	if err == nil && isActive {
		clash.Stop()
		a.startProxyService()
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
		// 重新生成配置并启动
		a.mu.Lock()
		mode := a.activeMode
		a.mu.Unlock()
		clash.BuildRuntimeConfig(newName, mode)
		a.startProxyService()
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
	wasActive := a.isProxyActive
	
	// ⚠️ 核心修复 3：切换配置时清空前一个配置的离线记忆，防止策略组名称冲突
	a.offlineNodes = make(map[string]string) 
	
	a.mu.Unlock()

	clash.Stop()
	sys.DisableSystemProxy()

	a.mu.Lock()
	mode := a.activeMode
	a.mu.Unlock()
	if err := clash.BuildRuntimeConfig(fileName, mode); err != nil {
		return fmt.Errorf("生成运行时配置失败: %v", err)
	}

	if wasActive {
		if err := a.startProxyService(); err != nil {
			return err
		}
		// ⚠️ 核心修复：使用轮询检测内核复活，替代卡死界面的硬编码 time.Sleep
		for i := 0; i < 20; i++ {
			time.Sleep(100 * time.Millisecond)
			if clash.IsRunning() {
				break
			}
		}
	} else {
		// 如果只是离线切换，稍微让文件系统缓一下即可
		time.Sleep(200 * time.Millisecond)
	}

	runtime.EventsEmit(a.ctx, "config-changed", fileName)
	return nil
}

