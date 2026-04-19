package clash

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// GetConfigPath 获取 config.yaml 的绝对路径（导出供 app.go 使用，确保路径一致）
func GetConfigPath() string {
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

// NetworkConfig 基础网络配置
type NetworkConfig struct {
	IPv6                 bool `yaml:"ipv6" json:"ipv6"`
	UnifiedDelay         bool `yaml:"unified-delay" json:"unifiedDelay"`
	TCPConcurrent        bool `yaml:"tcp-concurrent" json:"tcpConcurrent"`
	TCPKeepAlive         bool `yaml:"tcp-keep-alive" json:"tcpKeepAlive"`
	TCPKeepAliveInterval int  `yaml:"tcp-keep-alive-interval" json:"tcpKeepAliveInterval"`
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
		"mode":       conf.Mode,
		"groups":     proxiesMap, // 这里的格式将完美契合前端 Proxies.vue 的逻辑
		"groupOrder": ExtractGroupOrder(data),
	}, nil
}


// DownloadSubscription 下载订阅文件并覆盖本地 config.yaml
func DownloadSubscription(subUrl string, userAgent string) error {
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

// GetTunConfig 从 config.yaml 读取 TUN 配置
func GetTunConfig() (*TunConfig, error) {
	configPath := GetConfigPath() // 👈 使用绝对路径

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
	configPath := GetConfigPath() // 👈 使用绝对路径

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
	Domain    []string `yaml:"domain,omitempty" json:"domain"` // 👈 新增：域名过滤
}

// DNSConfig 映射 yaml 中的 dns 配置块
type DNSConfig struct {
	Enable                bool                 `yaml:"enable" json:"enable"`
	Listen                string               `yaml:"listen,omitempty" json:"listen"`             // 👈 新增：监听端口
	IPv6                  bool                 `yaml:"ipv6" json:"ipv6"`
	PreferH3              bool                 `yaml:"prefer-h3,omitempty" json:"preferH3"`        // 👈 新增：偏好 HTTP/3
	EnhancedMode          string               `yaml:"enhanced-mode" json:"enhancedMode"`
	RespectRules          bool                 `yaml:"respect-rules,omitempty" json:"respectRules"`// 👈 新增：遵守规则
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

// GetDNSConfig 读取 DNS 配置
func GetDNSConfig() (*DNSConfig, error) {
	configPath := GetConfigPath()

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	// 初始化完整默认值
	conf := &DNSConfig{
		Enable:            true,
		Listen:            "0.0.0.0:1053",
		IPv6:              false,
		PreferH3:          false,
		EnhancedMode:      "fake-ip",
		RespectRules:      false,
		FakeIPRange:       "198.18.0.1/16",
		FakeIPFilter:      []string{"*.lan", "*.localdomain", "*.example", "*.invalid", "*.localhost", "*.test", "lan", "localdomain", "localhost"},
		UseSystemHosts:    true,
		UseHosts:          true,
		DefaultNameserver: []string{"223.5.5.5", "114.114.114.114"},
		Nameserver:        []string{"https://doh.pub/dns-query", "https://dns.alidns.com/dns-query"},
		Fallback:          []string{"https://doh.dns.sb/dns-query", "https://dns.cloudflare.com/dns-query"},
		DirectNameserver:  []string{"https://dns.alidns.com/dns-query"},
		ProxyServerNameserver: []string{"https://doh.pub/dns-query"},
		NameserverPolicy:      map[string]string{"geosite:cn": "https://doh.pub/dns-query"},
		FallbackFilter: FallbackFilterConfig{
			GeoIP:     true,
			GeoIPCode: "CN",
			IPCIDR:    []string{"240.0.0.0/4", "0.0.0.0/32"},
			Domain:    []string{"+.google.com", "+.facebook.com", "+.twitter.com"},
		},
	}

	if dnsMap, ok := root["dns"].(map[string]interface{}); ok {
		raw, _ := yaml.Marshal(dnsMap)
		yaml.Unmarshal(raw, conf)
		if fakeRange, ok := dnsMap["fake-ip-range"].(string); ok {
			conf.FakeIPRange = fakeRange
		}
	}

	return conf, nil
}

// UpdateDNSConfig 将新的 DNS 配置写入 config.yaml
func UpdateDNSConfig(newDNS *DNSConfig) error {
	configPath := GetConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}

	dnsMap := map[string]interface{}{
		"enable":                  newDNS.Enable,
		"listen":                  newDNS.Listen,
		"ipv6":                    newDNS.IPv6,
		"prefer-h3":               newDNS.PreferH3,
		"enhanced-mode":           newDNS.EnhancedMode,
		"respect-rules":           newDNS.RespectRules,
		"fake-ip-range":           newDNS.FakeIPRange,
		"fake-ip-filter":          newDNS.FakeIPFilter,
		"use-system-hosts":        newDNS.UseSystemHosts,
		"use-hosts":               newDNS.UseHosts,
		"default-nameserver":      newDNS.DefaultNameserver,
		"nameserver":              newDNS.Nameserver,
		"fallback":                newDNS.Fallback,
		"direct-nameserver":       newDNS.DirectNameserver,
		"proxy-server-nameserver": newDNS.ProxyServerNameserver,
		"nameserver-policy":       newDNS.NameserverPolicy,
		"fallback-filter":         newDNS.FallbackFilter,
	}

	root["dns"] = dnsMap
	out, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, out, 0644)
}

