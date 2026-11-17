//go:build windows

package sys

import (
	"fmt"
	jsonutil "goclashz/core/jsonutil"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"
)

const (
	HelperServiceName = "GoclashZHelper"
	HelperDisplayName = "GoclashZ Helper Service"
	HelperDescription = "为 GoclashZ 提供高权限能力：TUN 启动、Wintun 安装、核心文件替换、权限修复"

	// 固定 Named Pipe 名称
	HelperPipeName = `\\.\pipe\GoclashZ.Helper`
)

const (
	// HelperCoreStopGraceDefault 是 helper 收到 stop-core 后等待内核优雅退出的默认预算。
	// TUN 内核通过 CTRL_BREAK_EVENT 触发 mihomo 的优雅关闭（清理 TUN 路由 / DNS），
	// 正常 1~3 秒完成；20s 只是上限，避免内核异常时无限等待。
	HelperCoreStopGraceDefault = 20 * time.Second

	// HelperCoreStopKillWait 是强制 Kill 之后等待进程彻底退出的预算。
	HelperCoreStopKillWait = 5 * time.Second

	// HelperCoreStopGraceEnv 允许用环境变量按「秒」覆盖优雅退出预算（排障用）。
	// helper 与客户端读取同一个变量，保证双方预算一致。
	HelperCoreStopGraceEnv = "GOCLASHZ_CORE_STOP_GRACE_SECONDS"
)

// HelperCoreStopGrace 返回内核优雅退出预算：默认 HelperCoreStopGraceDefault，
// 可由环境变量 HelperCoreStopGraceEnv 按秒覆盖（非法值忽略）。
func HelperCoreStopGrace() time.Duration {
	if raw := strings.TrimSpace(os.Getenv(HelperCoreStopGraceEnv)); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return HelperCoreStopGraceDefault
}

// HelperStopCoreRPCTimeout 返回客户端调用 stop-core 的 RPC 预算。
// 必须覆盖 helper 侧「优雅等待 + 强杀等待」的完整预算并留 5s 余量，
// 否则客户端会先于 helper 放弃等待，把「正在优雅停止」误判成失败。
func HelperStopCoreRPCTimeout() time.Duration {
	return HelperCoreStopGrace() + HelperCoreStopKillWait + 5*time.Second
}

// GetHelperPipeName 返回 Named Pipe 路径（固定名称）
func GetHelperPipeName() string {
	return HelperPipeName
}

// CurrentUserSID 获取当前用户的 SID
func CurrentUserSID() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("get current user failed: %w", err)
	}
	return u.Uid, nil
}

// HelperRequest 是 UI -> Helper 的请求
type HelperRequest struct {
	Method string              `json:"method"`
	Params jsonutil.RawMessage `json:"params,omitempty"`
}

// HelperResponse 是 Helper -> UI 的响应
type HelperResponse struct {
	OK    bool                `json:"ok"`
	Data  jsonutil.RawMessage `json:"data,omitempty"`
	Error string              `json:"error,omitempty"`
}

// StartCoreParams 启动内核的参数
type StartCoreParams struct {
	CorePath      string   `json:"corePath"`
	BinDir        string   `json:"binDir"`
	RuntimeConfig string   `json:"runtimeConfig"`
	Args          []string `json:"args"`
}

// ReplaceCoreFileParams 替换核心文件的参数
type ReplaceCoreFileParams struct {
	Source string `json:"source"`
	Target string `json:"target"`
	SHA256 string `json:"sha256"`
}

// InstallWintunParams 安装 Wintun 驱动的参数
type InstallWintunParams struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// CoreStatusData 内核状态
type CoreStatusData struct {
	Running  bool   `json:"running"`           // 进程是否存活（优雅停止过程中仍然为 true）
	Stopping bool   `json:"stopping,omitzero"` // 是否正在优雅停止（CTRL_BREAK 已发出、等待退出）
	State    string `json:"state,omitempty"`   // "stopped" / "running" / "stopping"
	PID      int    `json:"pid,omitzero"`
	// ConsoleError 是 helper 控制台分配 / 校验失败原因。非空表示内核没有继承到 helper 的控制台，
	// CTRL_BREAK 无法送达，优雅停止会降级为强制结束。
	ConsoleError string `json:"consoleError,omitempty"`
	Error        string `json:"error,omitempty"`
}

// HelperStatusData helper 服务状态
type HelperStatusData struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
}
