//go:build windows

package sys

import (
	"encoding/json"
)

const (
	HelperServiceName = "GoclashZHelper"
	HelperDisplayName = "GoclashZ Helper Service"
	HelperDescription = "为 GoclashZ 提供高权限能力：TUN 启动、Wintun 安装、核心文件替换、权限修复"

	// Helper 监听地址 (仅本机)
	HelperAddr = "127.0.0.1:19720"

	// 简单共享密钥，防止其他程序误连
	HelperSecret = "GoclashZ-Helper-v1"
)

// HelperRequest 是 UI -> Helper 的请求
type HelperRequest struct {
	Secret string          `json:"secret"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// HelperResponse 是 Helper -> UI 的响应
type HelperResponse struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// StartCoreParams 启动内核的参数
type StartCoreParams struct {
	CorePath      string   `json:"corePath"`
	BinDir        string   `json:"binDir"`
	RuntimeConfig string   `json:"runtimeConfig"`
	Args          []string `json:"args"`
}

// StopCoreParams 停止内核的参数
type StopCoreParams struct {
	TargetExeName string `json:"targetExeName"`
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
	Running bool   `json:"running"`
	PID     int    `json:"pid,omitempty"`
	Error   string `json:"error,omitempty"`
}

// HelperStatusData helper 服务状态
type HelperStatusData struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
}
