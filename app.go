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
	cancelLogs    context.CancelFunc // 👈 新增：用于专门控制日志流的取消
	logRunning    bool
	mu            sync.Mutex
	activeConfig  string
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
	// 1. 启动内核 (现在是幂等的，运行中也会返回 nil)
	if err := clash.Start(a.ctx); err != nil {
		return err
	}

	// 2. 强制设置系统代理
	if err := sys.SetSystemProxy("127.0.0.1", 7890); err != nil {
		return err
	}

	// 3. 启动流量监控
	go a.StartTrafficStream()
	return nil
}

func NewApp() *App {
	return &App{}
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

// 只有在用户手动点击“检查环境”时才调用
func (a *App) CheckTunEnv() map[string]bool {
	return map[string]bool{
		"isAdmin":   sys.CheckAdmin(),
		"hasWintun": sys.CheckWintun(),
	}
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

// 👈 新增：离线测速方法（不依赖内核，直接 TCP 握手）
func (a *App) TestOfflineNodes() error {
	proxies, err := clash.GetRawProxyAddrs() // 从 yaml 解析物理地址
	if err != nil {
		return err
	}

	// 并发 15 个测速
	semaphore := make(chan struct{}, 15)
	for _, p := range proxies {
		semaphore <- struct{}{}
		go func(info clash.RawProxyInfo) {
			defer func() { <-semaphore }()
			delay := clash.TCPPing(info.Server, info.Port)
			// 发送给前端更新 UI
			runtime.EventsEmit(a.ctx, "proxy-delay-update", map[string]interface{}{
				"name":  info.Name,
				"delay": delay,
			})
		}(p)
	}
	return nil
}

func (a *App) SetConfigMode(mode string) error {
	return clash.UpdateMode(mode)
}

func (a *App) SelectProxy(groupName, nodeName string) error {
	return clash.SwitchProxy(groupName, nodeName)
}

func (a *App) UpdateSub(url string) error {
	return clash.UpdateSubscription(url)
}

func (a *App) TestAllProxies(nodeNames []string) {
	// 信号量控制 15 并发，实现数字瀑布流视觉效果
	semaphore := make(chan struct{}, 15)
	for _, name := range nodeNames {
		semaphore <- struct{}{}
		go func(nName string) {
			defer func() { <-semaphore }()
			delay, _ := clash.GetProxyDelay(nName)
			runtime.EventsEmit(a.ctx, "proxy-delay-update", map[string]interface{}{
				"name":  nName,
				"delay": delay,
			})
		}(name)
	}
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

func (a *App) SaveTunConfig(cfg *clash.TunConfig) error {
	err := clash.UpdateTunConfig(cfg)
	if err == nil && clash.IsRunning() {
		// 可选：如果内核在运行且修改了配置，你可以在这执行重启
		// clash.Stop()
		// clash.Start(a.ctx)
	}
	return err
}

func (a *App) InstallTunDriver() error {
	if !sys.CheckAdmin() {
		return fmt.Errorf("请右键以管理员身份运行本程序来进行驱动安装")
	}
	// 在这里可以放置你针对 wintun.dll 下载/解压的具体逻辑
	// 如果用户已有 wintun.dll，CheckTunEnv 会自动变为 true
	return nil
}
func (a *App) GetDNSConfig() (*clash.DNSConfig, error) {
	return clash.GetDNSConfig()
}

func (a *App) SaveDNSConfig(cfg *clash.DNSConfig) error {
	err := clash.UpdateDNSConfig(cfg)
	if err == nil && clash.IsRunning() {
		// 可选：重启内核
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

// 修改 SelectLocalConfig
func (a *App) SelectLocalConfig(fileName string) error {
	a.mu.Lock()
	a.activeConfig = fileName
	a.saveActiveConfig(fileName) // 👈 持久化保存到本地
	a.mu.Unlock()

	// 👈 记住切换前内核是否在运行
	wasRunning := clash.IsRunning()

	// 先彻底停止现有的内核和代理
	clash.Stop()
	sys.ClearSystemProxy()

	fileName = filepath.Base(fileName) // 净化前端传来的文件名

	// 👈 路径全部改为使用 getBaseDir() 作为基准
	baseDir := getBaseDir()
	sourcePath := filepath.Join(baseDir, "core", "bin", fileName)
	destPath := filepath.Join(baseDir, "core", "bin", "config.yaml")

	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	os.WriteFile(destPath, content, 0644)

	// 👈 核心修改：如果之前在运行，就重新启动代理。如果没在运行，就不启动！
	if wasRunning {
		if err := a.startProxyService(); err != nil {
			return err
		}
	}

	time.Sleep(800 * time.Millisecond)
	runtime.EventsEmit(a.ctx, "config-changed", fileName)
	return nil
}
