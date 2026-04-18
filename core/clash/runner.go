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
	mu        sync.Mutex
	clashCmd  *exec.Cmd
	isRunning bool
)

// 获取程序真实运行目录的辅助函数
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

	// 启动前尝试清理残留的旧内核进程
	exec.Command("taskkill", "/F", "/IM", "clash.exe").Run()

	if err := PrepareEnv(dirPath, exePath); err != nil {
		return err
	}

	cmd := exec.Command(exePath, "-d", dirPath)
	// 👇 核心修复：强制指定内核的工作目录，确保能加载 wintun.dll
	cmd.Dir = dirPath 
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("无法启动内核: %v", err)
	}

	clashCmd = cmd
	isRunning = true

	go func(c *exec.Cmd) {
		c.Wait()
		mu.Lock()
		defer mu.Unlock()
		if clashCmd == c {
			isRunning = false
			clashCmd = nil
		}
		runtime.EventsEmit(ctx, "clash-exited", "内核已退出")
	}(cmd)

	return nil
}

func Stop() error {
	mu.Lock()
	defer mu.Unlock()

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
