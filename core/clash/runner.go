package clash

import (
	"context" // 引入 context
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/wailsapp/wails/v2/pkg/runtime" // 引入 Wails runtime
)

var clashCmd *exec.Cmd
var isRunning bool

// Start 启动内核，需传入 ctx 以支持 Wails 事件分发
func Start(ctx context.Context) error {
	if isRunning {
		return fmt.Errorf("内核已经在运行中了")
	}

	pwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("无法获取当前目录: %v", err)
	}
	dirPath := filepath.Join(pwd, "core", "bin")
	exePath := filepath.Join(dirPath, "clash.exe")

	err = PrepareEnv(dirPath, exePath)
	if err != nil {
		return err
	}

	clashCmd = exec.Command(exePath, "-d", dirPath)
	clashCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	err = clashCmd.Start()
	if err != nil {
		return fmt.Errorf("无法启动内核: %v", err)
	}

	isRunning = true
	fmt.Println("✅ Clash 内核启动成功, PID:", clashCmd.Process.Pid)

	// 👉 新增：内核守护协程
	go func() {
		// Wait 会阻塞直到进程退出
		err := clashCmd.Wait()
		isRunning = false
		clashCmd = nil

		fmt.Printf("⚠️ 内核进程已退出, 原因: %v\n", err)

		// 主动向前端推送 "clash-exited" 事件，前端可监听此事件来重置 UI 开关状态
		runtime.EventsEmit(ctx, "clash-exited", "内核意外崩溃或已退出")
	}()

	return nil
}

// Stop 强制杀死进程
func Stop() error {
	if clashCmd != nil && clashCmd.Process != nil {
		fmt.Println("正在停止 Clash 内核...")
		err := clashCmd.Process.Kill()
		clashCmd = nil
		isRunning = false // 👉 停止成功，标记为 false
		if err != nil {
			return fmt.Errorf("关闭内核失败: %v", err)
		}
	}
	return nil
}

// IsRunning 👉 新增：提供给外部查询当前状态的方法
func IsRunning() bool {
	return isRunning
}
