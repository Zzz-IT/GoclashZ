//go:build windows

package sys

import (
	"fmt"
	"os/exec"
	"time"

	"goclashz/core/logger"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// isServiceInstalledSCM 通过 SCM 检查服务是否已注册
func isServiceInstalledSCM(name string) (bool, error) {
	m, err := mgr.Connect()
	if err != nil {
		return false, fmt.Errorf("SCM connect failed: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		// 服务不存在
		return false, nil
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

	s, err := m.OpenService(name)
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

// installServiceSCM 通过 SCM 安装服务
func installServiceSCM(name, exePath, description string) error {
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
		return fmt.Errorf("create service failed: %w", err)
	}
	defer s.Close()

	return nil
}

// uninstallServiceSCM 通过 SCM 卸载服务
func uninstallServiceSCM(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("SCM connect failed: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
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

	s, err := m.OpenService(name)
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

	s, err := m.OpenService(name)
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

// grantServiceControlToUser 设置服务 DACL，允许指定用户 start/stop/query
func grantServiceControlToUser(serviceName string, userSID string) error {
	sddl := fmt.Sprintf("D:(A;;CCDCLCSWRPWPDTLOCRRC;;;SY)(A;;CCDCLCSWRPWPDTLOCRRC;;;BA)(A;;LCRPWP;;;%s)", userSID)
	cmd := exec.Command("sc", "sdset", serviceName, sddl)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Warnf("设置服务 DACL 失败（非致命）: %v, output: %s", err, string(output))
		return nil
	}
	return nil
}
