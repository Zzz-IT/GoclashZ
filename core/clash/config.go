package clash

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"encoding/json"
	"gopkg.in/yaml.v3"
	"sync"
	"goclashz/core/utils"
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
	TCPConcurrent        bool   `yaml:"tcp-concurrent" json:"tcpConcurrent"`
	TCPKeepAlive         bool   `yaml:"tcp-keep-alive" json:"tcpKeepAlive"`
	TCPKeepAliveInterval int    `yaml:"tcp-keep-alive-interval" json:"tcpKeepAliveInterval"`
	TestURL              string `yaml:"test-url" json:"testUrl"` // 👈 新增
	Hosts                string `yaml:"-" json:"hosts"`         // 👈 新增：作为字符串传递给前端
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
	// 如果传入了空或者 config.yaml，直接指向主配置
	if fileName == "" || fileName == "config.yaml" {
		fileName = "config.yaml"
	}

	var configPath string
	if fileName == "config.yaml" {
		configPath = GetConfigPath()
	} else {
		configPath = filepath.Join(utils.GetProfilesDir(), fileName)
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
	configMu.Lock()         // ✅ 加锁
	defer configMu.Unlock() // ✅ 保证最终释放

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
	configMu.Lock()         // ✅ 加锁
	defer configMu.Unlock() // ✅ 保证最终释放

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
		Port:                 0,
		MixedPort:            7890,
		IPv6:                 false,
		UnifiedDelay:         true,
		TCPConcurrent:        true,
		TCPKeepAlive:         true,
		TCPKeepAliveInterval: 15,
		TestURL:              "http://www.gstatic.com/generate_204", // 👈 默认值
	}

	// 从 yaml 根路径读取
	if v, ok := root["port"].(int); ok {
		conf.Port = v
	}
	if v, ok := root["mixed-port"].(int); ok {
		conf.MixedPort = v
	}
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
	if v, ok := root["test-url"].(string); ok {
		conf.TestURL = v
	}

	// 👈 新增：读取 hosts 字段并转换为 YAML 字符串
	if v, ok := root["hosts"]; ok {
		if hostsData, err := yaml.Marshal(v); err == nil {
			conf.Hosts = string(hostsData)
		}
	}

	return conf, nil
}

// UpdateNetworkConfig 更新基础网络配置
func UpdateNetworkConfig(newCfg *NetworkConfig) error {
	configMu.Lock()         // ✅ 加锁
	defer configMu.Unlock() // ✅ 保证最终释放

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
	if newCfg.Port != 0 {
		root["port"] = newCfg.Port
	}
	if newCfg.MixedPort != 0 {
		root["mixed-port"] = newCfg.MixedPort
	}
	root["ipv6"] = newCfg.IPv6
	root["unified-delay"] = newCfg.UnifiedDelay
	root["tcp-concurrent"] = newCfg.TCPConcurrent
	root["tcp-keep-alive"] = newCfg.TCPKeepAlive
	root["tcp-keep-alive-interval"] = newCfg.TCPKeepAliveInterval
	root["test-url"] = newCfg.TestURL // 👈 保存到 root 以便下次读取

	// 👈 修复：增加严格的 YAML 语法检查，失败时将错误抛给前端
	if newCfg.Hosts != "" {
		var hostsMap map[string]interface{}
		if err := yaml.Unmarshal([]byte(newCfg.Hosts), &hostsMap); err != nil {
			return fmt.Errorf("Hosts 语法错误，必须符合 YAML 键值对格式: %v", err)
		}
		root["hosts"] = hostsMap
	} else {
		delete(root, "hosts")
	}

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
	configMu.Lock()         // ✅ 加锁
	defer configMu.Unlock() // ✅ 保证最终释放

	configPath := GetConfigPath() // 目标: DataDir/config.yaml

	// 1. 提取当前界面的全局设置 (避免被覆盖丢失)
	userDns, _ := GetDNSConfig()
	userTun, _ := GetTunConfig()
	userNet, _ := GetNetworkConfig()

	// 2. 读取选中的订阅文件作为 "Base Config" (只读模板)
	profilePath := filepath.Join(utils.GetProfilesDir(), profileName)
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
	
	// 👇 核心新增：动态读取并注入我们设置的日志等级
	root["log-level"] = getAppBehaviorLogLevel()

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

// -------------------- 规则配置相关 --------------------

// RuleInfo 封装规则数据及其元数据
type RuleInfo struct {
	Rules      []string `json:"rules"`
	IsEditable bool     `json:"isEditable"` // 是否允许增删
}

// GetRules 获取当前活跃配置的规则
func GetRules(profileName string) (RuleInfo, error) {
	// 判断是否允许编辑：
	// 如果 profileName 为空或者是运行时的 config.yaml，通常属于订阅源或合并后的产物，设为只读
	isEditable := profileName != "" && profileName != "config.yaml"

	path := GetConfigPath() // 默认指向 DataDir/config.yaml
	if isEditable {
		// 指向用户选择的具体本地/订阅配置文件
		path = filepath.Join(utils.GetProfilesDir(), profileName)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return RuleInfo{}, err
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return RuleInfo{}, err
	}

	rules := []string{}
	if r, ok := root["rules"].([]interface{}); ok {
		for _, val := range r {
			if s, ok := val.(string); ok {
				rules = append(rules, s)
			}
		}
	}

	return RuleInfo{
		Rules:      rules,
		IsEditable: isEditable,
	}, nil
}

// SaveRules 将新规则保存回原始导入的配置文件
func SaveRules(profileName string, newRules []string) error {
	if profileName == "" || profileName == "config.yaml" {
		return fmt.Errorf("当前配置不可直接修改，请在本地配置文件中操作")
	}

	sourcePath := filepath.Join(utils.GetProfilesDir(), profileName)

	// 1. 读取并修改原始导入文件 (保留其他配置不变)
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}

	// 找到 rules 节点并更新 (这里简化为转 map 处理以防丢失结构，生产环境建议直接操作 yaml.Node)
	var rootMap map[string]interface{}
	if err := yaml.Unmarshal(data, &rootMap); err != nil {
		return err
	}
	rootMap["rules"] = newRules

	out, err := yaml.Marshal(rootMap)
	if err != nil {
		return err
	}

	// 覆写原文件
	return os.WriteFile(sourcePath, out, 0644)
}

// getAppBehaviorLogLevel 在文件底部新增此辅助方法
func getAppBehaviorLogLevel() string {
	path := filepath.Join(utils.GetDataDir(), "app_behavior.json")
	data, err := os.ReadFile(path)
	if err != nil { return "info" }
	var config struct {
		LogLevel string `json:"logLevel"`
	}
	json.Unmarshal(data, &config)
	if config.LogLevel == "" { return "info" }
	return config.LogLevel
}

