package clash

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"goclashz/core/utils"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

var (
	mu                sync.Mutex
	clashCmd          *exec.Cmd
	isRunning         atomic.Bool
	isIntentionalStop atomic.Bool
)

// assignProcessToJobObject 将进程加入到随主进程退出的 Job 中
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

func Start(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()

	if isRunning.Load() {
		return nil
	}

	// ✅ 程序文件路径 (只读)
	binDir := utils.GetCoreBinDir()
	exePath := filepath.Join(binDir, "clash.exe")
	
	// ✅ 运行时数据路径 (可写，自定义模式或安全模式)
	dataDir := utils.GetDataDir()
	pidFile := filepath.Join(dataDir, "clash.pid")
	runtimeConfig := filepath.Join(dataDir, "config.yaml") // 运行时最终生成的配置

	// 先清理旧 PID
	if pidData, err := os.ReadFile(pidFile); err == nil {
		pidStr := string(pidData)
		killCmd := exec.Command("taskkill", "/F", "/FI", "IMAGENAME eq clash.exe", "/PID", pidStr)
		killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		killCmd.Run()
		os.Remove(pidFile)
	}

	// 传入 config 路径
	if err := PrepareEnv(binDir, exePath, runtimeConfig); err != nil {
		return err
	}

	// 🎯 核心分离：
	// -d 设定内核的 Home 目录，它会去这里找 GeoSite.dat / mmdb (因为是只读的，放 AppDir 没问题)
	// -f 设定内核强制读取的 yaml 配置，指向我们的 DataDir
	cmd := exec.Command(exePath, "-d", binDir, "-f", runtimeConfig)
	cmd.Dir = binDir
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_BREAKAWAY_FROM_JOB,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("无法启动内核: %v", err)
	}

	assignProcessToJobObject(cmd.Process)

	clashCmd = cmd
	isRunning.Store(true)
	isIntentionalStop.Store(false)

	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644)

	go func(c *exec.Cmd) {
		c.Wait()
		mu.Lock()
		defer mu.Unlock()
		if clashCmd == c {
			isRunning.Store(false)
			clashCmd = nil
			os.Remove(pidFile)
		}
		
		if !isIntentionalStop.Load() {
			runtime.EventsEmit(ctx, "clash-exited", "内核已异常退出")
		}
	}(cmd)

	return nil
}

func Stop() error {
	mu.Lock()
	defer mu.Unlock()

	isIntentionalStop.Store(true)

	if clashCmd != nil && clashCmd.Process != nil {
		clashCmd.Process.Kill()
	}
	clashCmd = nil
	isRunning.Store(false)
	return nil
}

func IsRunning() bool {
	return isRunning.Load()
}
