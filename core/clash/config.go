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

// TunConfig 映射 yaml 中的 tun 配置块
type TunConfig struct {
	Enable              bool     `yaml:"enable" json:"enable"`
	Stack               string   `yaml:"stack" json:"stack"`
	Device              string   `yaml:"device,omitempty" json:"device"`
	AutoRoute           bool     `yaml:"auto-route" json:"autoRoute"`
	AutoDetectInterface bool     `yaml:"auto-detect-interface" json:"autoDetect"`
	DNSHijack           []string `yaml:"dns-hijack" json:"dnsHijack"`
	StrictRoute         bool     `yaml:"strict-route" json:"strictRoute"`
	MTU                 int      `yaml:"mtu" json:"mtu"`
}

// GetTunConfig 从 config.yaml 读取 TUN 配置
func GetTunConfig() (*TunConfig, error) {
	pwd, _ := os.Getwd()
	configPath := filepath.Join(pwd, "core", "bin", "config.yaml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	// 初始化默认值
	conf := &TunConfig{
		Enable:              false,
		Stack:               "gvisor",
		Device:              "",
		AutoRoute:           true,
		AutoDetectInterface: true,
		DNSHijack:           []string{"any:53"},
		StrictRoute:         true,
		MTU:                 1500,
	}

	if tunMap, ok := root["tun"].(map[string]interface{}); ok {
		raw, _ := yaml.Marshal(tunMap)
		yaml.Unmarshal(raw, conf)
	}

	return conf, nil
}

// UpdateTunConfig 将新的 TUN 配置写入 config.yaml
func UpdateTunConfig(newTun *TunConfig) error {
	pwd, _ := os.Getwd()
	configPath := filepath.Join(pwd, "core", "bin", "config.yaml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}

	root["tun"] = newTun

	// TUN 模式通常依赖 DNS 拦截
	if newTun.Enable {
		if _, ok := root["dns"]; !ok {
			root["dns"] = map[string]interface{}{
				"enable":        true,
				"enhanced-mode": "fake-ip",
				"nameserver":    []string{"119.29.29.29", "223.5.5.5"},
			}
		}
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, out, 0644)
}

// -------------------- DNS 配置相关 --------------------

// FallbackFilterConfig 映射 yaml 中 dns.fallback-filter 配置块
type FallbackFilterConfig struct {
	GeoIP     bool     `yaml:"geoip" json:"geoip"`
	GeoIPCode string   `yaml:"geoip-code" json:"geoipCode"`
	IPCIDR    []string `yaml:"ipcidr" json:"ipcidr"`
	Domain    []string `yaml:"domain,omitempty" json:"domain"` // 截图虽然没有明确展示domain，但这是clash的常见配置
}

// DNSConfig 映射 yaml 中的 dns 配置块
type DNSConfig struct {
	Enable                bool                 `yaml:"enable" json:"enable"`
	IPv6                  bool                 `yaml:"ipv6" json:"ipv6"`
	EnhancedMode          string               `yaml:"enhanced-mode" json:"enhancedMode"`
	FakeIPRange           string               `yaml:"fake-ip-range,omitempty" json:"fakeIpRange"`
	FakeIPFilter          []string             `yaml:"fake-ip-filter,omitempty" json:"fakeIpFilter"`
	UseSystemHosts        bool                 `yaml:"use-system-hosts,omitempty" json:"useSystemHosts"`
	UseHosts              bool                 `yaml:"use-hosts,omitempty" json:"useHosts"`
	DefaultNameserver     []string             `yaml:"default-nameserver,omitempty" json:"defaultNameserver"`
	Nameserver            []string             `yaml:"nameserver" json:"nameserver"`
	Fallback              []string             `yaml:"fallback,omitempty" json:"fallback"`
	FallbackFilter        FallbackFilterConfig `yaml:"fallback-filter" json:"fallbackFilter"` // 新增
	NameserverPolicy      map[string]string    `yaml:"nameserver-policy,omitempty" json:"nameserverPolicy"`
	ProxyServerNameserver []string             `yaml:"proxy-server-nameserver,omitempty" json:"proxyServerNameserver"`
}

// GetDNSConfig 读取 DNS 配置
func GetDNSConfig() (*DNSConfig, error) {
	pwd, _ := os.Getwd()
	configPath := filepath.Join(pwd, "core", "bin", "config.yaml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	// 初始化默认值
	conf := &DNSConfig{
		Enable:            true,
		IPv6:              false,
		EnhancedMode:      "fake-ip",
		FakeIPRange:       "198.18.0.1/16",
		FakeIPFilter:      []string{"*.lan", "*.localdomain", "*.example", "*.invalid", "*.localhost", "*.test", "lan", "localdomain", "localhost"},
		UseSystemHosts:    true,
		UseHosts:          true,
		DefaultNameserver: []string{"223.5.5.5", "114.114.114.114"},
		Nameserver:        []string{"https://doh.pub/dns-query", "https://dns.alidns.com/dns-query"},
		Fallback:          []string{"https://doh.dns.sb/dns-query", "https://dns.cloudflare.com/dns-query"},
		FallbackFilter: FallbackFilterConfig{
			GeoIP:     true,
			GeoIPCode: "CN",
			IPCIDR:    []string{"240.0.0.0/4", "0.0.0.0/32"},
		},
		NameserverPolicy:      map[string]string{"geosite:cn": "https://doh.pub/dns-query"},
		ProxyServerNameserver: []string{"https://doh.pub/dns-query"},
	}

	if dnsMap, ok := root["dns"].(map[string]interface{}); ok {
		raw, _ := yaml.Marshal(dnsMap)
		yaml.Unmarshal(raw, conf)

		// 兼容部分内核写法
		if fakeRange, ok := dnsMap["fake-ip-range"].(string); ok {
			conf.FakeIPRange = fakeRange
		}
	}

	return conf, nil
}

// UpdateDNSConfig 写入 DNS 配置
func UpdateDNSConfig(newDNS *DNSConfig) error {
	pwd, _ := os.Getwd()
	configPath := filepath.Join(pwd, "core", "bin", "config.yaml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}

	// 转换为 map 进行精确控制
	dnsMap := map[string]interface{}{
		"enable":                  newDNS.Enable,
		"ipv6":                    newDNS.IPv6,
		"enhanced-mode":           newDNS.EnhancedMode,
		"fake-ip-range":           newDNS.FakeIPRange,
		"fake-ip-filter":          newDNS.FakeIPFilter,
		"use-system-hosts":        newDNS.UseSystemHosts,
		"use-hosts":               newDNS.UseHosts,
		"default-nameserver":      newDNS.DefaultNameserver,
		"nameserver":              newDNS.Nameserver,
		"fallback":                newDNS.Fallback,
		"fallback-filter":         newDNS.FallbackFilter, // 新增
		"nameserver-policy":       newDNS.NameserverPolicy,
		"proxy-server-nameserver": newDNS.ProxyServerNameserver,
	}

	root["dns"] = dnsMap

	out, err := yaml.Marshal(root)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, out, 0644)
}
