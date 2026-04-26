package clash

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"goclashz/core/utils"
	"sync"

	"gopkg.in/yaml.v3"
)

// 👈 新增：定义一个全局互斥锁，专门保护 config.yaml 的并发 RMW (读-改-写) 操作
var configMu sync.Mutex

// GetConfigPath 获取 config.yaml 的绝对路径（导出供 app.go 使用，确保路径一致）
func GetConfigPath() string {
	return filepath.Join(utils.GetDataDir(), "config.yaml")
}

// ClashConfig 映射完整的 YAML 结构
type ClashConfig struct {
	Mode        string                   `yaml:"mode"`
	ProxyGroups []map[string]interface{} `yaml:"proxy-groups"`
}

// NetworkConfig 基础网络配置
type NetworkConfig struct {
	Port                 int    `yaml:"port" json:"port"`
	MixedPort            int    `yaml:"mixed-port" json:"mixedPort"`
	IPv6                 bool   `yaml:"ipv6" json:"ipv6"`
	UnifiedDelay         bool   `yaml:"unified-delay" json:"unifiedDelay"`
	DelayRetention       bool   `yaml:"delay-retention" json:"delayRetention"`
	DelayRetentionTime   string `yaml:"delay-retention-time" json:"delayRetentionTime"`
	TCPConcurrent        bool   `yaml:"tcp-concurrent" json:"tcpConcurrent"`
	TCPKeepAlive         bool   `yaml:"tcp-keep-alive" json:"tcpKeepAlive"`
	TCPKeepAliveInterval int    `yaml:"tcp-keep-alive-interval" json:"tcpKeepAliveInterval"`
	TestURL              string `yaml:"test-url" json:"testUrl"` // 👈 新增
	Hosts                string `yaml:"-" json:"hosts"`          // 👈 新增：作为字符串传递给前端
}

// OfflineGroup 专供前端在“未启动”状态下展示的节点组结构
type OfflineGroup struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Proxies []string `json:"proxies"` // 组内包含的所有节点名称
}

// ProxyGroupSchema 模拟内核 API 的 Group 结构
type ProxyGroupSchema struct {
	Name string   `json:"name"`
	Type string   `json:"type"`
	Now  string   `json:"now"` // 当前选中节点
	All  []string `json:"all"` // 包含的所有节点名
}

