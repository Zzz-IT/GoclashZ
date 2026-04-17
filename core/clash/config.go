package clash

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// ClashConfig 映射完整的 YAML 结构
type ClashConfig struct {
	Mode        string                   `yaml:"mode"`
	ProxyGroups []map[string]interface{} `yaml:"proxy-groups"`
}

// OfflineGroup 专供前端在“未启动”状态下展示的节点组结构
type OfflineGroup struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Proxies []string `json:"proxies"` // 组内包含的所有节点名称
}

// RawProxyInfo 存储节点的物理地址信息，用于离线测速
// 注意：字段首字母必须大写，否则外部包无法访问，Wails 也无法序列化
type RawProxyInfo struct {
	Name   string `json:"name"`
	Server string `json:"server"`
	Port   string `json:"port"`
}

// GetStaticNodes 从本地 config.yaml 读取节点，用于启动前展示
func GetStaticNodes() (mode string, groups []OfflineGroup, err error) {
	pwd, _ := os.Getwd()
	configPath := filepath.Join(pwd, "core", "bin", "config.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return "rule", nil, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var conf ClashConfig
	err = yaml.Unmarshal(data, &conf)
	if err != nil {
		return "", nil, fmt.Errorf("YAML 格式错误: %v", err)
	}

	for _, g := range conf.ProxyGroups {
		name, _ := g["name"].(string)
		gType, _ := g["type"].(string)

		var proxyList []string
		if pList, ok := g["proxies"].([]interface{}); ok {
			for _, p := range pList {
				if pStr, ok := p.(string); ok {
					proxyList = append(proxyList, pStr)
				}
			}
		}

		// 过滤出常见的策略组类型
		if gType == "select" || gType == "url-test" || gType == "fallback" {
			groups = append(groups, OfflineGroup{
				Name:    name,
				Type:    gType,
				Proxies: proxyList,
			})
		}
	}

	mode = conf.Mode
	if mode == "" {
		mode = "rule"
	}

	return mode, groups, nil
}

// GetRawProxyAddrs 获取所有节点的物理地址映射列表
func GetRawProxyAddrs() ([]RawProxyInfo, error) {
	pwd, _ := os.Getwd()
	configPath := filepath.Join(pwd, "core", "bin", "config.yaml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var conf struct {
		Proxies []map[string]interface{} `yaml:"proxies"`
	}
	yaml.Unmarshal(data, &conf)

	var infos []RawProxyInfo
	for _, p := range conf.Proxies {
		name, _ := p["name"].(string)
		server, _ := p["server"].(string)
		port := fmt.Sprintf("%v", p["port"])
		if name != "" && server != "" {
			infos = append(infos, RawProxyInfo{Name: name, Server: server, Port: port})
		}
	}
	return infos, nil
}

// TCPPing 纯 Go 实现的 TCP 握手探测（离线测速）
func TCPPing(server string, port string) int {
	start := time.Now()
	address := net.JoinHostPort(server, port)

	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return -1
	}
	defer conn.Close()

	return int(time.Since(start).Milliseconds())
}

// DownloadSubscription 下载订阅文件并覆盖本地 config.yaml
func DownloadSubscription(subUrl string, userAgent string) error {
	pwd, _ := os.Getwd()
	configPath := filepath.Join(pwd, "core", "bin", "config.yaml")

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("GET", subUrl, nil)
	if err != nil {
		return err
	}

	if userAgent == "" {
		userAgent = "clash-verge/1.0"
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败: %s", resp.Status)
	}

	out, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
