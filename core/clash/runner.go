//go:build windows

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

	"golang.org/x/sys/windows"
)

// ExitEvent 描述内核退出事件
type ExitEvent struct {
	Intentional bool
	Message     string
}

// OnExitFunc 是内核异常退出时的回调函数类型
type OnExitFunc func(event ExitEvent)

var (
	mu                sync.Mutex
	clashCmd          *exec.Cmd
	isRunning         atomic.Bool
	isIntentionalStop atomic.Bool
	processExitCh     chan struct{} // 👈 新增：进程退出信号通道
	globalJobHandle   windows.Handle // 🚀 新增：用来维持 Job 句柄的生命周期，防止内核被系统秒杀
	onExitCallback    OnExitFunc     // 🚀 新增：退出回调，替代直接引用 Wails
)

// SetOnExitCallback 注册内核异常退出的回调（由 appcore 层在启动时设置）
func SetOnExitCallback(fn OnExitFunc) {
	mu.Lock()
	defer mu.Unlock()
	onExitCallback = fn
}

// assignProcessToJobObject 将进程加入到随主进程退出的 Job 中
func assignProcessToJobObject(proc *os.Process) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	
	// 🛑 修改前：绝对不能在这里 defer CloseHandle！这会导致内核刚启动就被系统杀掉
	// defer windows.CloseHandle(job)

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
	
	err = windows.AssignProcessToJobObject(job, windows.Handle(proc.Pid))
	if err == nil {
		// 🚀 核心修复：释放上一个已死亡进程留下的历史句柄，彻底根治 Handle Leak 隐患
		if globalJobHandle != 0 {
			windows.CloseHandle(globalJobHandle)
		}
		globalJobHandle = job
	}
	return err
}

// killProcessIfClash 安全杀进程：验证 PID 对应进程名是否确为目标执行文件名，防止 PID 复用误杀
func killProcessIfClash(pid int, expectedExeName string) {
	// 请求查询进程信息和终止权限
	hProcess, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return // 进程已不存在或无权限打开
	}
	defer windows.CloseHandle(hProcess)

	// 获取进程的完整镜像路径
	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	err = windows.QueryFullProcessImageName(hProcess, 0, &buf[0], &size)
	if err == nil {
		imageName := windows.UTF16ToString(buf[:size])
		// 👈 动态比对，并且统一转小写防止大小写不一致导致失效
		// 强制拼接上一个反斜杠 `\`，确保我们匹配的是完整文件名而非名字的后缀 (防止 PID 复用误杀)
		targetSuffix := "\\" + strings.ToLower(expectedExeName)
		if strings.HasSuffix(strings.ToLower(imageName), targetSuffix) {
			_ = windows.TerminateProcess(hProcess, 1)
		}
	}
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
	targetExeName := filepath.Base(exePath) // 👈 动态提取出 "clash.exe" 或未来更改的名字

	// ✅ 运行时数据路径 (可写，自定义模式或安全模式)
	dataDir := utils.GetDataDir()
	pidFile := filepath.Join(dataDir, "clash.pid")
	runtimeConfig := filepath.Join(dataDir, "config.yaml") // 运行时最终生成的配置

	// 🚀 修复：使用 PID 校验机制清理旧进程，防止误杀
	if pidData, err := os.ReadFile(pidFile); err == nil {
		pidStr := strings.TrimSpace(string(pidData))
		if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
			killProcessIfClash(pid, targetExeName) // 👈 传入动态名字进行校验
		}
		os.Remove(pidFile)
	}

	// 准备环境 (检查内核与基础配置)
	if err := PrepareEnv(ctx); err != nil {
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
		// 捕获回调的引用
		cb := onExitCallback
		mu.Unlock() // 🚀 关键：必须先释放锁，再发送信号
		
		// 🎯 修复：关闭的是与此进程绑定的局部 channel，而不是全局 channel
		close(ch) // 👈 发送进程彻底退出的广播信号

		if !isIntentionalStop.Load() && cb != nil {
			cb(ExitEvent{Intentional: false, Message: "内核已异常退出"})
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
