package clash

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	mu                sync.Mutex
	clashCmd          *exec.Cmd
	isRunning         bool
	isIntentionalStop bool // 👈 新增：标记是否为手动停止
)

// ⚠️ 修复：新增一个辅助函数，用于将进程加入到随主进程退出的 Job 中
func assignProcessToJobObject(proc *os.Process) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		return err
	}
	return windows.AssignProcessToJobObject(job, windows.Handle(proc.Pid))
}

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

	// ⚠️ 修复：加入 /FI 过滤器精准匹配进程名称，防止 PID 欺骗与误杀
	if pidData, err := os.ReadFile(pidFile); err == nil {
		pidStr := string(pidData)
		killCmd := exec.Command("taskkill", "/F", "/FI", "IMAGENAME eq clash.exe", "/PID", pidStr)
		// 👇 将杀进程操作独立出来，并添加隐藏窗口属性
		killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		killCmd.Run()
		os.Remove(pidFile)
	}

	if err := PrepareEnv(dirPath, exePath); err != nil {
		return err
	}

	cmd := exec.Command(exePath, "-d", dirPath)
	cmd.Dir = dirPath
	// ⚠️ 修复：必须设置 CREATE_BREAKAWAY_FROM_JOB 以允许加入新的 Job，并隐藏窗口
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_BREAKAWAY_FROM_JOB,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("无法启动内核: %v", err)
	}

	// ⚠️ 修复：将刚启动的子进程绑定到 Job Object，防止主进程崩溃导致子进程残留
	assignProcessToJobObject(cmd.Process)

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
