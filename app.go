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
	offlineNodes  map[string]string // 👈 新增：存储离线状态下选中的节点
}

// 1. 在 app.go 任意位置新增一个获取程序真实绝对路径的辅助方法
func getBaseDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return "."
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

	configPath := filepath.Join(getBaseDir(), "core", "bin", "config.yaml")
	
	// 👈 核心：内核启动前，动态注入/修复 TUN 配置
	if err := clash.InjectTunConfig(configPath, isTunEnabled); err != nil {
		fmt.Printf("注入 TUN 配置警告: %v\n", err)
	}

	// 启动内核 
	if err := clash.Start(a.ctx); err != nil {
		return err
	}

	// 离线节点状态同步
	a.mu.Lock()
	if len(a.offlineNodes) > 0 {
		nodesCopy := make(map[string]string)
		for k, v := range a.offlineNodes {
			nodesCopy[k] = v
		}
		go func(nodes map[string]string) {
			time.Sleep(600 * time.Millisecond) 
			for g, n := range nodes {
				clash.SwitchProxy(g, n)
			}
		}(nodesCopy)
	}
	a.mu.Unlock()

	// 2. 强制设置系统代理
	if err := sys.SetSystemProxy("127.0.0.1", 7890); err != nil {
		return err
	}

	// 3. 启动流量监控
	go a.StartTrafficStream()
	return nil
}

