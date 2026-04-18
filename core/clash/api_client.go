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
	// ⚠️ 核心修复：使用带有 Context 的请求，当 ctx 被取消时，网络请求立刻中断，解除 scanner.Scan() 的阻塞
	req, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:9090/logs", nil)
	if err != nil {
		return
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var logData interface{}
		if err := json.Unmarshal(scanner.Bytes(), &logData); err == nil {
			runtime.EventsEmit(ctx, "log-message", logData)
		}
	}
}

// GetProxyDelay 调用内核 API 测试真实延迟
func GetProxyDelay(proxyName string) (int, error) {
	// ⚠️ 极其关键：必须对节点名进行 URL 编码，防止空格 and 特殊符号导致 400 Bad Request
	encodedName := url.PathEscape(proxyName)

	// 测速目标和超时设置 (5000ms)
	testUrl := "http://www.gstatic.com/generate_204"
	timeout := 5000

	// 👈 核心修复：必须对 testUrl 进行 QueryEscape 编码
	apiURL := fmt.Sprintf("http://127.0.0.1:9090/proxies/%s/delay?timeout=%d&url=%s",
		encodedName, timeout, url.QueryEscape(testUrl))

	// HTTP 客户端的超时应略大于内核传入的 timeout
	client := &http.Client{
		Timeout: 6 * time.Second,
	}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return -1, err
	}

	// 如果你的 API 配置了 secret，记得取消下面这行的注释：
	// req.Header.Set("Authorization", "Bearer YOUR_SECRET")

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

	// 内核有时超时会返回 0，统一转为 -1
	if result.Delay == 0 {
		return -1, nil
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
