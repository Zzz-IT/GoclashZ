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

// shutdown 确保在用户关闭软件时，内核被杀死，且系统代理被恢复！
func (a *App) shutdown(ctx context.Context) {
	fmt.Println("应用正在退出，执行清理操作...")
	clash.Stop()
	sys.ClearSystemProxy() // 👉 极其重要：防止软件退出后用户电脑断网
}

// RunProxy 供前端点击“启动”时调用
func (a *App) RunProxy() string {
	err := clash.Start()
	if err != nil {
		return err.Error()
	}

	// 👉 内核启动成功后，去修改 Windows 系统代理
	// 注意：这里的 7890 对应你 config.yaml 中的 mixed-port
	err = sys.SetSystemProxy("127.0.0.1", 7890)
	if err != nil {
		return "内核已启动，但设置系统代理失败: " + err.Error()
	}

	return "✅ 代理核心已启动，系统代理接管成功！"
}

// StopProxy 供前端点击“停止”时调用
func (a *App) StopProxy() string {
	err := clash.Stop()
	if err != nil {
		return err.Error()
	}

	// 👉 内核停止后，必须关闭 Windows 系统代理
	err = sys.ClearSystemProxy()
	if err != nil {
		return "内核已停止，但清理系统代理失败: " + err.Error()
	}

	return "🛑 代理核心已停止，系统代理已恢复"
}

// GetProxyStatus 供前端在刚打开软件时，查询当前代理是不是活着的
func (a *App) GetProxyStatus() bool {
	return clash.IsRunning()
}

// GetProxyNodes 获取节点列表供前端展示
func (a *App) GetProxyNodes() []clash.ProxyNode {
	// 只有在内核运行的情况下才去请求 API
	if !clash.IsRunning() {
		return nil
	}

	nodes, err := clash.GetProxies()
	if err != nil {
		fmt.Println("获取节点报错:", err)
		return nil
	}
	return nodes
}

// SelectProxy 供前端点击节点时调用
func (a *App) SelectProxy(groupName, nodeName string) string {
	err := clash.SwitchProxy(groupName, nodeName)
	if err != nil {
		return "切换失败: " + err.Error()
	}
	return "✅ 节点已切换"
}

// GetInitialData 启动前获取基础信息和离线节点
func (a *App) GetInitialData() map[string]interface{} {
	mode, groups, err := clash.GetStaticNodes()

	if err != nil {
		// 如果 YAML 解析报错，把错误传给前端
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
		return "失败: " + err.Error()
	}
	return "✅ 模式已切换至: " + mode
}
