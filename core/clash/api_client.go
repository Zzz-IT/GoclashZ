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
)

// 关键修复：创建一个全局的、强制不走系统代理的 HTTP 客户端
var localAPIClient = &http.Client{
	Transport: &http.Transport{
		Proxy: nil, // 强制禁用代理，防止本地 9090 请求被 7890 系统代理劫持
	},
	Timeout: 10 * time.Second,
}

// FetchLogs 从内核获取实时日志流并执行回调
func FetchLogs(ctx context.Context, onLog func(data interface{})) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:9090/logs", nil)
	if err != nil {
		return
	}

	// 单独的 client 也需要禁用代理，并且不要设置超时以保持长连接
	client := &http.Client{
		Transport: &http.Transport{Proxy: nil}, 
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var logData interface{}
		if err := json.Unmarshal(scanner.Bytes(), &logData); err == nil {
			onLog(logData)
		}
	}
}

// GetProxyDelay 调用内核 API 测试真实延迟
func GetProxyDelay(proxyName string) (int, error) {
	encodedName := url.PathEscape(proxyName)
	testUrl := "http://www.gstatic.com/generate_204"
	timeout := 5000

	apiURL := fmt.Sprintf("http://127.0.0.1:9090/proxies/%s/delay?timeout=%d&url=%s",
		encodedName, timeout, url.QueryEscape(testUrl))

	// 测速 client 必须禁用代理
	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
		Timeout:   6 * time.Second,
	}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return -1, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return -1, fmt.Errorf("测速请求失败，状态码: %d", resp.StatusCode)
	}

	var result struct {
		Delay int `json:"delay"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return -1, err
	}

	if result.Delay == 0 {
		return -1, nil
	}

	return result.Delay, nil
}

// GetInitialData 获取模式和代理组信息
func GetInitialData() (map[string]interface{}, error) {
	// 使用 localAPIClient 替换 http.Get
	respConfig, err := localAPIClient.Get("http://127.0.0.1:9090/configs")
	if err != nil {
		return nil, err
	}
	defer respConfig.Body.Close()

	var config map[string]interface{}
	if err := json.NewDecoder(respConfig.Body).Decode(&config); err != nil {
		return nil, err
	}

	// 使用 localAPIClient 替换 http.Get
	respProxies, err := localAPIClient.Get("http://127.0.0.1:9090/proxies")
	if err != nil {
		return nil, err
	}
	defer respProxies.Body.Close()

	var proxies map[string]interface{}
	if err := json.NewDecoder(respProxies.Body).Decode(&proxies); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"mode":   config["mode"],
		"groups": proxies["proxies"],
	}, nil
}

// UpdateMode 切换 Clash 路由模式 (rule, global, direct)
func UpdateMode(mode string) error {
	body := map[string]string{"mode": mode}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequest(http.MethodPatch, "http://127.0.0.1:9090/configs", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	// 使用 localAPIClient
	resp, err := localAPIClient.Do(req)
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
	body := map[string]string{"name": nodeName}
	jsonBody, _ := json.Marshal(body)

	apiURL := fmt.Sprintf("http://127.0.0.1:9090/proxies/%s", url.PathEscape(groupName))

	req, err := http.NewRequest(http.MethodPut, apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	// 使用 localAPIClient
	resp, err := localAPIClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("内核切换节点失败: %d", resp.StatusCode)
	}

	return nil
}

// GetConnections 获取当前所有活跃连接
func GetConnections() (map[string]interface{}, error) {
	// 使用 localAPIClient 替换 http.Get
	resp, err := localAPIClient.Get("http://127.0.0.1:9090/connections")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}

// CloseConnection 断开指定的单个连接
func CloseConnection(id string) error {
	req, err := http.NewRequest(http.MethodDelete, "http://127.0.0.1:9090/connections/"+id, nil)
	if err != nil {
		return err
	}
	// 使用 localAPIClient
	resp, err := localAPIClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// CloseAllConnections 断开所有活动连接
func CloseAllConnections() error {
	req, err := http.NewRequest(http.MethodDelete, "http://127.0.0.1:9090/connections", nil)
	if err != nil {
		return err
	}
	// 使用 localAPIClient
	resp, err := localAPIClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
