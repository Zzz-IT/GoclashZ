package clash

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const apiUrl = "http://127.0.0.1:9090"

// ProxyNode 在线状态下传给前端的数据结构
type ProxyNode struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Now     string   `json:"now"`
	Proxies []string `json:"proxies"` // 对应 API 里的 "all" 字段 (所有可用节点)
}

// GetProxies 获取在线状态下的所有节点和策略组
func GetProxies() ([]ProxyNode, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(apiUrl + "/proxies")
	if err != nil {
		return nil, fmt.Errorf("无法连接内核 API: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]map[string]map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析节点数据失败: %v", err)
	}

	var nodes []ProxyNode
	proxies := result["proxies"]

	for name, info := range proxies {
		// ✨ 核心修复：屏蔽 Clash 内核强行注入的内置策略，让它和 yaml 保持绝对一致
		if name == "GLOBAL" || name == "DIRECT" || name == "REJECT" || name == "COMPATIBLE" {
			continue
		}

		nodeType := info["type"].(string)

		if nodeType == "Selector" || nodeType == "URLTest" || nodeType == "Fallback" {
			now := ""
			if nowVal, ok := info["now"]; ok {
				now = nowVal.(string)
			}

			var proxyList []string
			if all, ok := info["all"].([]interface{}); ok {
				for _, p := range all {
					if pStr, ok := p.(string); ok {
						proxyList = append(proxyList, pStr)
					}
				}
			}

			nodes = append(nodes, ProxyNode{
				Name:    name,
				Type:    nodeType,
				Now:     now,
				Proxies: proxyList,
			})
		}
	}
	return nodes, nil
}

// GetProxyDelay 获取指定节点的延迟 (Ping)
func GetProxyDelay(name string, testUrl string) (int, error) {
	// 1. 对节点名称进行转义，处理空格、表情符号等
	escapedName := url.PathEscape(name)
	// 2. 构造请求地址，设置超时和测试 URL
	// 默认 5000ms 超时
	fullUrl := fmt.Sprintf("%s/proxies/%s/delay?timeout=5000&url=%s", apiUrl, escapedName, url.QueryEscape(testUrl))

	resp, err := http.Get(fullUrl)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("测速失败，状态码: %d", resp.StatusCode)
	}

	// 解析返回结果 {"delay": 123}
	var result map[string]int
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	return result["delay"], nil
}

// SwitchProxy 切换策略组里的节点
// groupName 是策略组名称 (如 "GLOBAL" 或 "PROXIES")
// nodeName 是你要切换到的具体节点名称 (如 "香港 01")
func SwitchProxy(groupName, nodeName string) error {
	payload := map[string]string{"name": nodeName}
	jsonPayload, _ := json.Marshal(payload)

	req, err := http.NewRequest("PUT", apiUrl+"/proxies/"+groupName, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("切换节点失败，内核返回状态码: %d", resp.StatusCode)
	}

	fmt.Printf("✅ 成功将 [%s] 切换至节点: %s\n", groupName, nodeName)
	return nil
}

// 在 api_client.go 中增加此函数
func UpdateMode(mode string) error {
	// Clash 切换模式使用 PATCH /configs 接口
	payload := map[string]string{"mode": mode}
	jsonPayload, _ := json.Marshal(payload)

	req, err := http.NewRequest("PATCH", apiUrl+"/configs", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("切换模式失败: %d", resp.StatusCode)
	}
	return nil
}
