package clash

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync" // 引入互斥锁
	"syscall"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	mu        sync.Mutex // 全局锁
	clashCmd  *exec.Cmd
	isRunning bool
)

func Start(ctx context.Context) error {
	mu.Lock()         // 加锁
	defer mu.Unlock() // 确保退出时解锁

	if isRunning {
		return fmt.Errorf("内核已经在运行中了")
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

	// 关键：将当前的 cmd 引用传入协程，避免清理掉新启动的进程
	go func(targetCmd *exec.Cmd) {
		targetCmd.Wait()

		mu.Lock()
		defer mu.Unlock()
		// 只有当全局的 clashCmd 依然是当前这个退出的进程时，才重置状态
		if clashCmd == targetCmd {
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

	fmt.Println("正在停止 Clash 内核...")

	// 无论 clashCmd 是否为 nil，既然调用了 Stop，我们就应该尝试重置状态以备恢复
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
