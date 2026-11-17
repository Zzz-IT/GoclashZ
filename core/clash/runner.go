//go:build windows

package clash

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"goclashz/core/logger"
	"goclashz/core/sys"
	"io"
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

	// helper 状态 IPC 短缓存：IsRunning 在 helper 模式下每次都会拨 named pipe，
	// 热路径（如 2s ticker）上 IPC 往返不可忽略，此处缓存 1s 内的查询结果。
	helperStatusCacheMu  sync.Mutex
	helperStatusCacheAt  time.Time
	helperStatusCacheVal bool
	helperStatusCacheOK  bool

	// coreOutputTailLimit 是内核输出环形缓冲的行数上限。
	coreOutputTailLimit = 200
)

// coreOutput 保存内核 stdout/stderr 的最近若干行。
//
// F-A3-04：直连模式此前既不给内核设置 Stdout/Stderr，也不落盘，内核起不来时
// 唯一的信息源（HTTP /logs）还没监听，等于把失败原因彻底丢掉。
var coreOutput struct {
	mu    sync.Mutex
	lines []string
}

func appendCoreOutput(line string) {
	coreOutput.mu.Lock()
	coreOutput.lines = append(coreOutput.lines, line)
	if len(coreOutput.lines) > coreOutputTailLimit {
		coreOutput.lines = coreOutput.lines[len(coreOutput.lines)-coreOutputTailLimit:]
	}
	coreOutput.mu.Unlock()
}

// CoreOutputTail 返回内核最近 n 行输出（只读快照，供启动失败归因与诊断使用）。
func CoreOutputTail(n int) []string {
	coreOutput.mu.Lock()
	defer coreOutput.mu.Unlock()

	if n <= 0 || n > len(coreOutput.lines) {
		n = len(coreOutput.lines)
	}
	out := make([]string, n)
	copy(out, coreOutput.lines[len(coreOutput.lines)-n:])
	return out
}

// formatCoreOutputTail 把最近 n 行输出拼成单行摘要，便于塞进 error 或事件文案。
func formatCoreOutputTail(n int) string {
	lines := CoreOutputTail(n)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, " | ")
}

// withCoreOutputTail 给启动失败的错误附带内核输出尾巴（没有输出时原样返回）。
func withCoreOutputTail(err error) error {
	if err == nil {
		return nil
	}
	tail := formatCoreOutputTail(20)
	if tail == "" {
		return err
	}
	return fmt.Errorf("%w；最近内核输出: %s", err, tail)
}

