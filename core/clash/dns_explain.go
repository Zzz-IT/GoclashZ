//go:build windows

package clash

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"

	"goclashz/core/utils"

	"gopkg.in/yaml.v3"
)

// 🧭 DNS 解析决策解释
//
// 「这个域名会用哪个 DNS 解析器」是连接详情里最常被问的问题。这里不发起任何真实 DNS 查询，
// 只按运行时 config.yaml 的 dns 段做一次静态推演：nameserver-policy → fake-ip-filter →
// fallback → main(nameserver) → default(default-nameserver)，并在 Notes 里写清哪些部分无法静态判定。

// Source 取值（机器码，由前端翻译成文案）
const (
	dnsSourcePolicy   = "policy"   // 命中 nameserver-policy
	dnsSourceFallback = "fallback" // 走了 fallback 列表
	dnsSourceMain     = "main"     // 走了主 nameserver
	dnsSourceDefault  = "default"  // 退回 default-nameserver
)

// dnsStaticNote 固定提示：结果来自配置推演，不是真实查询
const dnsStaticNote = "纯静态推演，未发起真实 DNS 查询"

// DNSExplain 一次 DNS 解析决策的静态解释
type DNSExplain struct {
	Domain    string   `json:"domain"`    // 归一化后的域名（小写、去掉末尾根点）
	QueryType string   `json:"queryType"` // 查询类型：A 或 A / AAAA（取决于 dns.ipv6）
	Resolver  string   `json:"resolver"`  // 命中的解析器地址，如 223.5.5.5
	Source    string   `json:"source"`    // policy / fallback / main / default
	PolicyKey string   `json:"policyKey"` // 命中的 nameserver-policy 键，如 DOMAIN-SUFFIX,google.com
	FakeIP    bool     `json:"fakeIp"`    // 是否走 fake-ip
	Filtered  bool     `json:"filtered"`  // 是否命中 fake-ip-filter（命中则不走 fake-ip）
	Notes     []string `json:"notes"`     // 中文提示，永不 nil
}

// runtimeDNSSection 运行时配置里的 dns 段（只取解释需要的字段）
type runtimeDNSSection struct {
	Enable            bool                   `yaml:"enable"`
	IPv6              bool                   `yaml:"ipv6"`
	EnhancedMode      string                 `yaml:"enhanced-mode"`
	FakeIPFilter      []string               `yaml:"fake-ip-filter"`
	DefaultNameserver []string               `yaml:"default-nameserver"`
	Nameserver        []string               `yaml:"nameserver"`
	Fallback          []string               `yaml:"fallback"`
	NameserverPolicy  map[string]interface{} `yaml:"nameserver-policy"`
}

// ExplainDNS 静态推演 domain 会由哪个解析器处理。
func ExplainDNS(domain string) (*DNSExplain, error) {
	normalized, err := normalizeExplainDomain(domain)
	if err != nil {
		return nil, err
	}

	dns, err := loadRuntimeDNS()
	if err != nil {
		return nil, err
	}

	explain := &DNSExplain{
		Domain: normalized,
		Notes:  []string{},
	}

	// 查询类型：fake-ip 只影响 A / AAAA，这里按配置里的 ipv6 开关给出展示值
	if dns.IPv6 {
		explain.QueryType = "A / AAAA"
	} else {
		explain.QueryType = "A"
	}

	if !dns.Enable {
		explain.Source = dnsSourceDefault
		explain.Notes = append(explain.Notes, "内核 DNS 未启用，实际走系统解析")
		explain.Notes = append(explain.Notes, dnsStaticNote)
		return explain, nil
	}

	// 1. nameserver-policy 优先（只有 DOMAIN-SUFFIX / DOMAIN 能静态判定）
	policyResolver, policyKey, policyNotes := matchDNSNameserverPolicy(dns.NameserverPolicy, normalized)
	explain.Notes = append(explain.Notes, policyNotes...)

	// 2. fake-ip 与 fake-ip-filter
	fakeIPMode := strings.EqualFold(strings.TrimSpace(dns.EnhancedMode), "fake-ip")
	if fakeIPMode {
		filteredBy, filterNotes := matchFakeIPFilter(dns.FakeIPFilter, normalized)
		explain.Notes = append(explain.Notes, filterNotes...)
		if filteredBy != "" {
			explain.Filtered = true
			explain.Notes = append(explain.Notes, "命中 fake-ip-filter："+filteredBy)
		} else {
			explain.FakeIP = true
			explain.Notes = append(explain.Notes, "fake-ip：返回虚拟 IP，真实解析在建立连接时完成")
		}
	}

	// 3. 解析器选择
	if policyResolver != "" {
		explain.Resolver = policyResolver
		explain.Source = dnsSourcePolicy
		explain.PolicyKey = policyKey
	} else if explain.FakeIP {
		// fake-ip 分支：上游解析只用主 nameserver，fallback 不参与假 IP
		if resolver := firstNonEmpty(dns.Nameserver); resolver != "" {
			explain.Resolver = resolver
			explain.Source = dnsSourceMain
		} else if resolver := firstNonEmpty(dns.DefaultNameserver); resolver != "" {
			explain.Resolver = resolver
			explain.Source = dnsSourceDefault
		}
	} else if resolver := firstNonEmpty(dns.Fallback); resolver != "" {
		explain.Resolver = resolver
		explain.Source = dnsSourceFallback
	} else if resolver := firstNonEmpty(dns.Nameserver); resolver != "" {
		explain.Resolver = resolver
		explain.Source = dnsSourceMain
	} else if resolver := firstNonEmpty(dns.DefaultNameserver); resolver != "" {
		explain.Resolver = resolver
		explain.Source = dnsSourceDefault
	}

	// 兜底：配置里一个 nameserver 都没有
	if explain.Resolver == "" {
		explain.Source = dnsSourceDefault
		explain.Notes = append(explain.Notes, "未配置 nameserver，只能用系统默认解析")
	}

	explain.Notes = append(explain.Notes, dnsStaticNote)

	return explain, nil
}

