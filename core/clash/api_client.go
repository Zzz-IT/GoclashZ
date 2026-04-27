package clash

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 🚀 1. 定义全局共享的无代理 Transport，加入 TCP 探活机制
var noProxyTransport = &http.Transport{
	Proxy: nil,
	// 👇 核心修复：强制 TCP 层面每 15 秒探活一次，防止假死连接让解码器永久阻塞
	DialContext: (&net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 15 * time.Second,
	}).DialContext,
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

// 🚀 3. 统一 API 基准地址管理，彻底消灭 127.0.0.1:9090 硬编码
var apiBase = struct {
	sync.RWMutex
	value string
}{
	value: "http://127.0.0.1:9090", // 默认兜底
}

func NormalizeControllerHostPort(controller string) string {
	controller = strings.TrimSpace(controller)
	if controller == "" {
		return "127.0.0.1:9090"
	}
	// 用户只填端口，比如 9091
	if _, err := strconv.Atoi(controller); err == nil {
		return "127.0.0.1:" + controller
	}
	// 用户误填 URL，比如 http://127.0.0.1:9090
	if strings.Contains(controller, "://") {
		u, err := url.Parse(controller)
		if err == nil && u.Host != "" {
			controller = u.Host
		}
	}
	host, port, err := net.SplitHostPort(controller)
	if err != nil || host == "" || port == "" {
		return "127.0.0.1:9090"
	}
	// external-controller 必须限制在本机，避免管理 API 被局域网暴露
	if host != "127.0.0.1" && host != "localhost" && host != "::1" && host != "[::1]" {
		return "127.0.0.1:9090"
	}
	return net.JoinHostPort(strings.Trim(host, "[]"), port)
}

func normalizeAPIBaseURL(controller string) string {
	controller = NormalizeControllerHostPort(controller)

	// 补全协议头
	if !strings.HasPrefix(controller, "http://") && !strings.HasPrefix(controller, "https://") {
		controller = "http://" + controller
	}

	u, err := url.Parse(controller)
	if err != nil || u.Host == "" {
		return "http://127.0.0.1:9090"
	}

	// 规格化：只保留协议、主机和端口
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""

	return strings.TrimRight(u.String(), "/")
}

// UpdateAPIBaseURL 动态更新内核控制接口的基础地址
func UpdateAPIBaseURL(controller string) {
	apiBase.Lock()
	apiBase.value = normalizeAPIBaseURL(controller)
	apiBase.Unlock()
}

// APIURL 构造完整的 HTTP 请求地址
func APIURL(path string) string {
	apiBase.RLock()
	base := apiBase.value
	apiBase.RUnlock()

	if base == "" {
		base = "http://127.0.0.1:9090"
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	return base + path
}

// APIWSURL 构造完整的 WebSocket 请求地址
func APIWSURL(path string) string {
	apiBase.RLock()
	base := apiBase.value
	apiBase.RUnlock()

	if base == "" {
		base = "http://127.0.0.1:9090"
	}

	u, err := url.Parse(base)
	if err != nil {
		return "ws://127.0.0.1:9090" + path
	}

	// 协议转换
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	u.Path = path
	u.RawQuery = ""

	return u.String()
}

// APIWSURLWithRawQuery 构造带参数的 WebSocket 请求地址
func APIWSURLWithRawQuery(path string, rawQuery string) string {
	u, _ := url.Parse(APIWSURL(path))
	u.RawQuery = rawQuery
	return u.String()
}

// FetchLogs 获取实时日志流并执行回调（带自动重连）
func FetchLogs(ctx context.Context, level string, onLog func(data interface{})) {
	if level == "" {
		level = "info" // 兜底默认值
	}

	for {
		// 快速响应外部的 Cancel 信号
		select {
		case <-ctx.Done():
			return
		default:
		}

		apiURL := APIURL("/logs?level=" + url.QueryEscape(level))
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			// 👇 核心修复 1：使用显式的 Timer 替代 time.After
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop() // 👈 手动释放内存
				return
			case <-timer.C:
			}
			continue
		}

		resp, err := streamClient.Do(req)
		if err != nil {
			// 👇 核心修复 2：使用显式的 Timer 替代 time.After
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop() // 👈 手动释放内存
				return
			case <-timer.C:
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			continue
		}

		const logIdleTimeout = 70 * time.Second
		decoder := json.NewDecoder(resp.Body)
		idleTimer := time.NewTimer(logIdleTimeout)
		stopIdle := make(chan struct{})

		// 🚀 新增：Idle Watchdog 协程
		go func() {
			select {
			case <-ctx.Done():
				resp.Body.Close()
			case <-idleTimer.C:
				// 超过 70 秒没有收到任何日志，主动断开以触发重连
				resp.Body.Close()
			case <-stopIdle:
				// 循环结束，停止监控
			}
		}()

		for {
			var logData map[string]interface{}
			if err := decoder.Decode(&logData); err != nil {
				close(stopIdle)
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				resp.Body.Close()
				break
			}

			// 收到数据，重置探活计时器
			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C:
				default:
				}
			}
			idleTimer.Reset(logIdleTimeout)

			onLog(logData)
		}
	}
}

