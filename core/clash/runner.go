//go:build windows

package clash

import (
	"context"
	"errors"
	"fmt"
	"goclashz/core/logger"
	"goclashz/core/sys"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	processExitCh     chan struct{}
	onExitCallback    OnExitFunc
	startedViaHelper  atomic.Bool
)

// SetOnExitCallback 注册内核异常退出的回调（由 appcore 层在启动时设置）
func SetOnExitCallback(fn OnExitFunc) {
	mu.Lock()
	defer mu.Unlock()
	onExitCallback = fn
}

// killProcessIfClash 安全杀进程：验证 PID 对应进程名是否确为目标执行文件名，防止 PID 复用误杀
func killProcessIfClash(pid int, expectedExeName string) {
	hProcess, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(hProcess)

	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	err = windows.QueryFullProcessImageName(hProcess, 0, &buf[0], &size)
	if err == nil {
		imageName := windows.UTF16ToString(buf[:size])
		targetSuffix := "\\" + strings.ToLower(expectedExeName)
		if strings.HasSuffix(strings.ToLower(imageName), targetSuffix) {
			_ = windows.TerminateProcess(hProcess, 1)
		}
	}
}

// cleanupResidualClashProcess 清理残余内核进程
func cleanupResidualClashProcess(pidFile string, expectedExeName string) {
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}

	pidStr := strings.TrimSpace(string(pidData))
	if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
		killProcessIfClash(pid, expectedExeName)
		time.Sleep(300 * time.Millisecond)
	}

	_ = os.Remove(pidFile)
}

// Start 启动内核
// tun=true 时必须通过 helper 启动（TUN 需要服务权限）
// tun=false 时直接启动，不依赖 helper
func Start(ctx context.Context, tun bool) error {
	mu.Lock()
	defer mu.Unlock()

	if isRunning.Load() {
		return nil
	}

	binDir := utils.GetCoreBinDir()
	exePath := filepath.Join(binDir, "clash.exe")
	targetExeName := filepath.Base(exePath)

	pidFile := utils.GetPidFilePath()
	runtimeConfig := utils.GetRuntimeConfigPath()

	cleanupResidualClashProcess(pidFile, targetExeName)

	if err := PrepareEnv(ctx); err != nil {
		return err
	}

	if err := validateCoreExecutable(exePath); err != nil {
		return err
	}

	if tun {
		// TUN 模式：必须通过 helper 启动
		return startCoreViaHelper(ctx, exePath, binDir, runtimeConfig, pidFile)
	}

	// 普通模式：直接启动，不碰 helper
	return startCoreDirect(ctx, exePath, binDir, runtimeConfig, pidFile)
}

// startCoreViaHelper 通过 helper 服务启动内核（TUN 专用）
func startCoreViaHelper(_ context.Context, exePath, binDir, runtimeConfig, _ string) error {
	client := sys.NewHelperClient()

	if err := client.StartCore(sys.StartCoreParams{
		CorePath:      exePath,
		BinDir:        binDir,
		RuntimeConfig: runtimeConfig,
		Args:          []string{"-d", binDir, "-f", runtimeConfig},
	}); err != nil {
		return fmt.Errorf("通过 Helper 启动内核失败 (TUN 模式需要 Helper 服务): %w", err)
	}

	startedViaHelper.Store(true)
	isRunning.Store(true)
	isIntentionalStop.Store(false)
	logger.Infof("内核已通过 Helper 服务启动 (TUN)")
	return nil
}

