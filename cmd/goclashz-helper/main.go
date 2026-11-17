//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	jsonv2 "encoding/json/v2"
	"errors"

	"fmt"
	jsonutil "goclashz/core/jsonutil"
	"goclashz/core/sys"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName  = "GoclashZHelper"
	pipeName     = `\\.\pipe\GoclashZ.Helper`
	registryPath = `SYSTEM\CurrentControlSet\Services\GoclashZHelper`
)

type helperService struct {
	mu        sync.Mutex
	core      *coreProc // 当前托管的内核进程；nil 表示未运行
	parentPID uint32    // PID of the owning main process; watched for abnormal exit
	ln        net.Listener
	cancel    context.CancelFunc
	console   consoleState // 控制台分配状态（TUN 内核优雅停止依赖它）

	// 最近一次内核退出信息（F-A3-04）：进程对象被清理后仍可通过 core-status 查询，
	// 用于回答"内核为什么没了"——退出码 + 退出时间，配合日志文件定位。
	lastExitPID  int
	lastExitCode int
	lastExitAt   time.Time
}

const (
	// coreStopShutdownGrace 是服务退出 / 父进程消失时停止内核的优雅等待预算。
	// 比 stop-core 的默认 20s 短，避免 SCM 停止服务超时。
	coreStopShutdownGrace = 5 * time.Second
)

// ---- 控制台 ----
//
// TUN 内核的优雅停止依赖 Windows 控制台事件：helper 必须拥有控制台，内核继承该控制台
// 并以 CREATE_NEW_PROCESS_GROUP 启动，helper 才能用
// GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, corePID) 定点送达。
// 已实测：session 0 下的服务（Service-0x0-3e7$）可以 AllocConsole 成功；
// 若内核被 CREATE_NO_WINDOW 起成私有控制台，事件永远不可达。

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procAllocConsole          = kernel32.NewProc("AllocConsole")
	procFreeConsole           = kernel32.NewProc("FreeConsole")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
)

// consoleState 记录 helper 进程的控制台分配状态。
type consoleState struct {
	mu        sync.Mutex
	allocated bool   // 本进程是否已成功 AllocConsole
	errMsg    string // 最近一次分配 / 校验失败原因，通过 core-status 暴露
}

// ensureConsole 保证 helper 进程拥有可用的控制台。
//
// 每个 helper 进程只 AllocConsole 一次；此后每次启动内核前用 GetConsoleProcessList 校验，
// 控制台失效（返回 0，例如被外部释放）时先 FreeConsole 再重建。失败原因记录到 consoleState。
func (s *helperService) ensureConsole() error {
	s.console.mu.Lock()
	defer s.console.mu.Unlock()

	// 控制台仍然有效（本进程之前分配过，或 debug 模式从终端继承）：无需重建
	if consoleProcessCount() > 0 {
		s.console.allocated = true
		s.console.errMsg = ""
		return nil
	}

	// 只有「本进程分配过的控制台已失效」时才需要先 FreeConsole 再重建
	if s.console.allocated {
		_, _, _ = procFreeConsole.Call()
		s.console.allocated = false
	}

	if err := allocConsole(); err != nil {
		s.console.errMsg = err.Error()
		fmt.Fprintf(os.Stderr, "helper: 分配控制台失败，TUN 内核将无法优雅停止（会降级为强制结束）: %v\n", err)
		return err
	}

	s.console.allocated = true
	s.console.errMsg = ""
	return nil
}

// consoleError 返回最近一次控制台分配失败原因（空表示正常）。
func (s *helperService) consoleError() string {
	s.console.mu.Lock()
	defer s.console.mu.Unlock()
	return s.console.errMsg
}

// consoleProcessCount 返回附加在当前控制台上的进程数量；0 表示本进程没有控制台。
func consoleProcessCount() int {
	buf := make([]uint32, 16)
	n, _, _ := procGetConsoleProcessList.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	return int(n)
}

// allocConsole 为当前进程分配一个控制台。
func allocConsole() error {
	r, _, lastErr := procAllocConsole.Call()
	if r != 0 {
		return nil
	}
	if lastErr == nil || errors.Is(lastErr, windows.ERROR_SUCCESS) {
		return errors.New("AllocConsole returned FALSE (GetLastError=0)")
	}
	return fmt.Errorf("AllocConsole 失败: %w", lastErr)
}

// sendCtrlBreakToCore 向内核进程组发送 CTRL_BREAK_EVENT。
// 前提：内核以 CREATE_NEW_PROCESS_GROUP 启动（进程组 ID = 内核 PID），
// 且与 helper 共享同一个控制台（见 ensureConsole / startCoreProcess）。
func sendCtrlBreakToCore(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid core pid: %d", pid)
	}
	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(pid)); err != nil {
		return fmt.Errorf("GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, %d) 失败: %w", pid, err)
	}
	return nil
}

// ---- 内核进程 ----

// coreProcessState 表示 helper 托管内核进程的状态。
type coreProcessState int

const (
	coreStateStopped coreProcessState = iota
	coreStateRunning
	coreStateStopping
)

func (s coreProcessState) String() string {
	switch s {
	case coreStateRunning:
		return "running"
	case coreStateStopping:
		return "stopping"
	default:
		return "stopped"
	}
}

// coreProc 持有 helper 托管的单个内核进程。
//
// cmd.Wait 只允许由 superviseCore 中的唯一 goroutine 调用一次；其它路径一律等待 done 关闭，
// 避免重复 Wait 造成 "Wait was already called" 之类的竞态。
type coreProc struct {
	cmd   *exec.Cmd
	pid   int
	done  chan struct{} // cmd.Wait 返回后关闭
	state coreProcessState

	waitOnce sync.Once

	// 内核 stdout/stderr 落盘句柄（<dataDir>/logs/core-helper-YYYYMMDD.log）；
	// 进程退出后由 superviseCore 关闭。
	logFile *os.File

	// 进程退出后记录（在 s.mu 下写、core-status 在 s.mu 下读）。
	exitCode int
	exitAt   time.Time
}

// coreSnapshot 返回当前内核进程快照（在进程退出前该 coreProc 一直有效）。
func (s *helperService) coreSnapshot() *coreProc {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.core
}

// clearCore 在确认进程已退出后清理句柄引用；未确认退出时绝不调用。
func (s *helperService) clearCore(p *coreProc) {
	s.mu.Lock()
	if s.core == p {
		p.state = coreStateStopped
		s.core = nil
	}
	s.mu.Unlock()
}

