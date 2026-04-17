package main

import (
	"context"
	"fmt"
	"goclashz/core/clash" // 这里的 goclashz 是你的 go.mod 中的 module 名称
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

// shutdown 确保在用户点击右上角 X 关闭软件时，内核进程也会跟着被杀死，不留后台僵尸进程
func (a *App) shutdown(ctx context.Context) {
	fmt.Println("应用正在退出，执行清理操作...")
	clash.Stop()
}

// ----------- 下面是暴露给前端的方法 -----------

// RunProxy 供前端点击“启动”时调用
func (a *App) RunProxy() string {
	err := clash.Start()
	if err != nil {
		return err.Error()
	}
	return "✅ 代理核心已启动 (默认端口一般为 7890，请检查 config.yaml)"
}

// StopProxy 供前端点击“停止”时调用
func (a *App) StopProxy() string {
	err := clash.Stop()
	if err != nil {
		return err.Error()
	}
	return "🛑 代理核心已停止"
}

// GetProxyStatus 供前端在刚打开软件时，查询当前代理是不是活着的
func (a *App) GetProxyStatus() bool {
	return clash.IsRunning()
}
