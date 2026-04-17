package main

import (
	"context"
	"fmt"
	"goclashz/core/clash"
	"goclashz/core/sys"
	"goclashz/core/traffic"
)

// App struct
type App struct {
	ctx context.Context
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// shutdown 执行清理操作
func (a *App) shutdown(ctx context.Context) {
	fmt.Println("正在关闭应用，清理系统代理...")
	clash.Stop()
	sys.ClearSystemProxy()
}

// StartTrafficStream 供前端启动时调用
func (a *App) StartTrafficStream() {
	traffic.StartTrafficMonitor(a.ctx)
}

// StopTrafficStream 供前端停止或应用退出时调用
func (a *App) StopTrafficStream() {
	traffic.StopTrafficMonitor()
}

// ================== 核心代理控制 ==================

func (a *App) RunProxy() error {
	// 传入 a.ctx 给内核，以便内核崩溃时可以发送事件
	err := clash.Start(a.ctx)
	if err != nil {
		return err // 前端直接抛出异常
	}

	err = sys.SetSystemProxy("127.0.0.1", 7890)
	if err != nil {
		return fmt.Errorf("内核已启动，但系统代理接管失败: %v", err)
	}

	go a.StartTrafficStream()
	go a.StartStreamingLogs()

	return nil // 返回 nil，前端 Promise resolve
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

// ================== 数据交互与配置 ==================

// GetInitialData 启动前读取离线节点和模式
func (a *App) GetInitialData() map[string]interface{} {
	mode, groups, err := clash.GetStaticNodes()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{
		"mode":   mode,
		"groups": groups,
	}
}

// SetConfigMode 切换 Rule/Global/Direct
func (a *App) SetConfigMode(mode string) error {
	err := clash.UpdateMode(mode)
	if err != nil {
		return fmt.Errorf("切换失败: %v", err)
	}
	return nil
}

// UpdateSubscription 订阅下载更新
func (a *App) UpdateSubscription(url string, ua string) string {
	err := clash.DownloadSubscription(url, ua)
	if err != nil {
		return "更新失败: " + err.Error()
	}
	return "✅ 订阅更新成功"
}

// ================== 测速引擎 ==================

// GetNodeDelay 在线测速 (调用 Clash API)
func (a *App) GetNodeDelay(name string, testUrl string) int {
	delay, err := clash.GetProxyDelay(name, testUrl)
	if err != nil {
		return -1
	}
	return delay
}

// GetOfflineDelay 离线测速 (纯 TCP 探测)
func (a *App) GetOfflineDelay(name string) int {
	addrList, err := clash.GetRawProxyAddrs()
	if err != nil {
		return -1
	}
	for _, n := range addrList {
		if n.Name == name {
			return clash.TCPPing(n.Server, n.Port)
		}
	}
	return -1
}

// StartAsyncTest 启动高并发异步测速 (Stelliberty 风格)
func (a *App) StartAsyncTest(groupName string) string {
	allNodes, err := clash.GetRawProxyAddrs()
	if err != nil {
		return "获取配置失败"
	}

	_, groups, _ := clash.GetStaticNodes()
	var targetNodes []clash.RawProxyInfo
	for _, g := range groups {
		if g.Name == groupName {
			for _, pName := range g.Proxies {
				for _, raw := range allNodes {
					if raw.Name == pName {
						targetNodes = append(targetNodes, raw)
					}
				}
			}
		}
	}

	// 开启协程执行，通过 Wails Events 推送结果
	go clash.BatchTestNodes(a.ctx, targetNodes)
	return "🚀 测速开始"
}

// ================== 节点选择 ==================

func (a *App) GetProxyNodes() []clash.ProxyNode {
	nodes, err := clash.GetProxies()
	if err != nil {
		return nil
	}
	return nodes
}

func (a *App) SelectProxy(groupName, nodeName string) error {
	err := clash.SwitchProxy(groupName, nodeName)
	if err != nil {
		return err
	}
	return nil
}

// 在 app.go 中增加

// StartStreamingLogs 启动日志推送
func (a *App) StartStreamingLogs() {
	go clash.StartLogStream(a.ctx)
}

// UpdateClashSettings 更新特性设置
func (a *App) UpdateClashSettings(settings map[string]interface{}) string {
	err := clash.PatchConfig(settings)
	if err != nil {
		return "修改失败: " + err.Error()
	}
	return "✅ 特性已更新"
}

// 在 app.go 中添加

// CheckTunEnv 检查 TUN 环境：返回 (是否是管理员, 是否有 dll)
func (a *App) CheckTunEnv() map[string]bool {
	return map[string]bool{
		"isAdmin":   sys.CheckAdmin(),
		"hasWintun": sys.CheckWintun(),
	}
}