// superviseCore 启动唯一的 cmd.Wait 调用者。Wait 返回即表示进程已经退出：
// 关闭 done，并在 s.core 仍指向该进程时清理引用与状态。
func (s *helperService) superviseCore(p *coreProc) {
	p.waitOnce.Do(func() {
		go func() {
			_ = p.cmd.Wait()

			exitCode := -1
			if p.cmd.ProcessState != nil {
				exitCode = p.cmd.ProcessState.ExitCode()
			}
			exitAt := time.Now()

			close(p.done)

			// 退出码/时间先落盘再清引用：这样即使进程对象被回收，证据仍在日志里，
			// 也能通过 core-status 的 lastExit* 字段被主进程读到（F-A3-04）。
			if p.logFile != nil {
				fmt.Fprintf(p.logFile, "===== core exit at %s pid=%d code=%d =====\n",
					exitAt.Format(time.RFC3339Nano), p.pid, exitCode)
				_ = p.logFile.Close()
				p.logFile = nil
			}

			s.mu.Lock()
			p.exitCode = exitCode
			p.exitAt = exitAt
			s.lastExitPID = p.pid
			s.lastExitCode = exitCode
			s.lastExitAt = exitAt
			if s.core == p {
				p.state = coreStateStopped
				s.core = nil
			}
			s.mu.Unlock()
		}()
	})
}

// ---- 文件操作白名单（安全加固）----
//
// 背景（F-A2-03）：真正的运行时数据目录由主进程 utils.GetCoreBinDir() 决定
// （= <dataDir>/core/bin，dataDir 可能是 %LOCALAPPDATA%\GoclashZ、安装档案指定目录或
// <appDir>/data），而 helper 以 SYSTEM 身份运行，既拿不到交互用户的环境变量，也无法
// 用 os.UserCacheDir() 还原（那会指向系统账户）。因此必须从主进程写入的安装档案与
// 服务登记的 AllowedSids 反推真实目录，否则白名单永远对不上（历史表现为
// "target must be core binary"，内核替换/驱动安装永久失败）。

const (
	coreBinaryName   = "clash.exe"
	wintunBinaryName = "wintun.dll"

	// 以下区间与主进程 core/clash 的校验保持一致（setup.go:224、wintun.go:55）。
	coreExeMinSize = 5 << 20   // 5 MiB
	coreExeMaxSize = 300 << 20 // 300 MiB
	wintunMinSize  = 32 << 10  // 32 KiB
	wintunMaxSize  = 5 << 20   // 5 MiB
)

// installProfile 是主进程写入 <appDir>/install-profile.json 的安装档案
// （对应 core/utils/paths.go 的 InstallProfile，这里只解析需要的字段）。
type installProfile struct {
	Mode    string `json:"mode"`
	AppDir  string `json:"appDir"`
	DataDir string `json:"dataDir"`
}

// pathWhitelist 汇总 helper 允许访问的目录。
type pathWhitelist struct {
	AppDir      string
	DataDir     string
	StagingDir  string
	CoreBinDirs []string
}

// readInstallProfile 读取 <appDir>/install-profile.json（不存在/损坏时返回 false）。
func readInstallProfile(appDir string) (installProfile, bool) {
	data, err := os.ReadFile(filepath.Join(appDir, "install-profile.json"))
	if err != nil || len(data) == 0 {
		return installProfile{}, false
	}

	var p installProfile
	if err := jsonv2.Unmarshal(data, &p); err != nil {
		return installProfile{}, false
	}
	if strings.TrimSpace(p.DataDir) == "" {
		return installProfile{}, false
	}
	return p, true
}

// existingDir 校验候选目录：不能残留 %VAR% 占位符，且必须已存在。
func existingDir(dir string) (string, bool) {
	dir = strings.TrimSpace(dir)
	if dir == "" || strings.Contains(dir, "%") {
		return "", false
	}

	cleaned := filepath.Clean(dir)
	info, err := os.Stat(cleaned)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return cleaned, true
}

// dataDirFromInstallProfile 用安装档案解析 dataDir。
// standard：dataDir 可能带 %VAR%，需 ExpandEnv；self-contained/portable：直接用（相对路径按 appDir 展开）。
func dataDirFromInstallProfile(appDir string) (string, bool) {
	p, ok := readInstallProfile(appDir)
	if !ok {
		return "", false
	}

	candidate := strings.TrimSpace(p.DataDir)
	switch strings.ToLower(strings.TrimSpace(p.Mode)) {
	case "self-contained", "portable":
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(appDir, candidate)
		}
	case "standard":
		candidate = os.ExpandEnv(candidate)
	default:
		// 未知 mode：仍尝试展开，避免目录字面量带 %VAR%
		candidate = os.ExpandEnv(candidate)
	}

	return existingDir(candidate)
}