// streamCoreOutput 逐行消费内核 stdout/stderr（io.Pipe 读端）：写日志并进 ring。
func streamCoreOutput(r io.Reader) {
	scanner := bufio.NewScanner(r)
	// 内核单行可能很长（配置/规则 dump），放宽到 1MB，避免 Scanner 直接报错退出。
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if strings.TrimSpace(line) == "" {
			continue
		}
		logger.Infof("core: %s", line)
		appendCoreOutput(line)
	}
}

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

	cmd, outWriter, err := startCoreProcessWithRetry(ctx, exePath, binDir, runtimeConfig)
	if err != nil {
		// err 已由 withCoreOutputTail 附加内核输出尾巴（若有）。
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

	go func(c *exec.Cmd, ch chan struct{}, pw *io.PipeWriter) {
		c.Wait()

		// 进程退出后关闭写端：streamCoreOutput 的读循环随之 EOF 结束。
		if pw != nil {
			_ = pw.Close()
		}

		mu.Lock()
		isCurrent := (clashCmd == c)
		if isCurrent {
			isRunning.Store(false)
			clashCmd = nil
			os.Remove(pidFile)
		}
		cb := onExitCallback
		mu.Unlock()

		close(ch)

		if isCurrent && !isIntentionalStop.Load() {
			exitCode := -1
			if c.ProcessState != nil {
				exitCode = c.ProcessState.ExitCode()
			}

			// 全量尾巴进日志（最多 20 行），简短尾巴进 UI 事件（最多 3 行/300 字符）。
			if tail := formatCoreOutputTail(20); tail != "" {
				logger.Warnf("内核异常退出(exit=%d)前输出: %s", exitCode, tail)
			}

			if cb != nil {
				msg := "内核已异常退出"
				if exitCode >= 0 {
					msg = fmt.Sprintf("内核已异常退出 (exit=%d)", exitCode)
				}
				if tail := formatCoreOutputTail(3); tail != "" {
					if r := []rune(tail); len(r) > 300 {
						tail = string(r[len(r)-300:])
					}
					msg += "；最近输出: " + tail
				}
				cb(ExitEvent{Intentional: false, Message: msg})
			}
		}
	}(cmd, localExitCh, outWriter)

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
			// 不吞错、不假装已停（F-A2-06）：helper 可能仍在托管内核，
			// 此时保留 isRunning/startedViaHelper 让上层如实处理，
			// 由 watchdog 下一轮重试停止，而不是制造"已停"的假象。
			logger.Warnf("Helper 停止内核失败（保留运行态待重试）: %v", err)
			return fmt.Errorf("helper 停止内核失败: %w", err)
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
		// clash.exe 以 CREATE_NO_WINDOW 启动，没有控制台句柄，
		// GenerateConsoleCtrlEvent 无法送达，直接 Kill 即可（< 10ms）。
		if killErr := proc.Kill(); killErr != nil {
			logger.Errorf("停止内核进程失败: %v", killErr)
			if pid > 0 {
				killProcessIfClash(pid, targetExeName)
			}
		}
	}

	if exitCh != nil {
		select {
		case <-exitCh:
		case <-time.After(500 * time.Millisecond):
			// 兜底：正常 Kill 后进程应在 100ms 内退出；超时则强制清理
			if pid > 0 {
				killProcessIfClash(pid, targetExeName)
			}
		}
	}

	isRunning.Store(false)
	return nil
}

// validatedConfig 记录最近一次通过 -t 校验的运行时配置内容指纹（sha256）。
//
// 目的（F-A0-06）：起内核路径原本每次都要 spawn 一次 clash.exe -t；
// 只要配置内容未变，就没有必要重复启动子进程。用内容哈希而不是 mtime，
// 是因为 BuildRuntimeConfig 每次都会重写文件（mtime 必变）。
var validatedConfig struct {
	mu   sync.Mutex
	hash string
	at   time.Time
}

// validatedConfigTTL 是「同一份配置可以复用校验结论」的最长时间。
const validatedConfigTTL = 10 * time.Minute

// InvalidateValidatedConfig 让下一次 ValidateConfigCached 重新 spawn 校验（排障用）。
func InvalidateValidatedConfig() {
	validatedConfig.mu.Lock()
	validatedConfig.hash = ""
	validatedConfig.mu.Unlock()
}

func configContentHash(configPath string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// ValidateConfigCached 在配置内容与上次成功校验一致（且未超 TTL）时直接放行，
// 避免每次起内核都 spawn 一次 clash.exe -t；校验失败不缓存（失败必须每次重试）。
func ValidateConfigCached(ctx context.Context, configPath string) error {
	hash, err := configContentHash(configPath)
	if err != nil {
		// 读不到配置就交给 ValidateConfig 去报准确错误
		return ValidateConfig(ctx, configPath)
	}

	validatedConfig.mu.Lock()
	hit := validatedConfig.hash == hash && time.Since(validatedConfig.at) < validatedConfigTTL
	validatedConfig.mu.Unlock()
	if hit {
		return nil
	}

	if err := ValidateConfig(ctx, configPath); err != nil {
		return err
	}

	validatedConfig.mu.Lock()
	validatedConfig.hash = hash
	validatedConfig.at = time.Now()
	validatedConfig.mu.Unlock()
	return nil
}

// ValidateConfig 使用 clash.exe -t 验证配置有效性
func ValidateConfig(ctx context.Context, configPath string) error {
	binDir := utils.GetCoreBinDir()
	exePath := filepath.Join(binDir, "clash.exe")

	cmdCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, exePath, "-t", "-d", binDir, "-f", configPath)
	cmd.Dir = binDir
	utils.HideCommandWindow(cmd, 0)

	out, err := cmd.CombinedOutput()
	if err != nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		var errLines []string
		for _, line := range lines {
			l := strings.TrimSpace(line)
			if strings.Contains(l, "level=error") || strings.Contains(l, "level=fatal") || strings.Contains(l, "failed") {
				errLines = append(errLines, l)
			}
		}
		if len(errLines) > 0 {
			return fmt.Errorf("%s", strings.Join(errLines, "; "))
		}
		if len(out) > 0 {
			return fmt.Errorf("%s", strings.TrimSpace(string(out)))
		}
		return fmt.Errorf("配置校验失败: %w", err)
	}
	return nil
}

