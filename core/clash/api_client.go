package clash

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// FetchLogs 从内核获取实时日志流并推送至前端
func FetchLogs(ctx context.Context) {
	resp, err := http.Get("http://127.0.0.1:9090/logs")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
			var logData interface{}
			if err := json.Unmarshal(scanner.Bytes(), &logData); err == nil {
				runtime.EventsEmit(ctx, "log-message", logData)
			}
		}
	}
}

// GetProxyDelay 调用内核 API 测试节点真连接延迟
func GetProxyDelay(nodeName string) (int, error) {
	encodedName := url.PathEscape(nodeName)
	apiURL := fmt.Sprintf("http://127.0.0.1:9090/proxies/%s/delay?timeout=5000&url=http://www.gstatic.com/generate_204", encodedName)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Get(apiURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("timeout")
	}

	var result struct {
		Delay int `json:"delay"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}
	return result.Delay, nil
}

// GetInitialData 获取模式和代理组信息
func GetInitialData() (map[string]interface{}, error) {
	// 获取基础配置 (Mode)
	respConfig, err := http.Get("http://127.0.0.1:9090/configs")
	if err != nil {
		return nil, err
	}
	defer respConfig.Body.Close()

	var config map[string]interface{}
	// 修正：这里使用 respConfig.Body
	if err := json.NewDecoder(respConfig.Body).Decode(&config); err != nil {
		return nil, err
	}

	// 获取代理列表 (Groups)
	respProxies, err := http.Get("http://127.0.0.1:9090/proxies")
	if err != nil {
		return nil, err
	}
	defer respProxies.Body.Close()

	var proxies map[string]interface{}
	// 修正：这里使用 respProxies.Body，而不是直接用 respProxies
	if err := json.NewDecoder(respProxies.Body).Decode(&proxies); err != nil {
		return nil, err
	}

	// 整合数据
	return map[string]interface{}{
		"mode":   config["mode"],
		"groups": proxies["proxies"],
	}, nil
}

// UpdateMode 切换 Clash 路由模式 (rule, global, direct)
func UpdateMode(mode string) error {
	// 构造请求体
	body := map[string]string{"mode": mode}
	jsonBody, _ := json.Marshal(body)

	// Mihomo/Clash API: PATCH /configs
	req, err := http.NewRequest(http.MethodPatch, "http://127.0.0.1:9090/configs", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("内核返回异常状态码: %d", resp.StatusCode)
	}

	return nil
}

// SwitchProxy 切换特定策略组的节点
func SwitchProxy(groupName, nodeName string) error {
	// 构造请求体
	body := map[string]string{"name": nodeName}
	jsonBody, _ := json.Marshal(body)

	// Mihomo/Clash API: PUT /proxies/{groupName}
	// 注意：groupName 必须进行 URL 编码
	apiURL := fmt.Sprintf("http://127.0.0.1:9090/proxies/%s", url.PathEscape(groupName))

	req, err := http.NewRequest(http.MethodPut, apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("内核切换节点失败: %d", resp.StatusCode)
	}

	return nil
}