// loadRuntimeDNS 读取运行时 config.yaml 的 dns 段；只读，不写盘、不触发内核 API
func loadRuntimeDNS() (*runtimeDNSSection, error) {
	path := utils.GetRuntimeConfigPath()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("未找到运行时配置")
	}

	var root struct {
		DNS runtimeDNSSection `yaml:"dns"`
	}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("运行时配置格式错误")
	}

	return &root.DNS, nil
}

// matchDNSNameserverPolicy 在 nameserver-policy 里找命中的键。
// 键形如 DOMAIN-SUFFIX,example.com / DOMAIN,x / geosite:xx：只对前两种做匹配，
// 其它形式（需要 geosite / 规则集数据）只写一条「未展开」提示，不报错。
// 返回：解析器地址、命中的原始键、需要回显的提示。
func matchDNSNameserverPolicy(policy map[string]interface{}, domain string) (string, string, []string) {
	notes := []string{}
	if len(policy) == 0 {
		return "", "", notes
	}

	// map 遍历顺序随机，排序后再匹配，保证同样的配置每次给出同样的结果
	keys := make([]string, 0, len(policy))
	for key := range policy {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// 第一轮：先把无法静态展开的键记录下来 —— 它们同样可能命中，结论要保留余地
	supported := make([]string, 0, len(keys))
	for _, key := range keys {
		ruleType, _ := splitDNSPolicyKey(key)
		switch ruleType {
		case "DOMAIN-SUFFIX", "DOMAIN":
			supported = append(supported, key)
		default:
			notes = append(notes, "策略未展开："+key)
		}
	}

	// 第二轮：只对可判定的 DOMAIN-SUFFIX / DOMAIN 做匹配
	for _, key := range supported {
		ruleType, payload := splitDNSPolicyKey(key)

		switch ruleType {
		case "DOMAIN-SUFFIX":
			if !domainSuffixMatch(domain, payload) {
				continue
			}
		case "DOMAIN":
			if domain != strings.TrimSuffix(strings.ToLower(strings.TrimSpace(payload)), ".") {
				continue
			}
		}

		if resolver := firstResolverValue(policy[key]); resolver != "" {
			return resolver, key, notes
		}
		notes = append(notes, "策略值为空："+key)
	}

	return "", "", notes
}

// splitDNSPolicyKey 拆分 "DOMAIN-SUFFIX,example.com" 这种键；没有逗号（如 geosite:cn）时 type 为空
func splitDNSPolicyKey(key string) (string, string) {
	trimmed := strings.TrimSpace(key)
	echoPolicy := trimmed
	if index := strings.Index(trimmed, ","); index > 0 {
		return strings.ToUpper(strings.TrimSpace(trimmed[:index])), strings.TrimSpace(trimmed[index+1:])
	}
	return "", echoPolicy
}

// firstResolverValue nameserver-policy 的值可能是单个字符串，也可能是字符串数组，取第一个非空值
func firstResolverValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []interface{}:
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

// firstNonEmpty 取列表里第一个非空字符串
func firstNonEmpty(list []string) string {
	for _, item := range list {
		if value := strings.TrimSpace(item); value != "" {
			return value
		}
	}
	return ""
}

// dnsDomainPattern 只接受常见域名写法：字母/数字/下划线开头结尾，中间允许点与连字符
var dnsDomainPattern = regexp.MustCompile("^[a-z0-9_]([a-z0-9._-]*[a-z0-9_])?$")

// normalizeExplainDomain 归一化用户输入：允许直接贴 URL 或 host:port，但最终必须是合法域名
func normalizeExplainDomain(raw string) (string, error) {
	input := strings.TrimSpace(raw)
	if input == "" {
		return "", fmt.Errorf("请输入域名")
	}

	if strings.Contains(input, "://") {
		if parsed, err := url.Parse(input); err == nil && strings.TrimSpace(parsed.Hostname()) != "" {
			input = parsed.Hostname()
		}
	} else if !isIPLiteral(input) {
		if host, _, err := net.SplitHostPort(input); err == nil && strings.TrimSpace(host) != "" {
			input = host
		}
	}

	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(input), "."))
	if domain == "" || strings.Contains(domain, "..") || isIPLiteral(domain) || !dnsDomainPattern.MatchString(domain) {
		return "", fmt.Errorf("请输入域名")
	}

	return domain, nil
}