// dataDirFromInstallerUser 用服务登记的 AllowedSids 反推安装者的 %LOCALAPPDATA%\GoclashZ。
//
// 标准安装（不写 install-profile.json 的旧版本/开发版）里主进程的数据目录就是
// UserCacheDir()/GoclashZ；helper 在 SYSTEM 账户下算不出来，只能借注册表
// ProfileList\<SID>\ProfileImagePath 还原。SID 来自 HKLM 服务键（需管理员写入），
// 攻击者不能任意篡改。
func dataDirFromInstallerUser() (string, bool) {
	for _, sid := range readAllowedSids() {
		sid = strings.TrimSpace(sid)
		if !isSIDString(sid) {
			continue
		}

		key, err := registry.OpenKey(registry.LOCAL_MACHINE,
			`SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\`+sid, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		profile, _, err := key.GetStringValue("ProfileImagePath")
		key.Close()
		if err != nil || strings.TrimSpace(profile) == "" {
			continue
		}

		candidate := filepath.Join(os.ExpandEnv(profile), "AppData", "Local", "GoclashZ")
		if dir, ok := existingDir(candidate); ok {
			return dir, true
		}
	}

	return "", false
}

// resolveDataDir 解析 helper 视角的真实数据目录。
// 顺序：GOCLASHZ_DATA_DIR（显式覆盖）→ install-profile.json → <appDir>/.portable 标记
// → 安装者 %LOCALAPPDATA%\GoclashZ → <appDir>/data（兜底，不要求存在）。
func resolveDataDir(appDir string) string {
	if dir, ok := existingDir(os.Getenv("GOCLASHZ_DATA_DIR")); ok {
		return dir
	}

	if dir, ok := dataDirFromInstallProfile(appDir); ok {
		return dir
	}

	if _, err := os.Stat(filepath.Join(appDir, ".portable")); err == nil {
		if dir, ok := existingDir(filepath.Join(appDir, "data")); ok {
			return dir
		}
	}

	if dir, ok := dataDirFromInstallerUser(); ok {
		return dir
	}

	if dir, ok := existingDir(filepath.Join(appDir, "data")); ok {
		return dir
	}

	return filepath.Join(appDir, "data")
}

// coreBinDirCandidates 返回允许的内核目录候选（按优先级）：
// <dataDir>/core/bin，以及存在时的 <appDir>/core/bin。
func coreBinDirCandidates(appDir, dataDir string) []string {
	candidates := []string{filepath.Join(dataDir, "core", "bin")}

	alt := filepath.Join(appDir, "core", "bin")
	if !strings.EqualFold(alt, candidates[0]) {
		if info, err := os.Stat(alt); err == nil && info.IsDir() {
			candidates = append(candidates, alt)
		}
	}

	return candidates
}

// loadPathWhitelist 计算当前的白名单快照。
func loadPathWhitelist() pathWhitelist {
	exe, _ := os.Executable()
	appDir := filepath.Dir(exe)
	dataDir := resolveDataDir(appDir)

	return pathWhitelist{
		AppDir:      appDir,
		DataDir:     dataDir,
		StagingDir:  filepath.Join(dataDir, "staging"),
		CoreBinDirs: coreBinDirCandidates(appDir, dataDir),
	}
}

// getPathWhitelist 保留历史签名（主内核目录 / 暂存目录 / 数据目录）的内部调用兼容。
func getPathWhitelist() (coreBinDir, stagingDir, dataDir string) {
	wl := loadPathWhitelist()
	return wl.CoreBinDirs[0], wl.StagingDir, wl.DataDir
}

// matchCoreBinary 判断 path 是否等于某个白名单目录下的指定文件名（Clean 后大小写不敏感）。
// 命中时返回该目录，便于调用方继续校验 binDir。
func (w pathWhitelist) matchCoreBinary(path, fileName string) (string, bool) {
	cleaned := filepath.Clean(path)
	for _, dir := range w.CoreBinDirs {
		if strings.EqualFold(cleaned, filepath.Join(dir, fileName)) {
			return dir, true
		}
	}
	return "", false
}

// binaryPaths 返回所有白名单目录下指定文件名的绝对路径（用于错误信息与诊断）。
func (w pathWhitelist) binaryPaths(fileName string) []string {
	out := make([]string, 0, len(w.CoreBinDirs))
	for _, dir := range w.CoreBinDirs {
		out = append(out, filepath.Join(dir, fileName))
	}
	return out
}

// coreBinaryPaths 返回所有允许的内核文件绝对路径。
func (w pathWhitelist) coreBinaryPaths() []string {
	return w.binaryPaths(coreBinaryName)
}

// allowStagedSource 校验"待替换的新文件"来源：
//  1. 主进程约定的 <dataDir>/staging 目录下；
//  2. 或与目标同目录的 <name>.new 暂存文件 —— core/clash 的 setup.go:189 与 wintun.go:29
//     就是按 <coreBin>/clash.exe.new、<coreBin>/wintun.dll.new 暂存的；旧实现只认
//     <dataDir>/staging，导致真实调用方必被拒绝。
func (w pathWhitelist) allowStagedSource(source, fileName string) bool {
	if isUnderPath(source, w.StagingDir) {
		return true
	}

	if !strings.EqualFold(filepath.Base(source), fileName+".new") {
		return false
	}

	for _, dir := range w.CoreBinDirs {
		if strings.EqualFold(filepath.Dir(source), dir) {
			return true
		}
	}

	return false
}

// validatePEFile 校验 PE 头与体积区间（与主进程 core/clash 的校验口径一致）。
func validatePEFile(path string, minSize, maxSize int64) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory: %s", path)
	}
	if info.Size() < minSize || info.Size() > maxSize {
		return fmt.Errorf("size %d out of range [%d, %d]: %s", info.Size(), minSize, maxSize, path)
	}
	if !isValidPE(path) {
		return fmt.Errorf("not a valid PE: %s", path)
	}
	return nil
}

// equalArgs 比较两份命令行参数（顺序敏感）。
func equalArgs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isUnderPath(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func isValidPE(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	header := make([]byte, 2)
	if _, err := io.ReadFull(f, header); err != nil {
		return false
	}
	return header[0] == 'M' && header[1] == 'Z'
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "install":
			allowedSid := ""
			for i, arg := range os.Args {
				if arg == "--allowed-sid" && i+1 < len(os.Args) {
					allowedSid = os.Args[i+1]
					break
				}
			}
			installService(allowedSid)
			return
		case "uninstall":
			uninstallService()
			return
		case "debug":
			runDebug()
			return
		}
	}

	isWindowsService, err := svc.IsWindowsService()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to determine if running as service: %v\n", err)
		os.Exit(1)
	}

	if isWindowsService {
		err = svc.Run(serviceName, &helperService{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "service run failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	runDebug()
}

func (s *helperService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	changes <- svc.Status{State: svc.StartPending}

	ln, err := createPipeListener(pipeName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen pipe failed: %v\n", err)
		changes <- svc.Status{State: svc.StopPending}
		return false, 1
	}
	s.ln = ln

	// ctx/cancel lets watchParent trigger a clean shutdown when the owning
	// process disappears without sending an explicit "shutdown" command.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()

	changes <- svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptShutdown,
	}

	go s.serve(ln)
	go s.watchParent(ctx, cancel)

loop:
	for {
		select {
		case req, ok := <-r:
			if !ok {
				break loop
			}
			switch req.Cmd {
			case svc.Stop, svc.Shutdown:
				break loop
			}
		case <-ctx.Done():
			// watchParent cancelled the context — parent process is gone.
			break loop
		}
	}

	changes <- svc.Status{State: svc.StopPending}
	ln.Close()
	s.stopCore()
	return false, 0
}

// watchParent polls the registered parent-process PID every checkInterval.
// If the process has been gone for at least graceTimeout it calls cancel(),
// which causes Execute to exit and the Windows Service Manager to mark the
// service as stopped.
func (s *helperService) watchParent(ctx context.Context, cancel context.CancelFunc) {
	const (
		checkInterval = 2 * time.Second
		graceTimeout  = 8 * time.Second
	)

	var deadSince time.Time
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		s.mu.Lock()
		pid := s.parentPID
		s.mu.Unlock()

		if pid == 0 {
			// No parent registered yet — nothing to watch.
			deadSince = time.Time{}
			continue
		}

		if isProcessAlive(pid) {
			deadSince = time.Time{}
			continue
		}

		// Parent appears to be gone.
		if deadSince.IsZero() {
			deadSince = time.Now()
			continue
		}
		if time.Since(deadSince) >= graceTimeout {
			// 主进程已退出/异常终止：停止代理内核并触发服务退出
			s.stopCore()
			cancel()
			go func() {
				time.Sleep(1 * time.Second)
				os.Exit(0)
			}()
			return
		}
	}
}

// isProcessAlive returns true if the process with the given PID is still
// running. It uses PROCESS_QUERY_LIMITED_INFORMATION (0x1000) so that it
// works even when the target runs at a higher integrity level.
func isProcessAlive(pid uint32) bool {
	const stillActive = 259 // Win32 STILL_ACTIVE / STATUS_PENDING
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

func createPipeListener(pipeName string) (net.Listener, error) {
	// 读取授权的用户 SID
	allowedSids := readAllowedSids()
	sddl := buildPipeSDDL(allowedSids)

	cfg := &winio.PipeConfig{
		SecurityDescriptor: sddl,
		InputBufferSize:    4096,
		OutputBufferSize:   4096,
	}

	ln, err := winio.ListenPipe(pipeName, cfg)
	if err != nil {
		return nil, fmt.Errorf("listen pipe %s failed: %w", pipeName, err)
	}

	return ln, nil
}

func (s *helperService) serve(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *helperService) handleConn(conn net.Conn) {
	defer conn.Close()

	// 解码阶段：短超时，避免慢连接长期占用 goroutine
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	decoder := jsonutil.NewDecoder(conn)
	var req struct {
		Method string              `json:"method"`
		Params jsonutil.RawMessage `json:"params,omitempty"`
	}
	if err := decoder.Decode(&req); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("decode failed: %v", err))
		return
	}

	// 处理阶段：按方法放宽超时（stop-core 需要覆盖优雅停止 + 强杀等待的完整预算），
	// 避免 helper 还在优雅停止时连接先因 deadline 断开。
	_ = conn.SetDeadline(time.Now().Add(handlerTimeout(req.Method)))

	switch req.Method {
	case "ping":
		s.writeResponse(conn, true, nil, "")
	case "shutdown":
		s.writeResponse(conn, true, nil, "")
		// 延迟退出，让响应先发出去
		go func() {
			time.Sleep(200 * time.Millisecond)
			s.mu.Lock()
			cancel := s.cancel
			s.mu.Unlock()
			if cancel != nil {
				cancel()
				// Execute 会走正常退出路径；这里再兜底停一次内核（幂等），
				// 避免 os.Exit 提前结束进程把 TUN 内核留成孤儿进程。
				s.stopCore()
				os.Exit(0)
			} else {
				if s.ln != nil {
					s.ln.Close()
				}
				s.stopCore()
				os.Exit(0)
			}
		}()
	case "register-parent":
		s.handleRegisterParent(conn, req.Params)
	case "start-core":
		s.handleStartCore(conn, req.Params)
	case "stop-core":
		s.handleStopCore(conn, req.Params)
	case "core-status":
		s.handleCoreStatus(conn)
	case "repair-permission":
		s.handleRepairPermission(conn, req.Params)
	case "replace-core-file":
		s.handleReplaceCoreFile(conn, req.Params)
	case "install-wintun":
		s.handleInstallWintun(conn, req.Params)
	default:
		s.writeResponse(conn, false, nil, fmt.Sprintf("unknown method: %s", req.Method))
	}
}

// handlerTimeout 返回某个方法处理阶段的连接超时，必须不小于客户端 RPC 超时。
func handlerTimeout(method string) time.Duration {
	switch method {
	case "stop-core":
		// 优雅停止预算 + 强杀等待 + 余量
		return sys.HelperStopCoreRPCTimeout() + 5*time.Second
	case "start-core":
		// 清理旧内核（Kill + 等待退出 ≤5s）+ 启动新内核；
		// 与客户端 startCoreRPCTimeout 对齐，避免客户端先放弃（F-A2-11）。
		return 25 * time.Second
	case "replace-core-file", "install-wintun":
		// 备份 + 复制 61MB + rename；install-wintun 还要先优雅停止内核（F-A5-12）。
		return 65 * time.Second
	case "repair-permission":
		// icacls /T 需要遍历整个数据目录。
		return 35 * time.Second
	default:
		return 10 * time.Second
	}
}

func (s *helperService) handleStartCore(conn net.Conn, params jsonutil.RawMessage) {
	var p struct {
		CorePath      string   `json:"corePath"`
		BinDir        string   `json:"binDir"`
		RuntimeConfig string   `json:"runtimeConfig"`
		Args          []string `json:"args"`
	}
	if err := jsonv2.Unmarshal(params, &p); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("invalid params: %v", err))
		return
	}

	if !filepath.IsAbs(p.CorePath) || !filepath.IsAbs(p.BinDir) || !filepath.IsAbs(p.RuntimeConfig) {
		s.writeResponse(conn, false, nil, "absolute paths required (corePath/binDir/runtimeConfig)")
		return
	}

	// 安全加固（F-A2-04）：start-core 等价于「以 SYSTEM 身份执行一个 exe」，
	// 因此只允许白名单内核目录里的 clash.exe，且 binDir 必须是同一个目录。
	wl := loadPathWhitelist()
	corePath := filepath.Clean(p.CorePath)
	expectedBinDir, ok := wl.matchCoreBinary(corePath, coreBinaryName)
	if !ok {
		s.writeResponse(conn, false, nil, fmt.Sprintf(
			"core path not allowed: %q (allowed: %s)", corePath, strings.Join(wl.coreBinaryPaths(), ", ")))
		return
	}
	if !strings.EqualFold(filepath.Clean(p.BinDir), expectedBinDir) {
		s.writeResponse(conn, false, nil, fmt.Sprintf(
			"bin dir mismatch: got %q want %q", filepath.Clean(p.BinDir), expectedBinDir))
		return
	}

	runtimeConfig := filepath.Clean(p.RuntimeConfig)
	if !isUnderPath(runtimeConfig, wl.DataDir) {
		// 不直接拒绝：不同安装布局下 runtime 目录可能不在 dataDir 下，但必须留痕。
		fmt.Fprintf(os.Stderr, "helper: WARNING start-core 的 runtimeConfig 不在 dataDir 内: %q (dataDir=%q)\n", runtimeConfig, wl.DataDir)
	}

	if err := validatePEFile(corePath, coreExeMinSize, coreExeMaxSize); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("core binary rejected: %v", err))
		return
	}

	// 命令行由 helper 统一构造：corePath/binDir/runtimeConfig 均已校验，
	// 禁止客户端通过 Args 追加额外参数（例如改写 -d 指向任意配置目录）。
	argv := []string{"-d", expectedBinDir, "-f", runtimeConfig}
	if len(p.Args) > 0 && !equalArgs(p.Args, argv) {
		fmt.Fprintf(os.Stderr, "helper: start-core 收到非规范参数，已按白名单参数启动: %v\n", p.Args)
	}

	pid, err := s.startCoreProcess(corePath, expectedBinDir, runtimeConfig, argv)
	if err != nil {
		s.writeResponse(conn, false, nil, err.Error())
		return
	}

	// 返回新内核 PID（旧客户端会忽略该字段，保持协议兼容）
	data, _ := jsonv2.Marshal(map[string]int{"pid": pid})
	s.writeResponse(conn, true, jsonutil.RawMessage(data), "")
}

// coreOutputLogPath 返回当期内核输出日志路径：<dataDir>/logs/core-helper-YYYYMMDD.log。
// dataDir 与文件白名单共用同一套解析（install-profile / AllowedSids 回推），
// 保证 helper 写日志的位置与主进程认定的数据目录一致。
func (s *helperService) coreOutputLogPath() string {
	wl := loadPathWhitelist()
	return filepath.Join(wl.DataDir, "logs", "core-helper-"+time.Now().Format("20060102")+".log")
}

// openCoreOutputLog 以追加方式打开内核输出日志；失败返回 nil（调用方退回 NUL 并留痕）。
func openCoreOutputLog(path string) *os.File {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil
	}
	return f
}

// startCoreProcess 启动 TUN 内核进程。
//
// 旧的 coreProc 会先被强制结束并等待退出，等待期间不持有 s.mu，
// 因此 Ping / core-status 不会被启动流程阻塞。启动前确保 helper 拥有控制台，
// 内核不带 CREATE_NO_WINDOW，从而继承该控制台（CTRL_BREAK 才能送达）。
func (s *helperService) startCoreProcess(corePath, binDir, runtimeConfig string, args []string) (int, error) {
	// 1. 清理旧内核：Kill + 等待退出（不持锁）
	if old := s.coreSnapshot(); old != nil {
		if err := s.terminateCore(old, sys.HelperCoreStopKillWait); err != nil {
			return 0, fmt.Errorf("清理旧内核失败: %w", err)
		}
	}

	// 2. 确保控制台可用。分配失败不阻止启动（TUN 仍能工作，只是无法优雅停止），
	//    失败原因记录在 consoleState 中并通过 core-status 暴露。
	_ = s.ensureConsole()

	// 3. 组装命令行
	argv := args
	if len(argv) == 0 {
		argv = []string{"-d", binDir, "-f", runtimeConfig}
	}

	cmd := exec.Command(corePath, argv...)
	cmd.Dir = binDir
	cmd.Env = os.Environ()
	// 只加 CREATE_NEW_PROCESS_GROUP：内核成为独立进程组（组 ID = 内核 PID），
	// CTRL_BREAK_EVENT 可以精确定点送达。
	// 绝不能加 CREATE_NO_WINDOW —— 那会让内核拥有私有控制台，与 helper 的控制台
	// 事件通道彻底断开，优雅停止永远不可达。
	cmd.SysProcAttr = &windows.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}

	// stdout/stderr 追加写入 <dataDir>/logs/core-helper-YYYYMMDD.log（F-A3-04）。
	// 以前两者都被丢到 NUL：内核起不来时既没有 /logs 流（HTTP 还没监听），
	// 也没有其它证据，只能得到一句"内核已异常退出"。
	// stdin 仍然接 NUL，避免内核从继承来的控制台读数据。
	var devNull *os.File
	if f, err := os.OpenFile(os.DevNull, os.O_RDWR, 0); err == nil {
		devNull = f
	}

	logFile := openCoreOutputLog(s.coreOutputLogPath())
	if logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	} else if devNull != nil {
		// 日志文件打不开时退回 NUL，但要留痕（stderr 进 SCM 事件日志）。
		fmt.Fprintf(os.Stderr, "helper: 无法打开内核输出日志 %s，退回 NUL\n", s.coreOutputLogPath())
		cmd.Stdout = devNull
		cmd.Stderr = devNull
	}
	if devNull != nil {
		cmd.Stdin = devNull
	}

	if err := cmd.Start(); err != nil {
		if devNull != nil {
			_ = devNull.Close()
		}
		if logFile != nil {
			_ = logFile.Close()
		}
		return 0, fmt.Errorf("start core failed: %w", err)
	}
	if devNull != nil {
		_ = devNull.Close() // 子进程已继承句柄
	}

	proc := &coreProc{
		cmd:     cmd,
		pid:     cmd.Process.Pid,
		done:    make(chan struct{}),
		state:   coreStateRunning,
		logFile: logFile,
	}

	// 日志文件头：把启动时间/PID/命令行记在输出之前，方便把 stdout 与具体实例对齐。
	if logFile != nil {
		fmt.Fprintf(logFile, "\n===== core start at %s pid=%d =====\ncmd: %s %s\n",
			time.Now().Format(time.RFC3339Nano), cmd.Process.Pid, corePath, strings.Join(argv, " "))
	}

	s.mu.Lock()
	s.core = proc
	s.mu.Unlock()

	// 启动唯一的 cmd.Wait 调用者
	s.superviseCore(proc)

	return proc.pid, nil
}

// terminateCore 强制结束内核进程并等待退出确认（等待期间不持 s.mu）。
func (s *helperService) terminateCore(p *coreProc, wait time.Duration) error {
	if p == nil {
		return nil
	}

	if p.cmd != nil && p.cmd.Process != nil {
		if err := p.cmd.Process.Kill(); err != nil {
			// 进程可能在 Kill 之前已经自行退出
			select {
			case <-p.done:
				s.clearCore(p)
				return nil
			default:
			}
			return fmt.Errorf("结束内核进程 %d 失败: %w", p.pid, err)
		}
	}

	select {
	case <-p.done:
		s.clearCore(p)
		return nil
	case <-time.After(wait):
		return fmt.Errorf("内核进程 %d 在 %s 内未退出", p.pid, wait)
	}
}

// stopCoreGraceful 优雅停止内核：先发 CTRL_BREAK_EVENT 等待 grace 预算；
// 超时才强制 Kill，再等待退出确认。等待期间不持有 s.mu，不阻塞 Ping / core-status。
//
// 只有「已确认进程退出」才返回 nil；超时或无法确认退出时返回错误，绝不返回假成功。
func (s *helperService) stopCoreGraceful(p *coreProc, grace time.Duration) error {
	if p == nil {
		return nil
	}

	// 标记 stopping：core-status 在停止期间仍报告 running=true + stopping=true
	s.mu.Lock()
	if s.core == p && p.state == coreStateRunning {
		p.state = coreStateStopping
	}
	s.mu.Unlock()

	// 进程可能刚好自行退出：先确认一次，避免向已退出（甚至已复用）的 PID 发信号
	select {
	case <-p.done:
		s.clearCore(p)
		return nil
	default:
	}

	var ctrlErr error
	if p.pid > 0 && p.pid != os.Getpid() {
		ctrlErr = sendCtrlBreakToCore(p.pid)
		if ctrlErr != nil {
			fmt.Fprintf(os.Stderr, "helper: 向内核进程组 %d 发送 CTRL_BREAK 失败，将强制结束: %v\n", p.pid, ctrlErr)
		}
	} else {
		ctrlErr = errors.New("无效的内核 PID")
	}

	if ctrlErr == nil {
		select {
		case <-p.done:
			s.clearCore(p)
			return nil
		case <-time.After(grace):
			fmt.Fprintf(os.Stderr, "helper: 内核 %d 未在 %s 内优雅退出，强制结束\n", p.pid, grace)
		}
	}

	// 强制结束 + 等待退出确认
	if err := s.terminateCore(p, sys.HelperCoreStopKillWait); err != nil {
		if ctrlErr != nil {
			return fmt.Errorf("内核优雅停止不可用（%v）且强制结束未确认: %w", ctrlErr, err)
		}
		return fmt.Errorf("内核未在 %s 内优雅退出，且强制结束未确认: %w", grace, err)
	}
	return nil
}

func (s *helperService) handleRegisterParent(conn net.Conn, params jsonutil.RawMessage) {
	var p struct {
		PID int `json:"pid"`
	}
	if err := jsonv2.Unmarshal(params, &p); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("invalid params: %v", err))
		return
	}
	if p.PID <= 0 {
		s.writeResponse(conn, false, nil, "invalid pid")
		return
	}
	s.mu.Lock()
	s.parentPID = uint32(p.PID)
	s.mu.Unlock()
	s.writeResponse(conn, true, nil, "")
}

func (s *helperService) handleStopCore(conn net.Conn, params jsonutil.RawMessage) {
	// 只做快照，立刻释放 s.mu：优雅停止最长可能等待 20s（可配置），
	// 绝不能把锁握在手里，否则 Ping / core-status 会被一起阻塞。
	p := s.coreSnapshot()
	if p == nil {
		s.writeResponse(conn, true, nil, "")
		return
	}

	if err := s.stopCoreGraceful(p, sys.HelperCoreStopGrace()); err != nil {
		// 未确认退出：如实返回错误，不返回假成功
		s.writeResponse(conn, false, nil, err.Error())
		return
	}

	s.writeResponse(conn, true, nil, "")
}

func (s *helperService) handleCoreStatus(conn net.Conn) {
	// 只做极短的加锁快照，不参与任何等待，保证 core-status 永不被停止流程阻塞。
	s.mu.Lock()
	p := s.core
	running := p != nil
	stopping := p != nil && p.state == coreStateStopping
	pid := 0
	state := coreStateStopped.String()
	if p != nil {
		pid = p.pid
		state = p.state.String()
	}
	// lastExit* 与 core 同锁读取，避免退出瞬间读到半更新的一组值。
	lastExitPID := s.lastExitPID
	lastExitCode := s.lastExitCode
	lastExitAt := s.lastExitAt
	s.mu.Unlock()

	data := map[string]interface{}{
		"running":  running,
		"stopping": stopping,
		"state":    state,
	}
	if pid > 0 {
		data["pid"] = pid
	}
	// 退出码/退出时间（F-A3-04）：协议只加字段，旧客户端会忽略这些键。
	if !lastExitAt.IsZero() {
		data["exitCode"] = lastExitCode
		data["exitAt"] = lastExitAt.Format(time.RFC3339Nano)
		data["lastExitPid"] = lastExitPID
	}
	if consoleErr := s.consoleError(); consoleErr != "" {
		data["consoleError"] = consoleErr
	}

	jsonData, _ := jsonv2.Marshal(data)
	s.writeResponse(conn, true, jsonutil.RawMessage(jsonData), "")
}

func (s *helperService) handleRepairPermission(conn net.Conn, params jsonutil.RawMessage) {
	var p struct {
		DataDir string `json:"dataDir"`
	}
	if err := jsonv2.Unmarshal(params, &p); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("invalid params: %v", err))
		return
	}

	wl := loadPathWhitelist()
	dataDir := wl.DataDir
	if !strings.EqualFold(filepath.Clean(p.DataDir), dataDir) {
		s.writeResponse(conn, false, nil, fmt.Sprintf(
			"can only repair GoclashZ data dir: %q (expected %q)", filepath.Clean(p.DataDir), dataDir))
		return
	}

	cmd := exec.Command("icacls", dataDir, "/grant", "Users:(OI)(CI)F", "/T", "/Q")
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("icacls failed: %v, output: %s", err, string(output)))
		return
	}

	s.writeResponse(conn, true, nil, "")
}

func (s *helperService) handleReplaceCoreFile(conn net.Conn, params jsonutil.RawMessage) {
	var p struct {
		Source string `json:"source"`
		Target string `json:"target"`
		SHA256 string `json:"sha256"`
	}
	if err := jsonv2.Unmarshal(params, &p); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("invalid params: %v", err))
		return
	}

	wl := loadPathWhitelist()
	source := filepath.Clean(p.Source)
	target := filepath.Clean(p.Target)

	// 白名单校验：目标必须是白名单目录下的 clash.exe；
	// 来源只能是 <dataDir>/staging，或与目标同目录的 clash.exe.new 暂存文件。
	if _, ok := wl.matchCoreBinary(target, coreBinaryName); !ok {
		s.writeResponse(conn, false, nil, fmt.Sprintf(
			"target must be core binary: %q (allowed: %s)", target, strings.Join(wl.coreBinaryPaths(), ", ")))
		return
	}
	if !wl.allowStagedSource(source, coreBinaryName) {
		s.writeResponse(conn, false, nil, fmt.Sprintf(
			"source must be under staging dir or <coreBin>/%s.new: %q", coreBinaryName, source))
		return
	}
	if _, err := os.Stat(source); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("source not found: %v", err))
		return
	}

	// SHA256 校验
	if p.SHA256 != "" {
		hash, err := sha256File(source)
		if err != nil {
			s.writeResponse(conn, false, nil, fmt.Sprintf("sha256 calc failed: %v", err))
			return
		}
		if !strings.EqualFold(hash, p.SHA256) {
			s.writeResponse(conn, false, nil, "sha256 mismatch")
			return
		}
	}

	// PE + 体积校验（与主进程 core/clash/setup.go 的 5MiB~300MiB 口径一致）
	if err := validatePEFile(source, coreExeMinSize, coreExeMaxSize); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("source rejected: %v", err))
		return
	}

	// 停止 core（不持锁等待，避免阻塞 Ping / core-status）
	if old := s.coreSnapshot(); old != nil {
		if err := s.terminateCore(old, sys.HelperCoreStopKillWait); err != nil {
			s.writeResponse(conn, false, nil, fmt.Sprintf("停止内核失败: %v", err))
			return
		}
	}

	// 原子替换（F-A2-13）：先写 <target>.new，再同目录 rename 覆盖 target。
	// 不再直接 os.WriteFile 目标文件——中途失败/崩溃会留下半截的 61MB 内核。
	if err := replaceFileAtomic(source, target); err != nil {
		s.writeResponse(conn, false, nil, err.Error())
		return
	}

	s.writeResponse(conn, true, nil, "")
}

func (s *helperService) handleInstallWintun(conn net.Conn, params jsonutil.RawMessage) {
	var p struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}
	if err := jsonv2.Unmarshal(params, &p); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("invalid params: %v", err))
		return
	}

	wl := loadPathWhitelist()
	source := filepath.Clean(p.Source)
	target := filepath.Clean(p.Target)

	// 白名单校验：目标必须是白名单目录下的 wintun.dll；
	// 来源只能是 <dataDir>/staging，或与目标同目录的 wintun.dll.new 暂存文件。
	if _, ok := wl.matchCoreBinary(target, wintunBinaryName); !ok {
		s.writeResponse(conn, false, nil, fmt.Sprintf(
			"target must be wintun.dll: %q (allowed: %s)", target, strings.Join(wl.binaryPaths(wintunBinaryName), ", ")))
		return
	}
	if !wl.allowStagedSource(source, wintunBinaryName) {
		s.writeResponse(conn, false, nil, fmt.Sprintf(
			"source must be under staging dir or <coreBin>/%s.new: %q", wintunBinaryName, source))
		return
	}
	if _, err := os.Stat(source); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("source wintun.dll not found: %v", err))
		return
	}

	// DLL PE + 体积校验（与主进程 core/clash/wintun.go 的 32KiB~5MiB 口径一致）
	if err := validatePEFile(source, wintunMinSize, wintunMaxSize); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("source rejected: %v", err))
		return
	}

	// 安装前必须先停内核：wintun.dll 正被运行中的内核以 image 方式映射进来，
	// Windows 不允许覆盖映射中的镜像（否则只会得到 write target failed: Access is denied）。
	// 安装完成后**不自动拉起**，由调用方/用户决定何时重新开启 TUN。
	if old := s.coreSnapshot(); old != nil {
		if err := s.stopCoreGraceful(old, coreStopShutdownGrace); err != nil {
			s.writeResponse(conn, false, nil, fmt.Sprintf("停止内核失败，无法安装 Wintun: %v", err))
			return
		}
	}

	// 原子替换（F-A2-13）：同 replace-core-file，先 .new 再 rename 覆盖。
	if err := replaceFileAtomic(source, target); err != nil {
		s.writeResponse(conn, false, nil, err.Error())
		return
	}

	s.writeResponse(conn, true, nil, "")
}

// replaceFileAtomic 把 source 的内容原子替换到 target：
//
//	备份 target -> .bak  →  复制 source 到 target.new  →  rename(target.new, target)
//
// rename 只发生在同一目录内（原子），失败时回滚 .bak 并在错误里给出 backup 路径，
// 不再出现"写了一半的 target + 被吞掉的 rename 错误"（F-A2-13）。
func replaceFileAtomic(source, target string) error {
	staged := target + ".new"
	backup := target + ".bak"

	// 调用方（core/clash 的 setup.go / wintun.go）本来就是按 <target>.new 暂存的，
	// 此时 source 与 staged 是同一个文件——绝不能先 Remove(staged) 再复制，
	// 那会把要安装的新文件自己删掉。
	sameFile := strings.EqualFold(filepath.Clean(source), filepath.Clean(staged))

	if !sameFile {
		_ = os.Remove(staged)
	}
	_ = os.Remove(backup)

	backupMade := false
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return fmt.Errorf("备份旧文件失败（目标可能被占用）: %v", err)
		}
		backupMade = true
	}

	if !sameFile {
		if err := copyFileTo(source, staged); err != nil {
			_ = os.Remove(staged)
			if backupMade {
				_ = os.Rename(backup, target)
			}
			return fmt.Errorf("准备新文件失败: %v", err)
		}
	}

	if err := os.Rename(staged, target); err != nil {
		if !sameFile {
			_ = os.Remove(staged)
		}
		if backupMade {
			_ = os.Rename(backup, target)
		}
		return fmt.Errorf("替换失败（目标被占用）: %v (backup=%s)", err, backup)
	}

	_ = os.Remove(backup)
	return nil
}

// copyFileTo 复制文件内容（跨卷安全：先复制到同目录临时文件，再由调用方 rename）。
func copyFileTo(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}

// stopCore 在服务退出 / 父进程消失 / debug 模式退出时停止内核。
// 使用较短的优雅预算，避免 SCM 停止服务时超时。
func (s *helperService) stopCore() {
	p := s.coreSnapshot()
	if p == nil {
		return
	}

	if err := s.stopCoreGraceful(p, coreStopShutdownGrace); err != nil {
		fmt.Fprintf(os.Stderr, "helper: 退出时停止内核失败: %v\n", err)
	}
}

func (s *helperService) writeResponse(conn net.Conn, ok bool, data jsonutil.RawMessage, errMsg string) {
	resp := struct {
		OK    bool                `json:"ok"`
		Data  jsonutil.RawMessage `json:"data,omitempty"`
		Error string              `json:"error,omitempty"`
	}{
		OK:    ok,
		Data:  data,
		Error: errMsg,
	}
	encoder := jsonutil.NewEncoder(conn)
	encoder.Encode(resp)
}

func runDebug() {
	pipeName := `\\.\pipe\GoclashZ.Helper.debug`
	fmt.Printf("GoclashZHelper starting in debug mode on pipe: %s\n", pipeName)

	cfg := &winio.PipeConfig{
		InputBufferSize:  4096,
		OutputBufferSize: 4096,
	}

	ln, err := winio.ListenPipe(pipeName, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen pipe failed: %v\n", err)
		os.Exit(1)
	}
	defer ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &helperService{
		cancel: cancel,
		ln:     ln,
	}

	go s.watchParent(ctx, cancel)

	go func() {
		<-ctx.Done()
		ln.Close()
		s.stopCore()
		os.Exit(0)
	}()

	fmt.Println("Waiting for connections...")
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			fmt.Fprintf(os.Stderr, "accept error: %v\n", err)
			continue
		}
		go s.handleConn(conn)
	}
}

func installService(allowedSid string) {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get executable path: %v\n", err)
		os.Exit(1)
	}

	m, err := mgr.Connect()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to SCM: %v\n", err)
		os.Exit(1)
	}
	defer m.Disconnect()

	s, err := m.CreateService(serviceName, exePath, mgr.Config{
		DisplayName:  "GoclashZ Helper Service",
		Description:  "为 GoclashZ 提供高权限能力：TUN 启动、Wintun 安装、核心文件替换、权限修复",
		StartType:    mgr.StartManual,
		ErrorControl: mgr.ErrorNormal,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create service: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	// 写入允许的用户 SID 到注册表
	if allowedSid != "" {
		writeAllowedSid(allowedSid)
	}

	fmt.Println("GoclashZHelper service installed successfully")
}

func writeAllowedSid(sid string) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, registryPath, registry.SET_VALUE)
	if err != nil {
		// 尝试创建
		key, _, err = registry.CreateKey(registry.LOCAL_MACHINE, registryPath, registry.SET_VALUE)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to write allowed SID to registry: %v\n", err)
			return
		}
	}
	defer key.Close()

	if err := key.SetStringValue("AllowedSids", sid); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to set AllowedSids: %v\n", err)
	}
}

func readAllowedSids() []string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, registryPath, registry.READ)
	if err != nil {
		return nil
	}
	defer key.Close()

	val, _, err := key.GetStringValue("AllowedSids")
	if err != nil || val == "" {
		return nil
	}

	return []string{val}
}

// isSIDString 严格校验 SID 字面量：只允许 "S-1-" 前缀与数字/连字符。
// 目的有二：① 防止注册表里的脏值拼进 SDDL 造成语法注入；② 让"没登记 SID"可被识别。
func isSIDString(s string) bool {
	if len(s) < 4 || len(s) > 184 || !strings.HasPrefix(s, "S-1-") {
		return false
	}
	// 跳过开头的 'S'，其余只允许数字与连字符（防止注册表脏值拼进 SDDL 造成语法注入）。
	for _, r := range s[1:] {
		if (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

func buildPipeSDDL(allowedSids []string) string {
	// 安全加固（F-A2-04）：不再对 Interactive Users 授予 GENERIC_ALL。
	// GA 含 WRITE_DAC/WRITE_OWNER，任意交互用户可改写管道 DACL，进而让 SYSTEM 服务
	// 执行任意程序（本地提权）。这里只给 SYSTEM/Administrators 完全控制，再按
	// 服务登记的 AllowedSids 逐个授权安装者（GA，保持历史能力但不外溢到其它用户）。
	sddl := "D:(A;;GA;;;SY)(A;;GA;;;BA)"

	registered := 0
	for _, sid := range allowedSids {
		sid = strings.TrimSpace(sid)
		if !isSIDString(sid) {
			if sid != "" {
				fmt.Fprintf(os.Stderr, "helper: 忽略非法 SID 字面量: %q\n", sid)
			}
			continue
		}
		sddl += "(A;;GA;;;" + sid + ")"
		registered++
	}

	if registered == 0 {
		// 未登记任何 SID（旧版本安装、注册表被清理、或手工 install 未带 --allowed-sid）：
		// 既不能继续给 GA（提权），也不能一个都不给（TUN 会整体不可用且用户无从得知）。
		// 折中：Interactive Users 仅 GRGW（读请求/写应答），拿不到 WRITE_DAC、WRITE_OWNER。
		// 这是显式降级，必须留痕，不得静默。
		fmt.Fprintf(os.Stderr,
			"helper: WARNING AllowedSids 为空，管道 DACL 对 Interactive Users 降级为 GRGW；"+
				"请以管理员重装/修复后台服务以写入安装者 SID（服务键 %s 的 AllowedSids）\n", registryPath)
		sddl += "(A;;GRGW;;;IU)"
	}

	return sddl
}

func uninstallService() {
	m, err := mgr.Connect()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to SCM: %v\n", err)
		os.Exit(1)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open service: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	status, _ := s.Query()
	if status.State != svc.Stopped {
		s.Control(svc.Stop)
		time.Sleep(2 * time.Second)
	}

	if err := s.Delete(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to delete service: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("GoclashZHelper service uninstalled successfully")
}
