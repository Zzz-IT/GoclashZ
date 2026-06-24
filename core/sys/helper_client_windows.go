//go:build windows

package sys

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Microsoft/go-winio"
	"goclashz/core/logger"
)

const (
	helperConnectTimeout = 150 * time.Millisecond
)

// getMethodTimeout 根据方法返回不同的请求超时
func getMethodTimeout(method string) time.Duration {
	switch method {
	case "ping":
		return 200 * time.Millisecond
	case "core-status":
		return 500 * time.Millisecond
	case "start-core", "stop-core":
		return 3 * time.Second
	default:
		return 10 * time.Second
	}
}

// HelperClient 通过 Named Pipe 与 GoclashZHelper 服务通信
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
func (c *HelperClient) StopCore() error {
	resp, err := c.sendRequest("stop-core", nil)
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

// InstallWintun 通过 helper 安装 Wintun 驱动文件
func (c *HelperClient) InstallWintun(source, target string) error {
	data, err := json.Marshal(InstallWintunParams{Source: source, Target: target})
	if err != nil {
		return err
	}
	resp, err := c.sendRequest("install-wintun", data)
	if err != nil {
		return fmt.Errorf("helper install-wintun failed: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("helper install-wintun error: %s", resp.Error)
	}
	return nil
}

// sendRequest 通过 Named Pipe 发送请求并等待响应
func (c *HelperClient) sendRequest(method string, params json.RawMessage) (*HelperResponse, error) {
	timeout := helperConnectTimeout
	conn, err := winio.DialPipe(c.pipeName, &timeout)
	if err != nil {
		return nil, fmt.Errorf("connect to helper pipe failed: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(getMethodTimeout(method))); err != nil {
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

	installed, err := isServiceInstalled(HelperServiceName)
	if err != nil {
		status.Error = fmt.Sprintf("检查服务注册失败: %v", err)
		return status
	}
	status.Installed = installed

	if !installed {
		return status
	}

	running, err := isServiceRunning(HelperServiceName)
	if err != nil {
		status.Error = fmt.Sprintf("检查服务状态失败: %v", err)
		return status
	}
	status.Running = running

	if !running {
		return status
	}

	client := NewHelperClient()
	if err := client.Ping(); err != nil {
		status.Error = fmt.Sprintf("服务可达性检查失败: %v", err)
		return status
	}
	status.Reachable = true

	return status
}

func isServiceInstalled(name string) (bool, error) {
	return isServiceInstalledSCM(name)
}

func isServiceRunning(name string) (bool, error) {
	return isServiceRunningSCM(name)
}

// InstallHelperService 安装 helper 服务（需要管理员权限）
func InstallHelperService(exePath string) error {
	return installServiceSCM(HelperServiceName, exePath, HelperDescription)
}

// UninstallHelperService 卸载 helper 服务（需要管理员权限）
func UninstallHelperService() error {
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

// WaitForHelperReady 等待 helper 服务就绪
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
