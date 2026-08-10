//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	coreCmd   *exec.Cmd
	corePID   int
	parentPID uint32 // PID of the owning main process; watched for abnormal exit
	ln        net.Listener
}

// getPathWhitelist 返回允许的文件操作白名单
func getPathWhitelist() (coreBinDir, stagingDir, dataDir string) {
	exe, _ := os.Executable()
	appDir := filepath.Dir(exe)
	coreBinDir = filepath.Join(appDir, "core", "bin")

	// 读取 data dir：优先环境变量，其次 {app}\data
	dataDir = os.Getenv("GOCLASHZ_DATA_DIR")
	if dataDir == "" {
		dataDir = filepath.Join(appDir, "data")
	}
	stagingDir = filepath.Join(dataDir, "staging")

	return
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
		checkInterval = 5 * time.Second
		graceTimeout  = 30 * time.Second
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
			cancel()
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
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	decoder := json.NewDecoder(conn)
	var req struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params,omitempty"`
	}
	if err := decoder.Decode(&req); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("decode failed: %v", err))
		return
	}

	switch req.Method {
	case "ping":
		s.writeResponse(conn, true, nil, "")
	case "shutdown":
		s.writeResponse(conn, true, nil, "")
		// 延迟退出，让响应先发出去
		go func() {
			time.Sleep(200 * time.Millisecond)
			if s.ln != nil {
				s.ln.Close()
			}
			s.stopCore()
			os.Exit(0)
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

func (s *helperService) handleStartCore(conn net.Conn, params json.RawMessage) {
	var p struct {
		CorePath      string   `json:"corePath"`
		BinDir        string   `json:"binDir"`
		RuntimeConfig string   `json:"runtimeConfig"`
		Args          []string `json:"args"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("invalid params: %v", err))
		return
	}

	if !filepath.IsAbs(p.CorePath) || !filepath.IsAbs(p.BinDir) {
		s.writeResponse(conn, false, nil, "absolute paths required")
		return
	}

	if _, err := os.Stat(p.CorePath); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("core not found: %v", err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.coreCmd != nil && s.coreCmd.Process != nil {
		s.coreCmd.Process.Kill()
		s.coreCmd.Wait()
		s.coreCmd = nil
		s.corePID = 0
	}

	args := p.Args
	if len(args) == 0 {
		args = []string{"-d", p.BinDir, "-f", p.RuntimeConfig}
	}

	cmd := exec.Command(p.CorePath, args...)
	cmd.Dir = p.BinDir
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &windows.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP,
	}

	if err := cmd.Start(); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("start core failed: %v", err))
		return
	}

	s.coreCmd = cmd
	s.corePID = cmd.Process.Pid

	go func() {
		cmd.Wait()
		s.mu.Lock()
		if s.coreCmd == cmd {
			s.coreCmd = nil
			s.corePID = 0
		}
		s.mu.Unlock()
	}()

	s.writeResponse(conn, true, nil, "")
}

func (s *helperService) handleRegisterParent(conn net.Conn, params json.RawMessage) {
	var p struct {
		PID int `json:"pid"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
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

func (s *helperService) handleStopCore(conn net.Conn, params json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.coreCmd == nil || s.coreCmd.Process == nil {
		s.writeResponse(conn, true, nil, "")
		return
	}

	// 直接 Kill，不发 CTRL_BREAK_EVENT：
	// 1. core 以 CREATE_NO_WINDOW | CREATE_NEW_PROCESS_GROUP 运行，
	//    CTRL_BREAK_EVENT 会触发 Mihomo 的优雅关闭流程（清理 TUN 路由/DNS），
	//    需要 1-3 秒才能完成，导致关闭缓慢。
	// 2. Wintun 驱动在进程退出时由 Windows 内核自动回收，无需等待优雅关闭。
	// 3. 与非 helper 路径（runner.go）保持一致：直接 Kill < 10ms 完成。
	_ = s.coreCmd.Process.Kill()

	done := make(chan struct{})
	go func() {
		s.coreCmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		// 兜底：Kill 后进程通常 100ms 内退出，超时直接继续
	}

	s.coreCmd = nil
	s.corePID = 0
	s.writeResponse(conn, true, nil, "")
}

func (s *helperService) handleCoreStatus(conn net.Conn) {
	s.mu.Lock()
	running := s.coreCmd != nil && s.coreCmd.Process != nil
	pid := s.corePID
	s.mu.Unlock()

	data := map[string]interface{}{
		"running": running,
	}
	if running {
		data["pid"] = pid
	}

	jsonData, _ := json.Marshal(data)
	s.writeResponse(conn, true, json.RawMessage(jsonData), "")
}

func (s *helperService) handleRepairPermission(conn net.Conn, params json.RawMessage) {
	var p struct {
		DataDir string `json:"dataDir"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("invalid params: %v", err))
		return
	}

	_, _, dataDir := getPathWhitelist()
	if !strings.EqualFold(filepath.Clean(p.DataDir), filepath.Clean(dataDir)) {
		s.writeResponse(conn, false, nil, "can only repair GoclashZ data dir")
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

func (s *helperService) handleReplaceCoreFile(conn net.Conn, params json.RawMessage) {
	var p struct {
		Source string `json:"source"`
		Target string `json:"target"`
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("invalid params: %v", err))
		return
	}

	coreBinDir, stagingDir, _ := getPathWhitelist()
	source := filepath.Clean(p.Source)
	target := filepath.Clean(p.Target)

	// 白名单校验
	if !isUnderPath(source, stagingDir) {
		s.writeResponse(conn, false, nil, "source must be under staging dir")
		return
	}
	expectedTarget := filepath.Join(coreBinDir, "clash.exe")
	if !strings.EqualFold(target, expectedTarget) {
		s.writeResponse(conn, false, nil, "target must be core binary")
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

	// PE 校验
	if !isValidPE(source) {
		s.writeResponse(conn, false, nil, "source is not a valid PE")
		return
	}

	// 停止 core
	s.mu.Lock()
	if s.coreCmd != nil && s.coreCmd.Process != nil {
		s.coreCmd.Process.Kill()
		s.coreCmd.Wait()
		s.coreCmd = nil
		s.corePID = 0
	}
	s.mu.Unlock()

	// 原子替换：target -> .bak，source -> target
	_ = os.Remove(target + ".bak")
	_ = os.Rename(target, target+".bak")

	input, err := os.ReadFile(source)
	if err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("read source failed: %v", err))
		return
	}

	if err := os.WriteFile(target, input, 0755); err != nil {
		// rollback
		_ = os.Rename(target+".bak", target)
		s.writeResponse(conn, false, nil, fmt.Sprintf("write target failed: %v", err))
		return
	}

	s.writeResponse(conn, true, nil, "")
}

func (s *helperService) handleInstallWintun(conn net.Conn, params json.RawMessage) {
	var p struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("invalid params: %v", err))
		return
	}

	coreBinDir, stagingDir, _ := getPathWhitelist()
	source := filepath.Clean(p.Source)
	target := filepath.Clean(p.Target)

	// 白名单校验
	if !isUnderPath(source, stagingDir) {
		s.writeResponse(conn, false, nil, "source must be under staging dir")
		return
	}
	expectedTarget := filepath.Join(coreBinDir, "wintun.dll")
	if !strings.EqualFold(target, expectedTarget) {
		s.writeResponse(conn, false, nil, "target must be wintun.dll")
		return
	}
	if _, err := os.Stat(source); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("source wintun.dll not found: %v", err))
		return
	}

	// DLL PE 校验
	if !isValidPE(source) {
		s.writeResponse(conn, false, nil, "source is not a valid DLL/PE")
		return
	}

	// 原子替换
	_ = os.Remove(target + ".bak")
	_ = os.Rename(target, target+".bak")

	input, err := os.ReadFile(source)
	if err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("read source failed: %v", err))
		return
	}

	if err := os.WriteFile(target, input, 0755); err != nil {
		_ = os.Rename(target+".bak", target)
		s.writeResponse(conn, false, nil, fmt.Sprintf("write target failed: %v", err))
		return
	}

	s.writeResponse(conn, true, nil, "")
}

func (s *helperService) stopCore() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.coreCmd != nil && s.coreCmd.Process != nil {
		_ = s.coreCmd.Process.Kill()

		done := make(chan struct{})
		go func() {
			s.coreCmd.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(1 * time.Second):
		}

		s.coreCmd = nil
		s.corePID = 0
	}
}

func (s *helperService) writeResponse(conn net.Conn, ok bool, data json.RawMessage, errMsg string) {
	resp := struct {
		OK    bool            `json:"ok"`
		Data  json.RawMessage `json:"data,omitempty"`
		Error string          `json:"error,omitempty"`
	}{
		OK:    ok,
		Data:  data,
		Error: errMsg,
	}
	encoder := json.NewEncoder(conn)
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

	s := &helperService{}
	fmt.Println("Waiting for connections...")
	for {
		conn, err := ln.Accept()
		if err != nil {
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

func buildPipeSDDL(allowedSids []string) string {
	sddl := "D:(A;;GA;;;SY)(A;;GA;;;BA)"
	for _, sid := range allowedSids {
		sddl += "(A;;GA;;;" + sid + ")"
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