func IsRunning() bool {
	if startedViaHelper.Load() {
		helperStatusCacheMu.Lock()
		if helperStatusCacheOK && time.Since(helperStatusCacheAt) < time.Second {
			v := helperStatusCacheVal
			helperStatusCacheMu.Unlock()
			return v
		}
		helperStatusCacheMu.Unlock()

		client := sys.NewHelperClient()
		if status, err := client.CoreStatus(); err == nil {
			isRunning.Store(status.Running)
			helperStatusCacheMu.Lock()
			helperStatusCacheVal = status.Running
			helperStatusCacheAt = time.Now()
			helperStatusCacheOK = true
			helperStatusCacheMu.Unlock()
			return status.Running
		}
		// IPC 失败时使缓存失效，回落到本地标志位
		helperStatusCacheMu.Lock()
		helperStatusCacheOK = false
		helperStatusCacheMu.Unlock()
	}
	return isRunning.Load()
}

func StartedViaHelper() bool {
	return startedViaHelper.Load()
}

// startCoreProcessWithRetry 启动内核进程（access denied 时按退避重试）。
//
// 额外返回 stdout/stderr 的写端：调用方在进程退出后必须 Close，
// 否则 streamCoreOutput 的读循环不会结束（goroutine 泄漏）。
func startCoreProcessWithRetry(ctx context.Context, exePath, binDir, runtimeConfig string) (*exec.Cmd, *io.PipeWriter, error) {
	var lastErr error

	for i := 0; i < 8; i++ {
		// ⚠️ 内核是长期驻留后台的 Daemon 进程，切不可使用 request 级的 timeout ctx (如 15s 的 ToggleTunMode ctx)！
		// 否则 RPC 函数一返回 defer cancel() 就会立刻杀掉刚拉起的 clash.exe 进程！
		cmd := exec.Command(exePath, "-d", binDir, "-f", runtimeConfig)
		cmd.Dir = binDir
		utils.HideCommandWindow(cmd, windows.CREATE_NEW_PROCESS_GROUP)

		// 直连模式也要能看到内核输出（F-A3-04）：接 io.Pipe，由 streamCoreOutput
		// 逐行进 logger 与 ring 缓冲；启动失败时把尾巴拼进错误。
		pr, pw := io.Pipe()
		cmd.Stdout = pw
		cmd.Stderr = pw
		go streamCoreOutput(pr)

		err := cmd.Start()
		if err == nil {
			return cmd, pw, nil
		}

		_ = pw.Close() // 结束本次尝试的读循环
		lastErr = err

		if !isAccessDenied(err) {
			return nil, nil, withCoreOutputTail(err)
		}

		time.Sleep(time.Duration(250+i*250) * time.Millisecond)
	}

	return nil, nil, withCoreOutputTail(fmt.Errorf("启动内核被系统拒绝，可能是文件仍被安全软件扫描或目录策略阻止: %w", lastErr))
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
