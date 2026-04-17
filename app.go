package main

import (
	"context"
	"fmt"
	"goclashz/core/clash"
	"goclashz/core/sys"
	"goclashz/core/traffic"
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
