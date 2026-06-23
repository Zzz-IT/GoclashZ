//go:build windows

package sys

import (
	"encoding/json"
	"fmt"
	"os/user"
	"strings"
)

const (
	HelperServiceName = "GoclashZHelper"
	HelperDisplayName = "GoclashZ Helper Service"
	HelperDescription = "为 GoclashZ 提供高权限能力：TUN 启动、Wintun 安装、核心文件替换、权限修复"
)

// HelperRequest 是 UI -> Helper 的请求
type HelperRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// HelperResponse 是 Helper -> UI 的响应
type HelperResponse struct {
	OK     bool            `json:"ok"`
	Data   json.RawMessage `json:"data,omitempty"`
	Error  string          `json:"error,omitempty"`
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

// GetHelperPipeName 返回当前用户的 named pipe 路径
func GetHelperPipeName() string {
	sid := getUserSID()
	if sid == "" {
		return `\\.\pipe\GoclashZHelper`
	}
	return `\\.\pipe\GoclashZHelper-` + sid
}

func getUserSID() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(u.Uid)
}

// validatePipeName 防御性检查 pipe 名称
func validatePipeName(name string) error {
	if !strings.HasPrefix(name, `\\.\pipe\`) {
		return fmt.Errorf("invalid pipe name: %s", name)
	}
	if len(name) > 256 {
		return fmt.Errorf("pipe name too long: %d", len(name))
	}
	return nil
}