func NewApp() *App {
	return &App{
		offlineNodes: make(map[string]string), // 👈 初始化 map
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(_ context.Context) {
	sys.ClearSystemProxy()
	clash.Stop()
}

// --- 代理核心控制 ---

// 修改 RunProxy
func (a *App) RunProxy() error {
	return a.startProxyService()
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
	clash.Stop()
	sys.ClearSystemProxy()
	a.StopTrafficStream()
	return nil
}

func (a *App) GetProxyStatus() bool {
	return clash.IsRunning()
}

// --- 配置与测速 ---

func (a *App) GetInitialData() (map[string]interface{}, error) {
	a.mu.Lock()
	activeConfig := a.activeConfig
	if activeConfig == "" {
		activeConfig = a.loadActiveConfig()
	}
	a.mu.Unlock()

	// 👈 核心修复：判断内核是否在运行
	if !clash.IsRunning() {
		// 1. 如果内核没开，直接读本地文件 (离线模式)
		data, err := clash.GetOfflineData(activeConfig)
		if err != nil {
			// 如果连文件也读不到，返回最基础的空结构，防止前端报错
			return map[string]interface{}{"mode": "rule", "groups": make(map[string]interface{})}, nil
		}

		// 👇 新增：将我们离线记录的选中项合并回数据中返回给前端
		a.mu.Lock()
		if groups, ok := data["groups"].(map[string]interface{}); ok {
			for gName, groupData := range groups {
				if gMap, ok2 := groupData.(map[string]interface{}); ok2 {
					// 如果有离线选择，优先使用
					if a.offlineNodes != nil {
						if selNode, exists := a.offlineNodes[gName]; exists {
							gMap["now"] = selNode
						}
					}
					// 如果依旧没有当前选中项，默认选中第一项（防止前端出现空白）
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
		a.mu.Unlock()

		data["activeConfig"] = activeConfig
		data["isOffline"] = true // 标记为离线
		return data, nil
	}

	// 2. 如果内核在运行，尝试从 API 获取 (在线模式)
	data, err := clash.GetInitialData()
	if err != nil {
		// 如果 API 请求由于某种原因失败（比如内核刚启动还没加载完），降级回离线模式
		return clash.GetOfflineData(activeConfig)
	}

	a.mu.Lock()
	data["activeConfig"] = a.activeConfig
	a.mu.Unlock()
	data["isOffline"] = false
	return data, nil
}

func (a *App) TestAllProxies(nodeNames []string) {
	// 1. 静默唤醒机制：如果内核未运行，在后台启动它以提供测速 API
	// 注意：这里只启动进程，不调用 sys.SetSystemProxy，所以不会影响用户的系统网络（真正的离线测试）
	if !clash.IsRunning() {
		if err := clash.Start(a.ctx); err != nil {
			return // 启动失败直接退出
		}
		// 给予内核 1 秒钟的启动和加载配置时间，确保 API 端口(9090)已就绪
		time.Sleep(1 * time.Second)
	}

	// 2. 并发控制（滑动窗口）：严格限制并发数
	// 并发过高会导致内核网络栈或本地连接池雪崩，产生大量无辜的超时(-1)
	// 建议值：8 到 10 之间
	concurrency := 8
	semaphore := make(chan struct{}, concurrency)

	for _, name := range nodeNames {
		semaphore <- struct{}{} // 获取令牌

		go func(nName string) {
			defer func() { <-semaphore }() // 释放令牌

			// 3. 调用内核 HTTP 测速 API（由内核自动处理复杂协议握手和自定义 DNS）
			delay, err := clash.GetProxyDelay(nName)
			if err != nil || delay <= 0 {
				delay = -1 // 发生错误或超时，统一视为 -1
			}

			// 发送给前端更新 UI
			runtime.EventsEmit(a.ctx, "proxy-delay-update", map[string]interface{}{
				"name":  nName,
				"delay": delay,
			})
		}(name)
	}
}

func (a *App) SetConfigMode(mode string) error {
	return clash.UpdateMode(mode)
}

func (a *App) SelectProxy(groupName, nodeName string) error {
	// 👇 新增拦截：如果内核没在运行，不要发送 HTTP 请求报错，而是将其存入离线缓存
	if !clash.IsRunning() {
		a.mu.Lock()
		if a.offlineNodes == nil {
			a.offlineNodes = make(map[string]string)
		}
		a.offlineNodes[groupName] = nodeName
		a.mu.Unlock()
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
	if err == nil && clash.IsRunning() {
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
	if err == nil && clash.IsRunning() {
		// 监听端口的改变和 fake-ip 的劫持需要重启内核
		clash.Stop()
		a.startProxyService()
	}
	return err
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
	// 👈 核心修复：净化文件名，防止类似 "../../Windows" 的路径穿越
	oldName = filepath.Base(oldName)
	newName = filepath.Base(newName)

	if !strings.HasSuffix(newName, ".yaml") && !strings.HasSuffix(newName, ".yml") {
		newName += ".yaml"
	}
	oldPath := filepath.Join(a.getProfilesDir(), oldName)
	newPath := filepath.Join(a.getProfilesDir(), newName)
	return os.Rename(oldPath, newPath)
}

// OpenConfigFile 使用系统默认应用打开配置文件
func (a *App) OpenConfigFile(fileName string) error {
	fileName = filepath.Base(fileName) // 👈 净化
	path := filepath.Join(a.getProfilesDir(), fileName)
	var cmd *exec.Cmd
	switch stdruntime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
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
	a.mu.Unlock()

	baseDir := getBaseDir()
	destPath := filepath.Join(baseDir, "core", "bin", "config.yaml")

	// 写入一个最基础的空结构，防止 Clash 内核解析时直接崩溃
	emptyConfig := "mode: rule\nproxies: []\nproxy-groups: []\nrules: []\n"
	return os.WriteFile(destPath, []byte(emptyConfig), 0644)
}

// 替换1：切换本地配置时，使用流水线生成机制
func (a *App) SelectLocalConfig(fileName string) error {
	fileName = filepath.Base(fileName) // 净化前端传来的文件名

	a.mu.Lock()
	a.activeConfig = fileName
	a.saveActiveConfig(fileName) // 持久化保存到本地
	a.mu.Unlock()

	// 记住切换前内核是否在运行
	wasRunning := clash.IsRunning()

	// 1. 先彻底停止现有的内核和代理，释放文件占用
	clash.Stop()
	sys.ClearSystemProxy()

	// 2. 调用注入器流水线：读取选中的 yaml，注入用户 DNS/TUN，生成 config.yaml
	if err := clash.BuildRuntimeConfig(fileName); err != nil {
		return fmt.Errorf("生成运行时配置失败: %v", err)
	}

	// 3. 如果之前在运行，就重新启动代理加载新配置
	if wasRunning {
		if err := a.startProxyService(); err != nil {
			return err
		}
	}

	time.Sleep(800 * time.Millisecond)
	runtime.EventsEmit(a.ctx, "config-changed", fileName)
	return nil
}

