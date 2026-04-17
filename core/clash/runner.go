package clash

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

var clashCmd *exec.Cmd
var isRunning bool // 👉 新增一个状态标记

// Start 启动 Clash 内核进程
func Start() error {
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

	isRunning = true // 👉 启动成功，标记为 true
	fmt.Println("✅ Clash 内核启动成功, PID:", clashCmd.Process.Pid)
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