// GetNetworkConfig 获取基础网络配置
func GetNetworkConfig() (*NetworkConfig, error) {
	configPath := GetConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	// 默认值设置
	conf := &NetworkConfig{
		IPv6:                 false,
		UnifiedDelay:         true,
		TCPConcurrent:        true,
		TCPKeepAlive:         true,
		TCPKeepAliveInterval: 15,
	}

	// 从 yaml 根路径读取
	if v, ok := root["ipv6"].(bool); ok {
		conf.IPv6 = v
	}
	if v, ok := root["unified-delay"].(bool); ok {
		conf.UnifiedDelay = v
	}
	if v, ok := root["tcp-concurrent"].(bool); ok {
		conf.TCPConcurrent = v
	}
	if v, ok := root["tcp-keep-alive"].(bool); ok {
		conf.TCPKeepAlive = v
	}
	if v, ok := root["tcp-keep-alive-interval"].(int); ok {
		conf.TCPKeepAliveInterval = v
	}

	return conf, nil
}

// UpdateNetworkConfig 更新基础网络配置
func UpdateNetworkConfig(newCfg *NetworkConfig) error {
	configPath := GetConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}

	// 直接注入根节点
	root["ipv6"] = newCfg.IPv6
	root["unified-delay"] = newCfg.UnifiedDelay
	root["tcp-concurrent"] = newCfg.TCPConcurrent
	root["tcp-keep-alive"] = newCfg.TCPKeepAlive
	root["tcp-keep-alive-interval"] = newCfg.TCPKeepAliveInterval

	out, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, out, 0644)
}

// ==========================================
// --- 运行时参数注入器 (借鉴 Stelliberty) ---
// ==========================================

// BuildRuntimeConfig 核心流水线：基础配置 + 用户设置 = 最终运行配置
func BuildRuntimeConfig(profileName string, mode string) error {
	configPath := GetConfigPath() // 目标: core/bin/config.yaml
	baseDir := filepath.Dir(configPath)

	// 1. 提取当前界面的全局设置 (避免被覆盖丢失)
	userDns, _ := GetDNSConfig()
	userTun, _ := GetTunConfig()
	userNet, _ := GetNetworkConfig()

	// 2. 读取选中的订阅文件作为 "Base Config" (只读模板)
	profilePath := filepath.Join(baseDir, profileName)
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

	// 👇 新增：强制注入混合代理端口和外部控制 API
	// 确保在不启用 TUN 时，系统代理 (7890) 依然能够将流量送入内核
	root["mixed-port"] = 7890
	root["allow-lan"] = true
	root["external-controller"] = "127.0.0.1:9090"
	root["secret"] = "" // 确保没有意外的密码阻挡前端 WebSocket

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

