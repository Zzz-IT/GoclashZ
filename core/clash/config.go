package clash

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings" // 👈 确保引入 strings
	"time"

	"gopkg.in/yaml.v3"
)

// getConfigPath 获取 config.yaml 的绝对路径
func getConfigPath() string {
	exePath, err := os.Executable()
	if err != nil {
		return filepath.Join("core", "bin", "config.yaml")
	}
	return filepath.Join(filepath.Dir(exePath), "core", "bin", "config.yaml")
}

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

// ProxyGroupSchema 模拟内核 API 的 Group 结构
type ProxyGroupSchema struct {
	Name string   `json:"name"`
	Type string   `json:"type"`
	Now  string   `json:"now"` // 当前选中节点
	All  []string `json:"all"` // 包含的所有节点名
}

// GetOfflineData 核心方法：模拟内核 API 返回的格式，从本地文件中提取数据
func GetOfflineData(fileName string) (map[string]interface{}, error) {
	// 1. 获取路径
	exePath, _ := os.Executable()
	baseDir := filepath.Dir(exePath)

	// 如果传入了空或者 config.yaml，直接指向主配置
	if fileName == "" || fileName == "config.yaml" {
		fileName = "config.yaml"
	}
	configPath := filepath.Join(baseDir, "core", "bin", fileName)

	// 如果文件不存在，回退到主 config.yaml
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = filepath.Join(baseDir, "core", "bin", "config.yaml")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var conf struct {
		Mode        string                   `yaml:"mode"`
		ProxyGroups []map[string]interface{} `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal(data, &conf); err != nil {
		return nil, err
	}

	// 2. 构造与内核 API 完全一致的 Map 结构
	// 内核返回的 proxies 字段是一个 Map，Key 是组名
	proxiesMap := make(map[string]interface{})

	for _, g := range conf.ProxyGroups {
		name, _ := g["name"].(string)
		gTypeRaw, _ := g["type"].(string) // 改个名字，获取原始 type

		// 👇 新增类型转换，将其翻译为前端认识的 API 标准名称
		gType := gTypeRaw
		switch gTypeRaw {
		case "select":
			gType = "Selector"
		case "url-test":
			gType = "URLTest"
		case "fallback":
			gType = "Fallback"
		case "load-balance":
			gType = "LoadBalance"
		}

		var all []string
		if pList, ok := g["proxies"].([]interface{}); ok {
			for _, p := range pList {
				if s, ok := p.(string); ok {
					all = append(all, s)
				}
			}
		}

		// 模拟内核的单组数据结构
		proxiesMap[name] = map[string]interface{}{
			"name": name,
			"type": gType,
			"now":  "", // 离线状态下没有当前选中项
			"all":  all,
		}
	}

	return map[string]interface{}{
		"mode":   conf.Mode,
		"groups": proxiesMap, // 这里的格式将完美契合前端 Proxies.vue 的逻辑
	}, nil
}

// GetRawProxyAddrs 获取所有节点的物理地址映射列表
func GetRawProxyAddrs(fileName string) ([]RawProxyInfo, error) {
	// 获取正确的运行目录路径
	exePath, _ := os.Executable()
	baseDir := filepath.Dir(exePath)

	if fileName == "" || fileName == "config.yaml" {
		fileName = "config.yaml"
	}
	configPath := filepath.Join(baseDir, "core", "bin", fileName)

	// 如果指定的文件不存在，回退到主配置
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = filepath.Join(baseDir, "core", "bin", "config.yaml")
	}

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
		port := fmt.Sprintf("%v", p["port"]) // 兼容 int 和 string 格式的端口
		if name != "" && server != "" {
			infos = append(infos, RawProxyInfo{Name: name, Server: server, Port: port})
		}
	}
	return infos, nil
}

// resolveWithCustomDNS 读取用户的 DNS 配置，并进行离线域名解析
func resolveWithCustomDNS(domain string) string {
	dnsCfg, err := GetDNSConfig()
	dnsServer := "223.5.5.5:53" // 默认保底 DNS

	if err == nil && dnsCfg != nil {
		// 按照优先级组装配置里的 DNS 列表，优先取 proxy-server-nameserver
		list := append(dnsCfg.ProxyServerNameserver, dnsCfg.Nameserver...)
		list = append(list, dnsCfg.DefaultNameserver...)

		for _, ns := range list {
			// 简单清洗格式，试图从中提取出一个标准的 IP。
			// 例如处理：https://223.5.5.5/dns-query -> 223.5.5.5
			clean := strings.TrimPrefix(ns, "https://")
			clean = strings.TrimPrefix(clean, "tls://")
			clean = strings.TrimPrefix(clean, "tcp://")
			clean = strings.TrimPrefix(clean, "udp://")

			parts := strings.Split(clean, "/")
			hostPort := parts[0]
			host, _, err := net.SplitHostPort(hostPort)
			if err != nil {
				host = hostPort // 如果没有写端口的情况
			}

			// 如果能成功解析为 IP，说明找到了可用的自建/配置 DNS 地址
			if net.ParseIP(host) != nil {
				dnsServer = net.JoinHostPort(host, "53")
				break
			}
		}
	}

	// 强制构建一个走特定 DNS 的底层 Resolver
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 2 * time.Second}
			return d.DialContext(ctx, "udp", dnsServer)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ips, err := r.LookupIPAddr(ctx, domain)
	if err == nil && len(ips) > 0 {
		return ips[0].IP.String() // 返回解析到的首个节点真实 IP
	}
	return ""
}

// TCPPing 纯 Go 实现的 TCP 握手探测（已接入自定义 DNS）
func TCPPing(server string, port string) int {
	ip := server

	// 如果传入的 server 是一个域名而不是纯 IP，则走我们的自定义解析器
	if net.ParseIP(server) == nil {
		resolvedIP := resolveWithCustomDNS(server)
		if resolvedIP != "" {
			ip = resolvedIP
		}
		// 如果解析失败，这里也会保持原来的 domain，交由系统网络兜底尝试
	}

	start := time.Now()
	address := net.JoinHostPort(ip, port)

	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return -1
	}
	defer conn.Close()

	return int(time.Since(start).Milliseconds())
}

// DownloadSubscription 下载订阅文件并覆盖本地 config.yaml
func DownloadSubscription(subUrl string, userAgent string) error {
	configPath := getConfigPath() // 👈 使用绝对路径

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
	configPath := getConfigPath() // 👈 使用绝对路径

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
	configPath := getConfigPath() // 👈 使用绝对路径

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
	configPath := getConfigPath() // 👈 使用绝对路径

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
	configPath := getConfigPath() // 👈 使用绝对路径

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
