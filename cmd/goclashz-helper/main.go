//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName  = "GoclashZHelper"
	helperAddr   = "127.0.0.1:19720"
	helperSecret = "GoclashZ-Helper-v1"
)

type helperService struct {
	mu      sync.Mutex
	coreCmd *exec.Cmd
	corePID int
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "install":
			installService()
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

	ln, err := net.Listen("tcp", helperAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen failed: %v\n", err)
		changes <- svc.Status{State: svc.StopPending}
		return false, 1
	}

	changes <- svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptShutdown,
	}

	go s.serve(ln)

	for req := range r {
		switch req.Cmd {
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			ln.Close()
			s.stopCore()
			return false, 0
		}
	}

	return false, 0
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
		Secret string          `json:"secret"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params,omitempty"`
	}
	if err := decoder.Decode(&req); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("decode failed: %v", err))
		return
	}

	if req.Secret != helperSecret {
		s.writeResponse(conn, false, nil, "unauthorized")
		return
	}

	switch req.Method {
	case "ping":
		s.writeResponse(conn, true, nil, "")
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

func (s *helperService) handleStopCore(conn net.Conn, params json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.coreCmd == nil || s.coreCmd.Process == nil {
		s.writeResponse(conn, true, nil, "")
		return
	}

	if err := s.coreCmd.Process.Kill(); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("kill core failed: %v", err))
		return
	}

	done := make(chan struct{})
	go func() {
		s.coreCmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
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

	cmd := exec.Command("icacls", p.DataDir, "/grant", "Users:(OI)(CI)F", "/T", "/Q")
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

	if !filepath.IsAbs(p.Source) || !filepath.IsAbs(p.Target) {
		s.writeResponse(conn, false, nil, "absolute paths required")
		return
	}

	if _, err := os.Stat(p.Source); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("source not found: %v", err))
		return
	}

	s.mu.Lock()
	if s.coreCmd != nil && s.coreCmd.Process != nil {
		s.coreCmd.Process.Kill()
		s.coreCmd.Wait()
		s.coreCmd = nil
		s.corePID = 0
	}
	s.mu.Unlock()

	_ = os.Rename(p.Target, p.Target+".bak")

	input, err := os.ReadFile(p.Source)
	if err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("read source failed: %v", err))
		return
	}

	if err := os.WriteFile(p.Target, input, 0755); err != nil {
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

	if !filepath.IsAbs(p.Source) || !filepath.IsAbs(p.Target) {
		s.writeResponse(conn, false, nil, "absolute paths required")
		return
	}

	if _, err := os.Stat(p.Source); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("source wintun.dll not found: %v", err))
		return
	}

	_ = os.Rename(p.Target, p.Target+".bak")

	input, err := os.ReadFile(p.Source)
	if err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("read source failed: %v", err))
		return
	}

	if err := os.WriteFile(p.Target, input, 0755); err != nil {
		s.writeResponse(conn, false, nil, fmt.Sprintf("write target failed: %v", err))
		return
	}

	s.writeResponse(conn, true, nil, "")
}

func (s *helperService) stopCore() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.coreCmd != nil && s.coreCmd.Process != nil {
		s.coreCmd.Process.Kill()
		s.coreCmd.Wait()
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
	fmt.Printf("GoclashZHelper starting in debug mode on %s\n", helperAddr)

	ln, err := net.Listen("tcp", helperAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen failed: %v\n", err)
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

func installService() {
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
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create service: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	fmt.Println("GoclashZHelper service installed successfully")
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
