//go:build windows

package clash

import (
	jsonv2 "encoding/json/v2"
	"fmt"
	"net/http"
	"strings"
)

// 说明（F-A3-17）：这里原本还有一条基于 WebSocket 的 StartLogStream（level 硬编码 info），
// 与 appcore 实际使用的 FetchLogs（api_client.go，可配置级别 + 空闲重连）重复且无人调用，
// 已删除；日志流请统一走 core/appcore/logs.go → FetchLogs。

// PatchConfig 修改内核运行特性 (TUN, LAN, IPv6, LogLevel等)
func PatchConfig(settings map[string]interface{}) error {
	payload, err := jsonv2.Marshal(settings)
	if err != nil {
		return fmt.Errorf("配置序列化失败: %v", err)
	}

	// 使用动态 API 地址
	req, err := http.NewRequest("PATCH", APIURL("/configs"), strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("构建补丁请求失败: %v", err)
	}

	resp, err := localAPIClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("内核返回错误码: %d", resp.StatusCode)
	}
	return nil
}