// GetProxyDelay 调用内核 API 测试节点延迟
func GetProxyDelay(ctx context.Context, proxyName string, testUrl string) (int, error) {
	encodedName := url.PathEscape(proxyName)
	if testUrl == "" {
		testUrl = "http://www.gstatic.com/generate_204"
	}
	timeout := 5000

	apiURL := fmt.Sprintf("%s?timeout=%d&url=%s",
		APIURL("/proxies/"+encodedName+"/delay"),
		timeout,
		url.QueryEscape(testUrl))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
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

func doKernelRequest(method, path string, body any, okStatus ...int) error {
	var reader io.Reader

	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, APIURL(path), reader)
	if err != nil {
		return err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := localAPIClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	for _, status := range okStatus {
		if resp.StatusCode == status {
			return nil
		}
	}

	return fmt.Errorf("内核 API 调用失败: %s %s -> HTTP %d", method, path, resp.StatusCode)
}

// GetInitialData 获取模式和代理组信息
func GetInitialData() (map[string]interface{}, error) {
	// 使用 localAPIClient 替换 http.Get
	respConfig, err := localAPIClient.Get(APIURL("/configs"))
	if err != nil {
		return nil, err
	}
	defer respConfig.Body.Close()

	var configData map[string]interface{}
	if err := json.NewDecoder(respConfig.Body).Decode(&configData); err != nil {
		return nil, err
	}

	respProxies, err := localAPIClient.Get(APIURL("/proxies"))
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
	return doKernelRequest(
		http.MethodPatch,
		"/configs",
		map[string]string{"mode": mode},
		http.StatusOK,
		http.StatusNoContent,
	)
}

// SelectProxy 切换代理节点
func SelectProxy(groupName, proxyName string) error {
	return doKernelRequest(
		http.MethodPut,
		"/proxies/"+url.PathEscape(groupName),
		map[string]string{"name": proxyName},
		http.StatusOK,
		http.StatusNoContent,
	)
}

// GetConnectionsRaw 获取实时连接原始数据
func GetConnectionsRaw() ([]byte, error) {
	resp, err := localAPIClient.Get(APIURL("/connections"))
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
	return doKernelRequest(
		http.MethodDelete,
		"/connections/"+url.PathEscape(id),
		nil,
		http.StatusOK,
		http.StatusNoContent,
	)
}

// CloseAllConnections 断开所有活动连接
func CloseAllConnections() error {
	return doKernelRequest(
		http.MethodDelete,
		"/connections",
		nil,
		http.StatusOK,
		http.StatusNoContent,
	)
}

// GetVersion 获取内核版本号
func GetVersion() string {
	resp, err := localAPIClient.Get(APIURL("/version"))
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

// FlushFakeIP 刷新 Fake-IP 缓存（带前置条件检查）
func FlushFakeIP() error {
	// 1. 读取当前的 DNS 配置，判断是否是 Fake-IP 模式
	dnsCfg, err := GetDNSConfig()
	if err != nil || dnsCfg == nil {
		// 🚀 核心修复：不再隐瞒，直接抛出，让前端弹窗提示内核异常
		return fmt.Errorf("通信失败：无法获取当前 DNS 状态，请检查内核是否运行") 
	}

	// 2. 如果根本不是 Fake-IP 模式，直接返回，不打扰内核
	if dnsCfg.EnhancedMode != "fake-ip" {
		// 🚀 核心修复：告知前端当前环境不需要清理
		return fmt.Errorf("当前为 %s 模式，仅 Fake-IP 模式支持刷新缓存", dnsCfg.EnhancedMode)
	}

	// 3. 只有确认是 Fake-IP 模式，才真正向内核发送清理请求
	req, _ := http.NewRequest("POST", APIURL("/cache/fakeip/flush"), nil)
	resp, err := localAPIClient.Do(req)
	if err != nil {
		return fmt.Errorf("清理指令未送达内核: %v", err)
	}
	defer resp.Body.Close()

	// 检查内核是否真的处理成功了
	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return fmt.Errorf("内核拒绝了清理请求，状态码: %d", resp.StatusCode)
	}

	return nil
}
