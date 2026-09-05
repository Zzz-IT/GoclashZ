//go:build windows

package sys

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Microsoft/go-winio"
	"goclashz/core/logger"
	"golang.org/x/sys/windows/registry"
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
	case "register-parent":
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

// Shutdown 通知 helper 服务自行退出（不需要管理员权限）
func (c *HelperClient) Shutdown() error {
	resp, err := c.sendRequest("shutdown", nil)
	if err != nil {
		return fmt.Errorf("helper shutdown failed: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("helper shutdown error: %s", resp.Error)
	}
	return nil
}

// RegisterParent 向 helper 注册当前进程的 PID。Helper 会周期性轮询该进程，
// 若主进程被强行终止且超过宽限期（8s）未恢复，helper 将自动退出。
// 错误不影响功能，调用方可忽略。
func (c *HelperClient) RegisterParent(pid int) error {
	data, _ := json.Marshal(map[string]int{"pid": pid})
	resp, err := c.sendRequest("register-parent", data)
	if err != nil {
		return fmt.Errorf("helper register-parent failed: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("helper register-parent error: %s", resp.Error)
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
//
// 设计原则：
//   - isServiceInstalled 在 ACCESS_DENIED 时返回 (true, nil)，服务视为已安装
//   - isServiceRunning 在 ACCESS_DENIED 时返回 (true, nil)，让 Ping 继续验证
//   - 只要 installed == true，无论 SCM 运行状态是否可确认，都尝试 Ping
//   - Ping 成功则以 Ping 结果为准，修正 Running/Reachable
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

	// 查询运行状态；ACCESS_DENIED 时已返回 (true, nil)，其他错误时 running=false
	running, runErr := isServiceRunning(HelperServiceName)
	if runErr == nil {
		status.Running = running
	}
	// runErr != nil（非 ACCESS_DENIED 的其他错误）时 Running 保持 false，
	// 但仍继续尝试 Ping，因为服务可能确实在运行

	// 只要已安装，无论是否能从 SCM 确认运行，都尝试 Ping
	client := NewHelperClient()
	if err := client.Ping(); err != nil {
		// Ping 失败时，若有 SCM 查询错误一并记录
		if runErr != nil {
			status.Error = fmt.Sprintf("服务状态查询受限: %v", runErr)
		}
		return status
	}

	// Ping 成功 → 服务确实在运行
	status.Running = true
	status.Reachable = true
	_ = client.RegisterParent(os.Getpid())
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
	return errors.New("deprecated: use InstallOrRepairHelperServiceForUser")
}

// InstallOrRepairHelperServiceForUser 安装或修复 helper 服务并授权指定用户 SID
// 服务已存在时跳过创建，继续补写 SID 和 DACL
func InstallOrRepairHelperServiceForUser(exePath string, userSID string) error {
	if _, err := os.Stat(exePath); err != nil {
		return fmt.Errorf("helper executable not found: %s: %w", exePath, err)
	}

	if err := installOrUpdateServiceSCM(HelperServiceName, exePath, HelperDescription); err != nil {
		return err
	}

	if userSID != "" {
		if err := writeAllowedSidToRegistry(userSID); err != nil {
			return fmt.Errorf("写入 AllowedSids 注册表失败: %w", err)
		}

		if err := grantServiceControlToUser(HelperServiceName, userSID); err != nil {
			// 这里不要直接失败。
			// 因为 f26c5ed 已经可能把旧服务 DACL 写坏，
			// 但只要管理员进程能启动服务，Helper 仍可恢复工作。
			logger.Warnf("设置服务 DACL 失败，继续尝试启动服务: %v", err)
		}
	}

	return nil
}

// RecoverHelperServiceForUser 强修复 Helper 服务
func RecoverHelperServiceForUser(exePath string, userSID string) error {
	if !CheckAdmin() {
		return fmt.Errorf("需要管理员权限修复 Helper 服务")
	}

	if _, err := os.Stat(exePath); err != nil {
		return fmt.Errorf("helper 程序不存在: %s: %w", exePath, err)
	}

	_ = StopHelperService()

	if err := InstallOrRepairHelperServiceForUser(exePath, userSID); err != nil {
		return fmt.Errorf("安装/修复 Helper 服务失败: %w", err)
	}

	if err := StartHelperService(); err != nil {
		// 再试一次停止 + 启动，不要立即 delete。
		_ = StopHelperService()
		time.Sleep(500 * time.Millisecond)

		if err2 := StartHelperService(); err2 != nil {
			// 最后才尝试 delete/recreate。
			if delErr := UninstallHelperService(); delErr == nil {
				time.Sleep(1200 * time.Millisecond)

				if rebuildErr := InstallOrRepairHelperServiceForUser(exePath, userSID); rebuildErr != nil {
					return fmt.Errorf("重建 Helper 服务失败: startErr=%v retryErr=%v rebuildErr=%w", err, err2, rebuildErr)
				}

				if startErr := StartHelperService(); startErr != nil {
					return fmt.Errorf("重建后启动 Helper 服务失败: first=%v retry=%v final=%w", err, err2, startErr)
				}
			} else {
				return fmt.Errorf("启动 Helper 服务失败: first=%v retry=%v deleteErr=%w", err, err2, delErr)
			}
		}
	}

	if err := WaitForHelperReady(30, 300*time.Millisecond); err != nil {
		return fmt.Errorf("Helper 服务启动后不可达: %w", err)
	}

	return nil
}

func writeAllowedSidToRegistry(sid string) error {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\GoclashZHelper`, registry.SET_VALUE)
	if err != nil {
		key, _, err = registry.CreateKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\GoclashZHelper`, registry.SET_VALUE)
		if err != nil {
			return fmt.Errorf("打开/创建注册表项失败: %w", err)
		}
	}
	defer key.Close()
	if err := key.SetStringValue("AllowedSids", sid); err != nil {
		return fmt.Errorf("设置 AllowedSids 值失败: %w", err)
	}
	return nil
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
