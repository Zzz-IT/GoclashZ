package clash

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync" // 必须引入
	"syscall"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	mu                sync.Mutex
	clashCmd          *exec.Cmd
	isRunning         bool
	isIntentionalStop bool // 👈 新增：标记是否为手动停止
)

func getExeDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exePath)
}

func Start(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()

	if isRunning {
		return nil
	}

	dirPath := filepath.Join(getExeDir(), "core", "bin")
	exePath := filepath.Join(dirPath, "clash.exe")
	pidFile := filepath.Join(dirPath, "clash.pid")

	// ⚠️ 核心修复：精准查杀上次遗留的自身进程，防止误杀其他应用
	if pidData, err := os.ReadFile(pidFile); err == nil {
		pidStr := string(pidData)
		exec.Command("taskkill", "/F", "/PID", pidStr).Run()
		os.Remove(pidFile)
	}

	if err := PrepareEnv(dirPath, exePath); err != nil {
		return err
	}

	cmd := exec.Command(exePath, "-d", dirPath)
	cmd.Dir = dirPath
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("无法启动内核: %v", err)
	}

	clashCmd = cmd
	isRunning = true
	isIntentionalStop = false // 重置标志位

	// 记录本次运行的 PID
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644)

	go func(c *exec.Cmd) {
		c.Wait()
		mu.Lock()
		defer mu.Unlock()
		if clashCmd == c {
			isRunning = false
			clashCmd = nil
			os.Remove(pidFile) // 进程结束后清理 PID 文件
		}
		
		// ⚠️ 核心修复：只有在非手动关闭的情况下，才向前端发送异常退出警告
		if !isIntentionalStop {
			runtime.EventsEmit(ctx, "clash-exited", "内核已异常退出")
		}
	}(cmd)

	return nil
}

func Stop() error {
	mu.Lock()
	defer mu.Unlock()

	isIntentionalStop = true // 👈 标记为主动停止

	if clashCmd != nil && clashCmd.Process != nil {
		clashCmd.Process.Kill()
	}
	clashCmd = nil
	isRunning = false
	return nil
}

func IsRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	return isRunning
}
