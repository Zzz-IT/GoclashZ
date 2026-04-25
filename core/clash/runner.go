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
	"time"
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
	processExitCh     chan struct{} // 👈 新增：进程退出信号通道
)

// assignProcessToJobObject 将进程加入到随主进程退出的 Job 中
func assignProcessToJobObject(proc *os.Process) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	
	// 🎯 修复：确保函数退出时释放主进程对 Job 对象的句柄引用
	// 注意：只要有进程还在 Job 内，Job 内核对象就不会被真正销毁，这是极其安全的做法
	defer windows.CloseHandle(job)

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

	// 🚀 修复：使用 Native API 安全地清理旧进程
	if pidData, err := os.ReadFile(pidFile); err == nil {
		pidStr := strings.TrimSpace(string(pidData))
		if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
			// 纯 Go 底层调用，无视环境变量劫持与部分杀软对 cmd/taskkill 的拦截
			hProcess, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
			if err == nil {
				_ = windows.TerminateProcess(hProcess, 1)
				windows.CloseHandle(hProcess)
			}
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
	processExitCh = make(chan struct{}) // 👈 启动时初始化通道
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644)

	// 在启动协程前，将当前的 channel 引用保存为局部变量
	localExitCh := processExitCh 

	go func(c *exec.Cmd, ch chan struct{}) {
		c.Wait()
		
		mu.Lock()
		if clashCmd == c {
			isRunning.Store(false)
			clashCmd = nil
			os.Remove(pidFile)
		}
		mu.Unlock() // 🚀 关键：必须先释放锁，再发送信号
		
		// 🎯 修复：关闭的是与此进程绑定的局部 channel，而不是全局 channel
		close(ch) // 👈 发送进程彻底退出的广播信号

		if !isIntentionalStop.Load() {
			runtime.EventsEmit(ctx, "clash-exited", "内核已异常退出")
		}
	}(cmd, localExitCh) // 👈 闭包传参

	return nil
}

func Stop() error {
	mu.Lock()
	isIntentionalStop.Store(true)
	
	var exitCh chan struct{}
	if clashCmd != nil && clashCmd.Process != nil {
		clashCmd.Process.Kill()
		exitCh = processExitCh // 👈 获取当前通道引用
	}
	mu.Unlock() // 🚀 关键：立刻释放锁，防止下面的 Wait 卡死协程

	// 👈 阻塞等待，直到操作系统真正完成进程清理和网络端口释放
	if exitCh != nil {
		// 🎯 修复：加入超时兜底机制（例如 3 秒）
		// 如果底层的 clash.exe 卡在系统驱动层无法被强杀，主界面也不至于彻底锁死
		select {
		case <-exitCh:
			// 正常退出，通道已关闭
		case <-time.After(3 * time.Second):
			// 进程顽固残留，超时放弃阻塞
		}
	}
	
	isRunning.Store(false)
	return nil
}

func IsRunning() bool {
	return isRunning.Load()
}