// matchFakeIPFilter 在 fake-ip-filter 里找命中的条目。
// 支持 *.lan 这类通配与 +.example.com / example.com 后缀写法；
// geosite: / rule-set: / regexp: 这些需要内核数据的形式只写一条「未展开」提示。
func matchFakeIPFilter(filters []string, domain string) (string, []string) {
	notes := []string{}

	for _, raw := range filters {
		pattern := strings.ToLower(strings.TrimSpace(raw))
		if pattern == "" {
			continue
		}

		if strings.Contains(pattern, ":") {
			notes = append(notes, "过滤器未展开："+raw)
			continue
		}

		if fakeIPFilterMatch(pattern, domain) {
			return raw, notes
		}
	}

	return "", notes
}

// fakeIPFilterMatch 常见写法：*.example.com（通配）、+.example.com（后缀）、example.com（精确）
func fakeIPFilterMatch(pattern, domain string) bool {
	normalized := strings.TrimSpace(pattern)

	switch {
	case normalized == "*":
		return true
	case strings.HasPrefix(normalized, "*."):
		return domainSuffixMatch(domain, normalized[2:])
	case strings.HasPrefix(normalized, "+."):
		return domainSuffixMatch(domain, normalized[2:])
	case strings.HasPrefix(normalized, "."):
		return domainSuffixMatch(domain, normalized[1:])
	case strings.Contains(normalized, "*"):
		return wildcardMatch(normalized, domain)
	default:
		return domain == normalized
	}
}

// isIPLiteral 判断字符串本身是否就是一个 IP 字面量（允许 IPv6 带方括号）
func isIPLiteral(s string) bool {
	candidate := strings.TrimSpace(s)
	if strings.HasPrefix(candidate, "[") && strings.HasSuffix(candidate, "]") {
		candidate = candidate[1 : len(candidate)-1]
	}
	return net.ParseIP(candidate) != nil
}

// domainSuffixMatch 判断 host 是否等于 suffix，或以 .suffix 结尾（Clash 的 DOMAIN-SUFFIX 语义）
func domainSuffixMatch(host, suffix string) bool {
	suffix = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(suffix)), ".")
	if suffix == "" {
		return false
	}
	return host == suffix || strings.HasSuffix(host, "."+suffix)
}

// wildcardMatch 把 * 当作任意字符做通配匹配（只服务 fake-ip-filter 这种短列表）
func wildcardMatch(pattern, domain string) bool {
	parts := strings.Split(pattern, "*")
	for index := range parts {
		parts[index] = regexp.QuoteMeta(parts[index])
	}

	matched, err := regexp.MatchString("^"+strings.Join(parts, ".*")+"$", domain)
	return err == nil && matched
}