// startCoreDirect 直接启动内核进程（普通代理模式）
func startCoreDirect(ctx context.Context, exePath, binDir, runtimeConfig, pidFile string) error {
	startedViaHelper.Store(false)

	cmd, err := startCoreProcessWithRetry(ctx, exePath, binDir, runtimeConfig)
	if err != nil {
		if isAccessDenied(err) {
			return fmt.Errorf(
				"无法启动内核：Windows 拒绝执行 %s。可能原因：核心位于可写 data 目录、文件仍被安全软件扫描、或权限策略阻止: %w",
				exePath, err,
			)
		}
		return fmt.Errorf("无法启动内核: %w", err)
	}

	clashCmd = cmd
	isRunning.Store(true)
	isIntentionalStop.Store(false)
	processExitCh = make(chan struct{})
	_ = os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644)

	localExitCh := processExitCh

	go func(c *exec.Cmd, ch chan struct{}) {
		c.Wait()

		mu.Lock()
		if clashCmd == c {
			isRunning.Store(false)
			clashCmd = nil
			os.Remove(pidFile)
		}
		cb := onExitCallback
		mu.Unlock()

		close(ch)

		if !isIntentionalStop.Load() && cb != nil {
			cb(ExitEvent{Intentional: false, Message: "内核已异常退出"})
		}
	}(cmd, localExitCh)

	return nil
}

func Stop() error {
	mu.Lock()
	isIntentionalStop.Store(true)

	// 通过 helper 启动的，也通过 helper 停止
	if startedViaHelper.Load() {
		mu.Unlock()
		client := sys.NewHelperClient()
		if err := client.StopCore(); err != nil {
			logger.Warnf("Helper 停止内核失败: %v", err)
		}
		startedViaHelper.Store(false)
		isRunning.Store(false)
		return nil
	}

	var exitCh chan struct{}
	var proc *os.Process
	var pid int

	if clashCmd != nil && clashCmd.Process != nil {
		proc = clashCmd.Process
		pid = clashCmd.Process.Pid
		exitCh = processExitCh
	}

	targetExeName := filepath.Base(filepath.Join(utils.GetCoreBinDir(), "clash.exe"))
	mu.Unlock()

	if proc != nil {
		if err := proc.Kill(); err != nil {
			logger.Errorf("停止内核进程失败: %v", err)
			if pid > 0 {
				killProcessIfClash(pid, targetExeName)
			}
		}
	}

	if exitCh != nil {
		select {
		case <-exitCh:
		case <-time.After(3 * time.Second):
			if pid > 0 {
				killProcessIfClash(pid, targetExeName)
			}
		}
	}

	isRunning.Store(false)
	return nil
}

func IsRunning() bool {
	return isRunning.Load()
}

func StartedViaHelper() bool {
	return startedViaHelper.Load()
}

func validateCoreExecutable(exePath string) error {
	st, err := os.Stat(exePath)
	if err != nil {
		return fmt.Errorf("内核文件不存在: %w", err)
	}

	if st.IsDir() {
		return fmt.Errorf("内核路径不是文件: %s", exePath)
	}

	if st.Size() < 5*1024*1024 {
		return fmt.Errorf("内核文件体积异常: %d bytes", st.Size())
	}

	return ValidateWindowsPE(exePath, 5*1024*1024)
}

func startCoreProcessWithRetry(ctx context.Context, exePath, binDir, runtimeConfig string) (*exec.Cmd, error) {
	var lastErr error

	for i := 0; i < 8; i++ {
		cmd := exec.CommandContext(ctx, exePath, "-d", binDir, "-f", runtimeConfig)
		cmd.Dir = binDir
		utils.HideCommandWindow(cmd, 0) // 不使用 CREATE_BREAKAWAY_FROM_JOB

		err := cmd.Start()
		if err == nil {
			return cmd, nil
		}

		lastErr = err

		if !isAccessDenied(err) {
			return nil, err
		}

		time.Sleep(time.Duration(250+i*250) * time.Millisecond)
	}

	return nil, fmt.Errorf("启动内核被系统拒绝，可能是文件仍被安全软件扫描或目录策略阻止: %w", lastErr)
}

func isAccessDenied(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, os.ErrPermission) {
		return true
	}

	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return errors.Is(pathErr.Err, windows.ERROR_ACCESS_DENIED)
	}

	return strings.Contains(strings.ToLower(err.Error()), "access is denied")
}
