package clash

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// 🚀 1. 定义全局共享的无代理 Transport，完美复用 TCP 底层连接
var noProxyTransport = &http.Transport{
	Proxy:               nil,
	MaxIdleConns:        100,              // 最大空闲连接数
	IdleConnTimeout:     90 * time.Second, // 空闲超时时间
	TLSHandshakeTimeout: 10 * time.Second,
}

// 🚀 2. 声明各场景的全局单例 Client
var localAPIClient = &http.Client{
	Transport: noProxyTransport,
	Timeout:   2 * time.Second,
}

var speedTestClient = &http.Client{
	Transport: noProxyTransport,
	Timeout:   6 * time.Second, // 测速专用超时
}

var streamClient = &http.Client{
	Transport: noProxyTransport, // 日志流/长连接专用，无超时
}

// FetchLogs 获取实时日志流并执行回调（带自动重连）
func FetchLogs(ctx context.Context, onLog func(data interface{})) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		req, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:9090/logs", nil)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		// 👇 核心修复：直接使用全局长连接客户端，绝不动态创建
		resp, err := streamClient.Do(req)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		decoder := json.NewDecoder(resp.Body)
		for {
			var logData map[string]interface{}
			if err := decoder.Decode(&logData); err != nil {
				// 发生 EOF 断开，关闭当前流，准备重连
				resp.Body.Close()
				break
			}
			onLog(logData)
		}
	}
}

// GetProxyDelay 调用内核 API 测试节点延迟
func GetProxyDelay(proxyName string, testUrl string) (int, error) {
	encodedName := url.PathEscape(proxyName)
	if testUrl == "" {
		testUrl = "http://www.gstatic.com/generate_204"
	}
	timeout := 5000

	apiURL := fmt.Sprintf("http://127.0.0.1:9090/proxies/%s/delay?timeout=%d&url=%s",
		encodedName, timeout, url.QueryEscape(testUrl))

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return -1, err
	}

	// 👇 核心修复：使用全局测速客户端，消除批量测速引发的 Goroutine 风暴
	resp, err := speedTestClient.Do(req)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return -1, fmt.Errorf("http error: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return -1, err
	}

	if delay, ok := result["delay"].(float64); ok {
		return int(delay), nil
	}

	return -1, fmt.Errorf("invalid delay format")
}

// GetInitialData 获取模式和代理组信息
func GetInitialData() (map[string]interface{}, error) {
	// 使用 localAPIClient 替换 http.Get
	respConfig, err := localAPIClient.Get("http://127.0.0.1:9090/configs")
	if err != nil {
		return nil, err
	}
	defer respConfig.Body.Close()

	var configData map[string]interface{}
	if err := json.NewDecoder(respConfig.Body).Decode(&configData); err != nil {
		return nil, err
	}

	respProxies, err := localAPIClient.Get("http://127.0.0.1:9090/proxies")
	if err != nil {
		return nil, err
	}
	defer respProxies.Body.Close()

	var proxiesData map[string]interface{}
	if err := json.NewDecoder(respProxies.Body).Decode(&proxiesData); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"mode":   configData["mode"],
		"groups": proxiesData["proxies"],
	}, nil
}

// UpdateMode 切换代理模式
func UpdateMode(mode string) error {
	req, err := http.NewRequest("PATCH", "http://127.0.0.1:9090/configs", bytes.NewBuffer([]byte(`{"mode":"`+mode+`"}`)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := localAPIClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// SelectProxy 切换代理节点
func SelectProxy(groupName, proxyName string) error {
	encodedGroup := url.PathEscape(groupName)
	body := map[string]string{"name": proxyName}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequest("PUT", "http://127.0.0.1:9090/proxies/"+encodedGroup, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := localAPIClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}



// GetConnectionsRaw 获取实时连接原始数据
func GetConnectionsRaw() ([]byte, error) {
	resp, err := localAPIClient.Get("http://127.0.0.1:9090/connections")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	_, err = buf.ReadFrom(resp.Body)
	return buf.Bytes(), err
}

// CloseConnection 断开指定的单个连接
func CloseConnection(id string) error {
	req, err := http.NewRequest(http.MethodDelete, "http://127.0.0.1:9090/connections/"+id, nil)
	if err != nil {
		return err
	}
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
	resp, err := localAPIClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// GetVersion 获取内核版本号
func GetVersion() string {
	resp, err := localAPIClient.Get("http://127.0.0.1:9090/version")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var data map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return ""
	}
	return data["version"]
}
