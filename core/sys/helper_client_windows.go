//go:build windows

package sys

import (
	"context"
	jsonv2 "encoding/json/v2"

	"errors"
	"fmt"
	jsonutil "goclashz/core/jsonutil"
	"os"
	"time"

	"github.com/Microsoft/go-winio"
	"goclashz/core/logger"
	"golang.org/x/sys/windows/registry"
)

const (
	helperConnectTimeout = 150 * time.Millisecond

	// startCoreRPCTimeout 覆盖 helper 侧「清理旧内核（Kill + 等待退出）+ 启动新内核」的耗时。
	// 必须 ≥ helper 端 handlerTimeout("start-core")=25s，否则客户端会先放弃、
	// 而 helper 其实已经把内核拉起来（F-A2-11）。
	startCoreRPCTimeout = 25 * time.Second

	// replaceCoreFileRPCTimeout 覆盖 helper 侧「备份 + 复制 61MB + rename」的完整耗时，
	// 以及 install-wintun 前的优雅停止内核（F-A5-12）。
	replaceCoreFileRPCTimeout = 60 * time.Second
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
	case "start-core":
		return startCoreRPCTimeout
	case "stop-core":
		// helper 会先发 CTRL_BREAK 等内核优雅退出（预算 20s，可配置），
		// 超时才强杀并等待退出确认；客户端预算必须不小于 helper 的完整预算。
		return HelperStopCoreRPCTimeout()
	case "replace-core-file", "install-wintun":
		// 61MB 复制 + rename，install-wintun 还要先优雅停止内核（F-A5-12）。
		return replaceCoreFileRPCTimeout
	case "repair-permission":
		// icacls /T 遍历整个数据目录。
		return 30 * time.Second
	case "shutdown":
		return 10 * time.Second
	default:
		// 未知方法保守给 5s：以前统一吃 10s，会让"假死"更难被快速发现。
		return 5 * time.Second
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
	data, _ := jsonv2.Marshal(map[string]int{"pid": pid})
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
	data, err := jsonv2.Marshal(params)
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

// CoreStatusDetail 在 CoreStatusData 之上追加内核退出信息（F-A3-04）。
//
// helper 侧 core-status 只加了字段（exitCode/exitAt/lastExitPid），
// 因此这里用内嵌结构体承接，不必修改 helper_protocol.go 的既有类型。
type CoreStatusDetail struct {
	CoreStatusData

	// ExitCode 是最近一次内核退出的退出码（仅当 helper 报告过退出时有效）。
	ExitCode int `json:"exitCode,omitzero"`
	// ExitAt 是最近一次内核退出时间（RFC3339Nano）。
	ExitAt string `json:"exitAt,omitempty"`
	// LastExitPID 是最近一次退出的内核 PID，便于与日志里的 pid 对齐。
	LastExitPID int `json:"lastExitPid,omitzero"`
}

// CoreStatus 查询内核运行状态
func (c *HelperClient) CoreStatus() (CoreStatusData, error) {
	detail, err := c.CoreStatusDetail()
	return detail.CoreStatusData, err
}

// CoreStatusDetail 查询内核运行状态，并带上最近一次退出的退出码/时间。
func (c *HelperClient) CoreStatusDetail() (CoreStatusDetail, error) {
	var detail CoreStatusDetail
	resp, err := c.sendRequest("core-status", nil)
	if err != nil {
		return detail, fmt.Errorf("helper core-status failed: %w", err)
	}
	if !resp.OK {
		return detail, fmt.Errorf("helper core-status error: %s", resp.Error)
	}
	if err := jsonv2.Unmarshal(resp.Data, &detail); err != nil {
		return detail, fmt.Errorf("helper core-status decode failed: %w", err)
	}
	return detail, nil
}

// RepairPermission 通过 helper 修复数据目录权限
func (c *HelperClient) RepairPermission(dataDir string) error {
	data, _ := jsonv2.Marshal(map[string]string{"dataDir": dataDir})
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
	data, err := jsonv2.Marshal(params)
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
	data, err := jsonv2.Marshal(InstallWintunParams{Source: source, Target: target})
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
func (c *HelperClient) sendRequest(method string, params jsonutil.RawMessage) (*HelperResponse, error) {
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

	encoder := jsonutil.NewEncoder(conn)
	if err := encoder.Encode(req); err != nil {
		return nil, fmt.Errorf("encode request failed: %w", err)
	}

	decoder := jsonutil.NewDecoder(conn)
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
	// 兼容旧调用点：整条修复链路（停止 → 安装 → 启动重试 → 重建 → 就绪等待）
	// 给一个总预算，避免旧调用点传入的仍是「无限等」。
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	return RecoverHelperServiceForUserCtx(ctx, exePath, userSID)
}

// RecoverHelperServiceForUserCtx 是可取消版本（F-A0-05）：
// 每一处固定等待都换成 sleepCtx，并且在每个重试阶段落日志，
// 让「服务修复到底卡在哪一步」可以在日志里复盘。
func RecoverHelperServiceForUserCtx(ctx context.Context, exePath string, userSID string) error {
	if !CheckAdmin() {
		return fmt.Errorf("需要管理员权限修复 Helper 服务")
	}

	if _, err := os.Stat(exePath); err != nil {
		return fmt.Errorf("helper 程序不存在: %s: %w", exePath, err)
	}

	_ = StopHelperServiceCtx(ctx)

	if err := InstallOrRepairHelperServiceForUser(exePath, userSID); err != nil {
		return fmt.Errorf("安装/修复 Helper 服务失败: %w", err)
	}

	if err := StartHelperServiceCtx(ctx); err != nil {
		logger.Warnf("helper 服务首次启动失败，重试一次: %v", err)
		// 再试一次停止 + 启动，不要立即 delete。
		_ = StopHelperServiceCtx(ctx)
		if !sleepCtx(ctx, 500*time.Millisecond) {
			return fmt.Errorf("重启 helper 服务等待被取消: %w", ctx.Err())
		}

		if err2 := StartHelperServiceCtx(ctx); err2 != nil {
			logger.Warnf("helper 服务第二次启动仍失败，尝试重建服务: %v", err2)
			// 最后才尝试 delete/recreate。
			if delErr := UninstallHelperServiceCtx(ctx); delErr == nil {
				if !sleepCtx(ctx, 1200*time.Millisecond) {
					return fmt.Errorf("重建 helper 服务等待被取消: %w", ctx.Err())
				}

				if rebuildErr := InstallOrRepairHelperServiceForUser(exePath, userSID); rebuildErr != nil {
					return fmt.Errorf("重建 Helper 服务失败: startErr=%v retryErr=%v rebuildErr=%w", err, err2, rebuildErr)
				}

				if startErr := StartHelperServiceCtx(ctx); startErr != nil {
					return fmt.Errorf("重建后启动 Helper 服务失败: first=%v retry=%v final=%w", err, err2, startErr)
				}
			} else {
				return fmt.Errorf("启动 Helper 服务失败: first=%v retry=%v deleteErr=%w", err, err2, delErr)
			}
		}
	}

	if err := WaitForHelperReadyCtx(ctx, 30, 300*time.Millisecond); err != nil {
		logger.Errorf("helper 服务修复后 30 次探测仍不可达: %v", err)
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return UninstallHelperServiceCtx(ctx)
}

// UninstallHelperServiceCtx 是可取消版本（F-A0-05）。
func UninstallHelperServiceCtx(ctx context.Context) error {
	_ = StopHelperServiceCtx(ctx)
	return uninstallServiceSCM(HelperServiceName)
}

// StartHelperService 启动 helper 服务（需要管理员权限）
func StartHelperService() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return StartHelperServiceCtx(ctx)
}

// StartHelperServiceCtx 是可取消版本（F-A0-05）。
func StartHelperServiceCtx(ctx context.Context) error {
	return startServiceSCMCtx(ctx, HelperServiceName)
}

// StopHelperService 停止 helper 服务（需要管理员权限）
func StopHelperService() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return StopHelperServiceCtx(ctx)
}

// StopHelperServiceCtx 是可取消版本（F-A0-05）。
func StopHelperServiceCtx(ctx context.Context) error {
	return stopServiceSCMCtx(ctx, HelperServiceName)
}

// WaitForHelperReady 等待 helper 服务就绪
func WaitForHelperReady(maxRetries int, interval time.Duration) error {
	// 兼容旧调用点：总预算 = 轮询次数 × 间隔 + 每次 Ping 的最坏耗时余量。
	budget := time.Duration(maxRetries)*interval + 30*time.Second
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	return WaitForHelperReadyCtx(ctx, maxRetries, interval)
}

// WaitForHelperReadyCtx 是可取消版本：ctx 到期立即返回，
// 不再「固定睡满 maxRetries 轮」而无法中断（F-A0-05）。
func WaitForHelperReadyCtx(ctx context.Context, maxRetries int, interval time.Duration) error {
	client := NewHelperClient()
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			if !sleepCtx(ctx, interval) {
				return fmt.Errorf("等待 helper 服务就绪被取消: %w", ctx.Err())
			}
		}
		if err := client.Ping(); err == nil {
			logger.Infof("helper 服务就绪 (attempt %d/%d)", i+1, maxRetries)
			return nil
		}
	}
	logger.Warnf("helper 服务在 %d 次尝试（间隔 %s）后仍未就绪", maxRetries, interval)
	return fmt.Errorf("helper 服务在 %d 次尝试后仍未就绪", maxRetries)
}
