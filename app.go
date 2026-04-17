package main

import (
	"context"
	"fmt"
	"goclashz/core/clash"
	"goclashz/core/sys"
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

// ================== 核心代理控制 ==================

func (a *App) RunProxy() string {
	err := clash.Start()
	if err != nil {
		return err.Error()
	}
	// 设置系统代理 (7890 对应配置中的 mixed-port)
	err = sys.SetSystemProxy("127.0.0.1", 7890)
	if err != nil {
		return "内核已启动，但代理接管失败: " + err.Error()
	}
	return "✅ 代理已启动并接管系统"
}

func (a *App) StopProxy() string {
	clash.Stop()
	sys.ClearSystemProxy()
	return "🛑 代理已停止"
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
func (a *App) SetConfigMode(mode string) string {
	err := clash.UpdateMode(mode)
	if err != nil {
		return "切换失败: " + err.Error()
	}
	return "✅ 模式已切换"
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

func (a *App) SelectProxy(groupName, nodeName string) string {
	err := clash.SwitchProxy(groupName, nodeName)
	if err != nil {
		return err.Error()
	}
	return "✅ 已切换"
}