// GetOfflineData 核心方法：模拟内核 API 返回的格式，从本地文件中提取数据
func GetOfflineData(id string) (map[string]interface{}, error) {
	// 如果传入了空或者 config.yaml，直接指向主配置
	if id == "" || id == "config.yaml" {
		id = "config.yaml"
	}

	var configPath string
	if id == "config.yaml" {
		configPath = GetConfigPath()
	} else {
		configPath = filepath.Join(utils.GetSubscriptionsDir(), id+".yaml")
	}

	// 如果文件不存在，回退到主 config.yaml
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = GetConfigPath()
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	// ⭐️ 核心修复 1：结构体中加入对 Proxies 原始节点的解析
	var conf struct {
		Mode        string                   `yaml:"mode"`
		Proxies     []map[string]interface{} `yaml:"proxies"` // <--- 新增这行，读取底层节点
		ProxyGroups []map[string]interface{} `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal(data, &conf); err != nil {
		return nil, err
	}

	// 2. 构造与内核 API 完全一致的 Map 结构
	proxiesMap := make(map[string]interface{})

	// ⭐️ 核心修复 2：遍历底层节点，将其注入到 Map 中，供前端提取协议名
	for _, p := range conf.Proxies {
		name, _ := p["name"].(string)
		pType, _ := p["type"].(string)
		proxiesMap[name] = map[string]interface{}{
			"name": name,
			"type": pType, // 这里就是真实的 vless, trojan, hysteria2 等
		}
	}

	for _, g := range conf.ProxyGroups {
		name, _ := g["name"].(string)
		gTypeRaw, _ := g["type"].(string)

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
			"now":  "",
			"all":  all,
		}
	}

	return map[string]interface{}{
		"mode":       conf.Mode,
		"groups":     proxiesMap, // 现在包含了所有真实节点信息
		"groupOrder": ExtractGroupOrder(data),
	}, nil
}

// DownloadSubscription 下载订阅文件并覆盖本地 config.yaml
func DownloadSubscription(subUrl string, userAgent string) error {
	configMu.Lock()
	defer configMu.Unlock()

	configPath := GetConfigPath() // 👈 使用绝对路径

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

// ================= TUN 设置 =================
func GetTunConfig() (*TunConfig, error) {
	defaultTun := TunConfig{
		Enable:              false,
		Stack:               "mixed",
		Device:              "",
		AutoRoute:           true,
		AutoDetectInterface: true,
		DNSHijack:           []string{"any:53"},
		StrictRoute:         false,
		MTU:                 1430,
	}
	return utils.LoadSetting("tun", defaultTun)
}

func UpdateTunConfig(newTun *TunConfig) error {
	return utils.SaveSetting("tun", newTun)
}

// -------------------- DNS 配置相关 --------------------

// FallbackFilterConfig 映射 yaml 中 dns.fallback-filter 配置块
type FallbackFilterConfig struct {
	GeoIP     bool     `yaml:"geoip" json:"geoip"`
	GeoIPCode string   `yaml:"geoip-code" json:"geoipCode"`
	IPCIDR    []string `yaml:"ipcidr" json:"ipcidr"`
	Domain    []string `yaml:"domain,omitempty" json:"domain"` // 👈 新增：域名过滤
}

// DNSConfig 映射 yaml 中的 dns 配置块
type DNSConfig struct {
	Enable                bool                 `yaml:"enable" json:"enable"`
	Listen                string               `yaml:"listen,omitempty" json:"listen"` // 👈 新增：监听端口
	IPv6                  bool                 `yaml:"ipv6" json:"ipv6"`
	PreferH3              bool                 `yaml:"prefer-h3,omitempty" json:"preferH3"` // 👈 新增：偏好 HTTP/3
	EnhancedMode          string               `yaml:"enhanced-mode" json:"enhancedMode"`
	RespectRules          bool                 `yaml:"respect-rules,omitempty" json:"respectRules"` // 👈 新增：遵守规则
	FakeIPRange           string               `yaml:"fake-ip-range,omitempty" json:"fakeIpRange"`
	FakeIPFilter          []string             `yaml:"fake-ip-filter,omitempty" json:"fakeIpFilter"`
	UseSystemHosts        bool                 `yaml:"use-system-hosts,omitempty" json:"useSystemHosts"`
	UseHosts              bool                 `yaml:"use-hosts,omitempty" json:"useHosts"`
	DefaultNameserver     []string             `yaml:"default-nameserver,omitempty" json:"defaultNameserver"`
	Nameserver            []string             `yaml:"nameserver" json:"nameserver"`
	Fallback              []string             `yaml:"fallback,omitempty" json:"fallback"`
	DirectNameserver      []string             `yaml:"direct-nameserver,omitempty" json:"directNameserver"` // 👈 新增：直连 DNS
	ProxyServerNameserver []string             `yaml:"proxy-server-nameserver,omitempty" json:"proxyServerNameserver"`
	NameserverPolicy      map[string]string    `yaml:"nameserver-policy,omitempty" json:"nameserverPolicy"`
	FallbackFilter        FallbackFilterConfig `yaml:"fallback-filter" json:"fallbackFilter"`
}

// ================= DNS 设置 =================
func GetDNSConfig() (*DNSConfig, error) {
	defaultDNS := DNSConfig{
		Enable:                true,
		Listen:                "0.0.0.0:1053",
		IPv6:                  true,
		PreferH3:              false,
		EnhancedMode:          "fake-ip",
		RespectRules:          false,
		FakeIPRange:           "198.18.0.1/16",
		FakeIPFilter:          []string{"*.lan", "*.localdomain", "*.example", "*.invalid", "*.localhost", "*.test", "lan", "localdomain", "localhost"},
		UseSystemHosts:        true,
		UseHosts:              false,
		DefaultNameserver:     []string{"223.5.5.5", "114.114.114.114"},
		Nameserver:            []string{"https://doh.pub/dns-query", "https://dns.alidns.com/dns-query"},
		Fallback:              []string{"https://doh.dns.sb/dns-query", "https://dns.cloudflare.com/dns-query"},
		DirectNameserver:      []string{"https://dns.alidns.com/dns-query", "https://doh.pub/dns-query"},
		ProxyServerNameserver: []string{"223.5.5.5", "114.114.114.114"},
		NameserverPolicy:      map[string]string{"geosite:cn": "https://doh.pub/dns-query"},
		FallbackFilter: FallbackFilterConfig{
			GeoIP:     true,
			GeoIPCode: "CN",
			IPCIDR:    []string{"240.0.0.0/4", "0.0.0.0/32"},
			Domain:    []string{"+.google.com", "+.facebook.com", "+.twitter.com"},
		},
	}
	return utils.LoadSetting("dns", defaultDNS)
}

func UpdateDNSConfig(newDNS *DNSConfig) error {
	return utils.SaveSetting("dns", newDNS)
}

// ================= 基础网络设置 =================
func GetNetworkConfig() (*NetworkConfig, error) {
	defaultNet := NetworkConfig{
		Port:                 0,
		MixedPort:            7890,
		IPv6:                 false,
		UnifiedDelay:         true,
		DelayRetention:       false,
		DelayRetentionTime:   "long",
		TCPConcurrent:        true,
		TCPKeepAlive:         true,
		TCPKeepAliveInterval: 30,
		TestURL:              "http://www.gstatic.com/generate_204",
		Hosts:                "",
	}
	return utils.LoadSetting("network", defaultNet)
}

func UpdateNetworkConfig(newCfg *NetworkConfig) error {
	return utils.SaveSetting("network", newCfg)
}

// ==========================================
// --- 运行时参数注入器 (借鉴 Stelliberty) ---
// ==========================================

// BuildRuntimeConfig 核心流水线：基础配置 + 用户设置 = 最终运行配置
func BuildRuntimeConfig(id string, mode string, logLevel string) error {
	configMu.Lock()         // ✅ 加锁
	defer configMu.Unlock() // ✅ 保证最终释放

	configPath := GetConfigPath() // 目标: DataDir/config.yaml

	// 1. 提取当前界面的全局设置 (避免被覆盖丢失)
	userDns, _ := GetDNSConfig()
	userTun, _ := GetTunConfig()
	userNet, _ := GetNetworkConfig()

	// 2. 读取选中的订阅文件作为 "Base Config" (只读模板)
	profilePath := filepath.Join(utils.GetSubscriptionsDir(), id+".yaml")
	baseData, err := os.ReadFile(profilePath)
	if err != nil {
		return fmt.Errorf("读取基础配置失败: %v", err)
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal(baseData, &root); err != nil {
		return fmt.Errorf("解析基础配置失败: %v", err)
	}

	// 3. 运行时参数强制注入 (Injector)
	// 注入模式 (rule / global / direct)
	if mode != "" {
		root["mode"] = mode
	}

	// 注入基础网络设置
	if userNet != nil {
		root["ipv6"] = userNet.IPv6
		root["unified-delay"] = userNet.UnifiedDelay
		root["tcp-concurrent"] = userNet.TCPConcurrent
		root["tcp-keep-alive"] = userNet.TCPKeepAlive
		root["tcp-keep-alive-interval"] = userNet.TCPKeepAliveInterval
		root["test-url"] = userNet.TestURL // 👈 核心修复：确保自定义测速地址在重启后不丢失

		// 👈 新增：注入自定义 Hosts 映射
		if userNet.Hosts != "" {
			var hostsMap map[string]interface{}
			if err := yaml.Unmarshal([]byte(userNet.Hosts), &hostsMap); err == nil {
				root["hosts"] = hostsMap
			}
		}
	}

	// 注入 TUN 配置
	if userTun != nil {
		root["tun"] = userTun
	}

	// 注入 DNS 配置
	if userDns != nil && userDns.Enable {
		root["dns"] = map[string]interface{}{
			"enable":                  userDns.Enable,
			"listen":                  userDns.Listen,
			"ipv6":                    userDns.IPv6,
			"prefer-h3":               userDns.PreferH3,
			"enhanced-mode":           userDns.EnhancedMode,
			"respect-rules":           userDns.RespectRules,
			"fake-ip-range":           userDns.FakeIPRange,
			"fake-ip-filter":          userDns.FakeIPFilter,
			"use-system-hosts":        userDns.UseSystemHosts,
			"use-hosts":               userDns.UseHosts,
			"default-nameserver":      userDns.DefaultNameserver,
			"nameserver":              userDns.Nameserver,
			"fallback":                userDns.Fallback,
			"direct-nameserver":       userDns.DirectNameserver,
			"proxy-server-nameserver": userDns.ProxyServerNameserver,
			"nameserver-policy":       userDns.NameserverPolicy,
			"fallback-filter":         userDns.FallbackFilter,
		}
	}

	// 👇 核心注入：遍历所有策略组，将用户设置的测速网址应用到 url-test/fallback 组
	if userNet != nil && userNet.TestURL != "" {
		if groups, ok := root["proxy-groups"].([]interface{}); ok {
			for _, g := range groups {
				if group, ok := g.(map[string]interface{}); ok {
					gType, _ := group["type"].(string)
					// 仅对具有自动测速性质的组生效
					if gType == "url-test" || gType == "fallback" || gType == "load-balance" {
						group["url"] = userNet.TestURL
					}
				}
			}
		}
	}

	// 👇 新增：强制注入混合代理端口和外部控制 API
	// 确保在不启用 TUN 时，系统代理能够将流量送入内核
	if userNet != nil && userNet.MixedPort != 0 {
		root["mixed-port"] = userNet.MixedPort
	} else if userNet != nil && userNet.Port != 0 {
		// 如果只有 Port 没有 MixedPort，优先保证连通性
		root["port"] = userNet.Port
		delete(root, "mixed-port")
	} else {
		root["mixed-port"] = 7890
	}
	root["allow-lan"] = true
	root["external-controller"] = "127.0.0.1:9090"
	root["secret"] = "" // 确保没有意外的密码阻挡前端 WebSocket

	// 👇 核心注入：规则接管 (真理只在 JSON 中)
	customRules, _ := GetCustomRules(id)
	root["rules"] = customRules

	// 👇 核心新增：动态读取并注入我们设置的日志等级
	if logLevel != "" {
		root["log-level"] = logLevel
	} else {
		root["log-level"] = "info"
	}

	// 4. 序列化并生成最终的 config.yaml
	out, err := yaml.Marshal(root)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, out, 0644)
}

// ExtractGroupOrder 核心逻辑：从 YAML 数据中提取 proxy-groups 的原始定义顺序
func ExtractGroupOrder(yamlData []byte) []string {
	var order []string
	var node yaml.Node
	if err := yaml.Unmarshal(yamlData, &node); err == nil && len(node.Content) > 0 {
		// 遍历根节点，寻找 proxy-groups
		for i := 0; i < len(node.Content[0].Content); i += 2 {
			keyNode := node.Content[0].Content[i]
			if keyNode.Value == "proxy-groups" {
				valueNode := node.Content[0].Content[i+1]
				for _, groupNode := range valueNode.Content {
					// 提取每个 group 的 name
					for j := 0; j < len(groupNode.Content); j += 2 {
						if groupNode.Content[j].Value == "name" {
							order = append(order, groupNode.Content[j+1].Value)
							break
						}
					}
				}
				break
			}
		}
	}
	return order
}
