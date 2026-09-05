//go:build windows

package sys

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
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

// isServiceInstalledSCM 通过 SCM 检查服务是否已注册，失败时通过注册表兜底
func isServiceInstalledSCM(name string) (bool, error) {
	m, err := mgr.Connect()
	if err == nil {
		defer m.Disconnect()

		s, err := openServiceWithAccess(m, name, windows.SERVICE_QUERY_STATUS)
		if err == nil {
			s.Close()
			return true, nil
		}
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return false, nil
		}
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			// 服务存在但当前用户暂无查询权限（DACL 尚未生效或被覆盖）
			// 视为已安装，后续由 Ping 来确认可达性
			return true, nil
		}
	}

	// 注册表兜底检查（普通用户对 Services 注册表项拥有通用读取权限）
	key, regErr := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\`+name, registry.READ)
	if regErr == nil {
		key.Close()
		return true, nil
	}
	if errors.Is(regErr, registry.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("SCM connect failed: %w", err)
	}
	return false, fmt.Errorf("check service failed: %w", regErr)
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
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			// 无权查询运行状态，返回 true 让调用方继续尝试 Ping 验证
			return true, nil
		}
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

// grantServiceControlToUser 使用 Win32 SetSecurityInfo API 直接设置服务 DACL，
// 授权指定用户 SID 对服务进行 query/start/stop/interrogate 操作。
// 不依赖 sc.exe 子进程，在各种安全策略环境下更可靠。
func grantServiceControlToUser(serviceName string, userSID string) error {
	adminRights := "CCDCLCSWRPWPDTLOCRSDRCWDWO"
	userRights := "LCRPWPLOCRRC"

	// 构造与原来等价的 SDDL，但通过 Win32 API 写入，不走 sc.exe 子进程
	sddl := fmt.Sprintf(
		"D:(A;;%s;;;SY)(A;;%s;;;BA)(A;;%s;;;%s)",
		adminRights,
		adminRights,
		userRights,
		userSID,
	)

	// 将 SDDL 字符串解析为安全描述符
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("parse SDDL failed: %w", err)
	}

	// 提取 DACL
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("extract DACL from security descriptor failed: %w", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("SCM connect failed: %w", err)
	}
	defer m.Disconnect()

	// 需要 READ_CONTROL + WRITE_DAC 权限来写入 DACL
	s, err := openServiceWithAccess(m, serviceName,
		windows.READ_CONTROL|windows.WRITE_DAC)
	if err != nil {
		return fmt.Errorf("open service for DACL write failed: %w", err)
	}
	defer s.Close()

	// 通过 Win32 SetSecurityInfo 写入新 DACL
	return windows.SetSecurityInfo(
		windows.Handle(s.Handle),
		windows.SE_SERVICE,
		windows.DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
}
