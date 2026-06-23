//go:build windows

package sys

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"goclashz/core/logger"
)

const (
	helperConnectTimeout = 3 * time.Second
	helperRequestTimeout = 10 * time.Second
)

// HelperClient 通过 named pipe 与 GoclashZHelper 服务通信
type HelperClient struct {
	pipeName string
}

// NewHelperClient 创建 helper 客户端
func NewHelperClient() *HelperClient {
	return &HelperClient{
		pipeName: GetHelperPipeName(),
	}
}

// Ping 检查 helper 服务是否可达
func (c *HelperClient) Ping() error {
	resp, err := c.sendRequest("ping", nil)
	if err != nil {
		return fmt.Errorf("helper ping failed: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("helper ping error: %s", resp.Error)
	}
	return nil
}

// StartCore 通过 helper 启动内核进程
func (c *HelperClient) StartCore(params StartCoreParams) error {
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	resp, err := c.sendRequest("start-core", data)
	if err != nil {
		return fmt.Errorf("helper start-core failed: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("helper start-core error: %s", resp.Error)
	}
	return nil
}

// StopCore 通过 helper 停止内核进程
func (c *HelperClient) StopCore(targetExeName string) error {
	data, err := json.Marshal(StopCoreParams{TargetExeName: targetExeName})
	if err != nil {
		return err
	}
	resp, err := c.sendRequest("stop-core", data)
	if err != nil {
		return fmt.Errorf("helper stop-core failed: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("helper stop-core error: %s", resp.Error)
	}
	return nil
}

// CoreStatus 查询内核运行状态
func (c *HelperClient) CoreStatus() (CoreStatusData, error) {
	var status CoreStatusData
	resp, err := c.sendRequest("core-status", nil)
	if err != nil {
		return status, fmt.Errorf("helper core-status failed: %w", err)
	}
	if !resp.OK {
		return status, fmt.Errorf("helper core-status error: %s", resp.Error)
	}
	if err := json.Unmarshal(resp.Data, &status); err != nil {
		return status, fmt.Errorf("helper core-status decode failed: %w", err)
	}
	return status, nil
}

// RepairPermission 通过 helper 修复数据目录权限
func (c *HelperClient) RepairPermission(dataDir string) error {
	data, _ := json.Marshal(map[string]string{"dataDir": dataDir})
	resp, err := c.sendRequest("repair-permission", data)
	if err != nil {
		return fmt.Errorf("helper repair-permission failed: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("helper repair-permission error: %s", resp.Error)
	}
	return nil
}

// ReplaceCoreFile 通过 helper 替换核心文件
func (c *HelperClient) ReplaceCoreFile(params ReplaceCoreFileParams) error {
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	resp, err := c.sendRequest("replace-core-file", data)
	if err != nil {
		return fmt.Errorf("helper replace-core-file failed: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("helper replace-core-file error: %s", resp.Error)
	}
	return nil
}

// sendRequest 通过 named pipe 发送请求并等待响应
func (c *HelperClient) sendRequest(method string, params json.RawMessage) (*HelperResponse, error) {
	if err := validatePipeName(c.pipeName); err != nil {
		return nil, err
	}

	conn, err := net.DialTimeout("pipe", c.pipeName, helperConnectTimeout)
	if err != nil {
		return nil, fmt.Errorf("connect to helper pipe failed: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(helperRequestTimeout)); err != nil {
		return nil, fmt.Errorf("set deadline failed: %w", err)
	}

	req := HelperRequest{
		Method: method,
		Params: params,
	}

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(req); err != nil {
		return nil, fmt.Errorf("encode request failed: %w", err)
	}

	decoder := json.NewDecoder(conn)
	var resp HelperResponse
	if err := decoder.Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	return &resp, nil
}

// CheckHelperService 检查 helper 服务状态（安装 + 运行 + 可达）
func CheckHelperService() HelperStatusData {
	status := HelperStatusData{}

	// 1. 检查服务是否已注册
	installed, err := isServiceInstalled(HelperServiceName)
	if err != nil {
		status.Error = fmt.Sprintf("检查服务注册失败: %v", err)
		return status
	}
	status.Installed = installed

	if !installed {
		return status
	}

	// 2. 检查服务是否正在运行
	running, err := isServiceRunning(HelperServiceName)
	if err != nil {
		status.Error = fmt.Sprintf("检查服务状态失败: %v", err)
		return status
	}
	status.Running = running

	if !running {
		return status
	}

	// 3. 通过 named pipe ping 验证可达
	client := NewHelperClient()
	if err := client.Ping(); err != nil {
		status.Error = fmt.Sprintf("服务可达性检查失败: %v", err)
		return status
	}
	status.Reachable = true

	return status
}

// isServiceInstalled 检查 Windows 服务是否已注册
func isServiceInstalled(name string) (bool, error) {
	return isServiceInstalledSCM(name)
}

// isServiceRunning 检查 Windows 服务是否正在运行
func isServiceRunning(name string) (bool, error) {
	return isServiceRunningSCM(name)
}

// InstallHelperService 安装 helper 服务（需要管理员权限）
func InstallHelperService(exePath string) error {
	return installServiceSCM(HelperServiceName, exePath, HelperDescription)
}

// UninstallHelperService 卸载 helper 服务（需要管理员权限）
func UninstallHelperService() error {
	// 先尝试停止服务
	_ = StopHelperService()
	return uninstallServiceSCM(HelperServiceName)
}

// StartHelperService 启动 helper 服务（需要管理员权限）
func StartHelperService() error {
	return startServiceSCM(HelperServiceName)
}

// StopHelperService 停止 helper 服务（需要管理员权限）
func StopHelperService() error {
	return stopServiceSCM(HelperServiceName)
}

// WaitForHelperReady 等待 helper 服务就绪（最多 maxRetries 次，每次间隔 interval）
func WaitForHelperReady(maxRetries int, interval time.Duration) error {
	client := NewHelperClient()
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			time.Sleep(interval)
		}
		if err := client.Ping(); err == nil {
			logger.Infof("helper 服务就绪 (attempt %d/%d)", i+1, maxRetries)
			return nil
		}
	}
	return fmt.Errorf("helper 服务在 %d 次尝试后仍未就绪", maxRetries)
}
