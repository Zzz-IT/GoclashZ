package clash

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

	// 🚀 修复：安全地清理旧进程
	if pidData, err := os.ReadFile(pidFile); err == nil {
		pidStr := strings.TrimSpace(string(pidData))
		// 1. 严格校验：必须能转换为正整数，杜绝任何命令注入可能
		if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
			// 2. 使用绝对路径调用系统自带的 taskkill，防止环境变量劫持
			sysDir := os.Getenv("SystemRoot")
			if sysDir == "" {
				sysDir = "C:\\Windows"
			}
			taskkillPath := filepath.Join(sysDir, "System32", "taskkill.exe")
			
			// 3. /FI 校验进程名 + 严格安全的整数 PID
			killCmd := exec.Command(taskkillPath, "/F", "/FI", "IMAGENAME eq clash.exe", "/PID", strconv.Itoa(pid))
			killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			_ = killCmd.Run()
		}
		os.Remove(pidFile)
	}

	// 准备环境 (检查内核与基础配置)
	if err := PrepareEnv(); err != nil {
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
