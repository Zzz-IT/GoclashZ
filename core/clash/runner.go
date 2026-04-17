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

func Start(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()

	if isRunning {
		return nil // 👈 关键修改：如果已经在运行，直接返回 nil，不要报错
	}

	pwd, _ := os.Getwd()
	dirPath := filepath.Join(pwd, "core", "bin")
	exePath := filepath.Join(dirPath, "clash.exe")

	if err := PrepareEnv(dirPath, exePath); err != nil {
		return err
	}

	cmd := exec.Command(exePath, "-d", dirPath)
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
		if clashCmd == c { // 👈 只处理当前进程的退出
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
