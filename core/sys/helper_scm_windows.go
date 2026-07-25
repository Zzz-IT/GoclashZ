//go:build windows

package sys

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func openServiceWithAccess(m *mgr.Mgr, name string, access uint32) (*mgr.Service, error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}

	handle, err := windows.OpenService(m.Handle, namePtr, access)
	if err != nil {
		return nil, err
	}

	return &mgr.Service{
		Name:   name,
		Handle: handle,
	}, nil
}

// isServiceInstalledSCM 通过 SCM 检查服务是否已注册
func isServiceInstalledSCM(name string) (bool, error) {
	m, err := mgr.Connect()
	if err != nil {
		return false, fmt.Errorf("SCM connect failed: %w", err)
	}
	defer m.Disconnect()

	s, err := openServiceWithAccess(m, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return false, nil
		}
		return false, fmt.Errorf("open service failed: %w", err)
	}
	defer s.Close()
	return true, nil
}

// isServiceRunningSCM 通过 SCM 检查服务是否正在运行
func isServiceRunningSCM(name string) (bool, error) {
	m, err := mgr.Connect()
	if err != nil {
		return false, fmt.Errorf("SCM connect failed: %w", err)
	}
	defer m.Disconnect()

	s, err := openServiceWithAccess(m, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return false, nil
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return false, fmt.Errorf("query service status failed: %w", err)
	}

	return status.State == svc.Running, nil
}

// installOrUpdateServiceSCM 通过 SCM 安装或更新服务
func installOrUpdateServiceSCM(name, exePath, description string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("SCM connect failed: %w", err)
	}
	defer m.Disconnect()

	s, err := m.CreateService(name, exePath, mgr.Config{
		DisplayName:  HelperDisplayName,
		Description:  description,
		StartType:    mgr.StartManual,
		ErrorControl: mgr.ErrorNormal,
	})

	if err != nil {
		if !errors.Is(err, windows.ERROR_SERVICE_EXISTS) {
			return fmt.Errorf("create service failed: %w", err)
		}

		s, err = openServiceWithAccess(
			m,
			name,
			windows.SERVICE_QUERY_CONFIG|windows.SERVICE_CHANGE_CONFIG,
		)
		if err != nil {
			return fmt.Errorf("open existing service failed: %w", err)
		}
	}
	defer s.Close()

	cfg, err := s.Config()
	if err != nil {
		return fmt.Errorf("query service config failed: %w", err)
	}

	changed := false

	if cfg.BinaryPathName != exePath {
		cfg.BinaryPathName = exePath
		changed = true
	}
	if cfg.DisplayName != HelperDisplayName {
		cfg.DisplayName = HelperDisplayName
		changed = true
	}
	if cfg.Description != description {
		cfg.Description = description
		changed = true
	}
	if cfg.StartType != mgr.StartManual {
		cfg.StartType = mgr.StartManual
		changed = true
	}
	if cfg.ErrorControl != mgr.ErrorNormal {
		cfg.ErrorControl = mgr.ErrorNormal
		changed = true
	}

	if changed {
		if err := s.UpdateConfig(cfg); err != nil {
			return fmt.Errorf("update service config failed: %w", err)
		}
	}

	return nil
}

// uninstallServiceSCM 通过 SCM 卸载服务
func uninstallServiceSCM(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("SCM connect failed: %w", err)
	}
	defer m.Disconnect()

	s, err := openServiceWithAccess(m, name, windows.SERVICE_QUERY_STATUS|windows.SERVICE_STOP|windows.DELETE)
	if err != nil {
		return fmt.Errorf("open service failed: %w", err)
	}
	defer s.Close()

	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service failed: %w", err)
	}

	return nil
}

// startServiceSCM 通过 SCM 启动服务（已运行时容错）
func startServiceSCM(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("SCM connect failed: %w", err)
	}
	defer m.Disconnect()

	s, err := openServiceWithAccess(m, name, windows.SERVICE_QUERY_STATUS|windows.SERVICE_START|windows.SERVICE_INTERROGATE)
	if err != nil {
		return fmt.Errorf("open service failed: %w", err)
	}
	defer s.Close()

	// 已运行则直接返回
	status, err := s.Query()
	if err == nil && status.State == svc.Running {
		return nil
	}

	if err := s.Start(); err != nil {
		if err == windows.ERROR_SERVICE_ALREADY_RUNNING {
			return nil
		}
		return fmt.Errorf("start service failed: %w", err)
	}

	// 等待服务进入 Running 状态
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, err := s.Query()
		if err != nil {
			return fmt.Errorf("query service status failed: %w", err)
		}
		if status.State == svc.Running {
			return nil
		}
		if status.State != svc.StartPending && status.State != svc.ContinuePending {
			return fmt.Errorf("service entered unexpected state: %v", status.State)
		}
		time.Sleep(200 * time.Millisecond)
	}

	return fmt.Errorf("service did not start within timeout")
}

// stopServiceSCM 通过 SCM 停止服务
func stopServiceSCM(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("SCM connect failed: %w", err)
	}
	defer m.Disconnect()

	s, err := openServiceWithAccess(m, name, windows.SERVICE_QUERY_STATUS|windows.SERVICE_STOP|windows.SERVICE_INTERROGATE)
	if err != nil {
		return fmt.Errorf("open service failed: %w", err)
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return fmt.Errorf("query service status failed: %w", err)
	}

	if status.State == svc.Stopped {
		return nil
	}

	if _, err := s.Control(svc.Stop); err != nil {
		return fmt.Errorf("stop service failed: %w", err)
	}

	// 等待服务停止
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, err := s.Query()
		if err != nil {
			return nil // 服务可能已删除
		}
		if status.State == svc.Stopped {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	return fmt.Errorf("service did not stop within timeout")
}

// grantServiceControlToUser 设置服务 DACL，允许指定用户 query/start/stop/interrogate
func grantServiceControlToUser(serviceName string, userSID string) error {
	adminRights := "CCDCLCSWRPWPDTLOCRSDRCWDWO"
	userRights := "LCRPWPLOCRRC"

	sddl := fmt.Sprintf(
		"D:(A;;%s;;;SY)(A;;%s;;;BA)(A;;%s;;;%s)",
		adminRights,
		adminRights,
		userRights,
		userSID,
	)
	cmd := exec.Command("sc", "sdset", serviceName, sddl)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("set service DACL failed: %w, output: %s", err, string(output))
	}
	return nil
}
