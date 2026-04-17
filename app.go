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
	logRunning    bool
	mu            sync.Mutex
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	sys.ClearSystemProxy()
	clash.Stop()
}

// --- 代理核心控制 ---

func (a *App) RunProxy() error {
	// ⚠️ 移除任何试图安装或强制检查 Wintun 的前置逻辑
	// 仅启动 Clash 内核
	if err := clash.Start(a.ctx); err != nil {
		return err
	}

	// 设置系统代理 (HTTP/Socks 模式)
	// 这里不再强制要求管理员权限，因为设置系统代理通常不需要它
	if err := sys.SetSystemProxy("127.0.0.1", 7890); err != nil {
		return err
	}

	go a.StartTrafficStream()
	return nil
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
	return clash.GetInitialData()
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

// --- 实时流管理 ---

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
	a.mu.Unlock()
	// 调用 api_client.go 中定义的 FetchLogs
	go clash.FetchLogs(a.ctx)
}

func (a *App) StopStreamingLogs() {
	a.mu.Lock()
	a.logRunning = false
	a.mu.Unlock()
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
	// 假定配置放在 core/bin 下
	return filepath.Join(".", "core", "bin")
}

// GetLocalConfigs 获取所有本地配置文件列表
func (a *App) GetLocalConfigs() ([]string, error) {
	dir := a.getProfilesDir()
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var configs []string
	for _, file := range files {
		if !file.IsDir() && (strings.HasSuffix(file.Name(), ".yaml") || strings.HasSuffix(file.Name(), ".yml")) {
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
	if !strings.HasSuffix(newName, ".yaml") && !strings.HasSuffix(newName, ".yml") {
		newName += ".yaml"
	}
	oldPath := filepath.Join(a.getProfilesDir(), oldName)
	newPath := filepath.Join(a.getProfilesDir(), newName)
	return os.Rename(oldPath, newPath)
}

// OpenConfigFile 使用系统默认应用打开配置文件
func (a *App) OpenConfigFile(fileName string) error {
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
	path := filepath.Join(a.getProfilesDir(), fileName)
	return os.Remove(path)
}
