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
// 只要存在无法展开的策略键（geosite: / rule-set:）或 fallback-filter 生效，Uncertain 就是 true：
// 此时 Resolver 只是「最可能」的答案，不能当成定论。

// Source 取值（机器码，由前端翻译成文案）
const (
	dnsSourcePolicy   = "policy"   // 命中 nameserver-policy
	dnsSourceFallback = "fallback" // 走了 fallback 列表
	dnsSourceMain     = "main"     // 走了主 nameserver
	dnsSourceDefault  = "default"  // 退回 default-nameserver
)

// DNSExplain.Notes 的文案 code：前端按 diag.note.<code> 取文案，插值参数名固定为 {arg}。
// 只允许下面 9 个 code，新增文案必须先在这里登记。
const (
	dnsNoteDisabled             = "dnsDisabled"          // 内核 DNS 未启用
	dnsNoteStatic               = "static"               // 纯静态推演，未发起真实查询
	dnsNotePolicyUnresolved     = "policyUnresolved"     // 策略键无法静态展开（Arg=键）
	dnsNotePolicyEmpty          = "policyEmpty"          // 命中的策略值为空（Arg=键）
	dnsNoteFilterUnresolved     = "filterUnresolved"     // fake-ip-filter 条目无法静态展开（Arg=原始条目）
	dnsNoteFakeIPFiltered       = "fakeIpFiltered"       // 命中 fake-ip-filter（Arg=命中的条目）
	dnsNoteFakeIP               = "fakeIp"               // 走 fake-ip，返回虚拟 IP
	dnsNoteFallbackFilterActive = "fallbackFilterActive" // fallback-filter 生效，结论取决于真实解析
	dnsNoteNoNameserver         = "noNameserver"         // 配置里没有任何 nameserver
)

// DNSExplain 一次 DNS 解析决策的静态解释
type DNSExplain struct {
	Domain    string          `json:"domain"`    // 归一化后的域名（小写、去掉末尾根点）
	QueryType string          `json:"queryType"` // 查询类型：A 或 A / AAAA（取决于 dns.ipv6）
	Resolver  string          `json:"resolver"`  // 命中的解析器地址，如 223.5.5.5
	Source    string          `json:"source"`    // policy / fallback / main / default
	PolicyKey string          `json:"policyKey"` // 命中的 nameserver-policy 键，如 DOMAIN-SUFFIX,google.com
	FakeIP    bool            `json:"fakeIp"`    // 是否走 fake-ip
	Filtered  bool            `json:"filtered"`  // 是否命中 fake-ip-filter（命中则不走 fake-ip）
	Uncertain bool            `json:"uncertain"` // 存在无法静态判定的策略键 / fallback-filter 生效，结论不确定
	Notes     []LocalizedText `json:"notes"`     // 本地化提示（code + arg），永不 nil
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
	FallbackFilter    dnsFallbackFilter      `yaml:"fallback-filter"`
}

// dnsFallbackFilter fallback-filter 的开关信息。
// 内核没有单独的 enable 字段，这里兼容「写了 enable: true」的旧配置，
// 只要 geoip / ipcidr / domain 任一启用就视为生效。
type dnsFallbackFilter struct {
	Enable    bool     `yaml:"enable"`
	GeoIP     bool     `yaml:"geoip"`
	GeoIPCode string   `yaml:"geoip-code"`
	IPCIDR    []string `yaml:"ipcidr"`
	Domain    []string `yaml:"domain"`
}

// active 判断 fallback-filter 是否生效
func (f dnsFallbackFilter) active() bool {
	return f.Enable || f.GeoIP || len(f.IPCIDR) > 0 || len(f.Domain) > 0
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
		Notes:  []LocalizedText{},
	}

	// 查询类型：fake-ip 只影响 A / AAAA，这里按配置里的 ipv6 开关给出展示值
	if dns.IPv6 {
		explain.QueryType = "A / AAAA"
	} else {
		explain.QueryType = "A"
	}

	if !dns.Enable {
		explain.Source = dnsSourceDefault
		explain.Notes = append(explain.Notes, LocalizedText{Code: dnsNoteDisabled})
		explain.Notes = append(explain.Notes, LocalizedText{Code: dnsNoteStatic})
		return explain, nil
	}

	// 1. nameserver-policy 优先：支持 DOMAIN,x / DOMAIN-SUFFIX,x / +.x / .x / 裸域
	policyResolver, policyKey, policyNotes, policyUncertain := matchDNSNameserverPolicy(dns.NameserverPolicy, normalized)
	explain.Notes = append(explain.Notes, policyNotes...)
	uncertain := policyUncertain

	// 2. fake-ip 与 fake-ip-filter
	fakeIPMode := strings.EqualFold(strings.TrimSpace(dns.EnhancedMode), "fake-ip")
	if fakeIPMode {
		filteredBy, filterNotes, filterUncertain := matchFakeIPFilter(dns.FakeIPFilter, normalized)
		explain.Notes = append(explain.Notes, filterNotes...)
		if filterUncertain {
			uncertain = true
		}
		if filteredBy != "" {
			explain.Filtered = true
			explain.Notes = append(explain.Notes, LocalizedText{Code: dnsNoteFakeIPFiltered, Arg: filteredBy})
		} else {
			explain.FakeIP = true
			explain.Notes = append(explain.Notes, LocalizedText{Code: dnsNoteFakeIP})
		}
	}

	// 3. 解析器选择
	fallbackResolver := firstNonEmpty(dns.Fallback)
	mainResolver := firstNonEmpty(dns.Nameserver)
	defaultResolver := firstNonEmpty(dns.DefaultNameserver)

	if policyResolver != "" {
		explain.Resolver = policyResolver
		explain.Source = dnsSourcePolicy
		explain.PolicyKey = policyKey
	} else {
		// fallback-filter 生效时，是否回落到 fallback 取决于真实解析结果（污染判定），
		// 静态推演不能断言：这里把主 nameserver 当作「最可能」的解析器，并标记 uncertain。
		fallbackConditional := fallbackResolver != "" && dns.FallbackFilter.active()

		switch {
		case explain.FakeIP:
			// fake-ip 分支：上游解析只用主 nameserver，fallback 不参与假 IP
			if mainResolver != "" {
				explain.Resolver = mainResolver
				explain.Source = dnsSourceMain
			} else if defaultResolver != "" {
				explain.Resolver = defaultResolver
				explain.Source = dnsSourceDefault
			}
		case fallbackResolver != "" && !fallbackConditional:
			explain.Resolver = fallbackResolver
			explain.Source = dnsSourceFallback
		case mainResolver != "":
			explain.Resolver = mainResolver
			explain.Source = dnsSourceMain
		case defaultResolver != "":
			explain.Resolver = defaultResolver
			explain.Source = dnsSourceDefault
		}

		if fallbackConditional {
			uncertain = true
			explain.Notes = append(explain.Notes, LocalizedText{Code: dnsNoteFallbackFilterActive})
		}
	}

	// 兜底：配置里一个 nameserver 都没有
	if explain.Resolver == "" {
		explain.Source = dnsSourceDefault
		explain.Notes = append(explain.Notes, LocalizedText{Code: dnsNoteNoNameserver})
	}

	explain.Uncertain = uncertain
	explain.Notes = append(explain.Notes, LocalizedText{Code: dnsNoteStatic})

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
// 可静态判定的形式：DOMAIN,x（全等）、DOMAIN-SUFFIX,x / +.x / .x / 裸域（后缀匹配）；
// geosite: / rule-set: 等需要内核数据的形式只写一条「未展开」提示，同时把 uncertain 置为 true。
// 返回：解析器地址、命中的原始键、需要回显的提示、是否存在无法静态判定的键。
func matchDNSNameserverPolicy(policy map[string]interface{}, domain string) (string, string, []LocalizedText, bool) {
	notes := []LocalizedText{}
	uncertain := false
	if len(policy) == 0 {
		return "", "", notes, false
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
		if dnsPolicyKeySupported(key) {
			supported = append(supported, key)
			continue
		}
		notes = append(notes, LocalizedText{Code: dnsNotePolicyUnresolved, Arg: key})
		uncertain = true
	}

	// 第二轮：只对可判定的键做匹配
	for _, key := range supported {
		if !dnsPolicyKeyMatches(key, domain) {
			continue
		}

		if resolver := firstResolverValue(policy[key]); resolver != "" {
			return resolver, key, notes, uncertain
		}
		notes = append(notes, LocalizedText{Code: dnsNotePolicyEmpty, Arg: key})
	}

	return "", "", notes, uncertain
}

// dnsPolicyKeySupported 判断 nameserver-policy 的键能否静态判定：
// 支持 DOMAIN,x / DOMAIN-SUFFIX,x / +.x / .x / 裸域；
// 带冒号的（geosite: / rule-set: / regexp:）与其它规则类型不支持。
func dnsPolicyKeySupported(key string) bool {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return false
	}

	if index := strings.Index(trimmed, ","); index > 0 {
		switch strings.ToUpper(strings.TrimSpace(trimmed[:index])) {
		case "DOMAIN", "DOMAIN-SUFFIX":
			return true
		default:
			return false
		}
	}

	return !strings.Contains(strings.ToLower(trimmed), ":")
}

// dnsPolicyKeyMatches 判断一个「可静态判定」的键是否命中 domain
func dnsPolicyKeyMatches(key, domain string) bool {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return false
	}

	if index := strings.Index(trimmed, ","); index > 0 {
		ruleType := strings.ToUpper(strings.TrimSpace(trimmed[:index]))
		payload := strings.TrimSpace(trimmed[index+1:])

		switch ruleType {
		case "DOMAIN":
			return domain == strings.TrimSuffix(strings.ToLower(payload), ".")
		case "DOMAIN-SUFFIX":
			return domainSuffixMatch(domain, payload)
		default:
			return false
		}
	}

	lowered := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(lowered, "+."):
		return domainSuffixMatch(domain, lowered[2:])
	case strings.HasPrefix(lowered, "."):
		return domainSuffixMatch(domain, lowered[1:])
	default:
		// 裸域：按后缀匹配（与内核新版语义一致）
		return domainSuffixMatch(domain, lowered)
	}
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
// geosite: / rule-set: / regexp: 这些需要内核数据的形式只写一条「未展开」提示，并把 uncertain 置 true
// （它们可能命中，从而改变「是否走 fake-ip」的结论）。
func matchFakeIPFilter(filters []string, domain string) (string, []LocalizedText, bool) {
	notes := []LocalizedText{}
	uncertain := false

	for _, raw := range filters {
		pattern := strings.ToLower(strings.TrimSpace(raw))
		if pattern == "" {
			continue
		}

		if strings.Contains(pattern, ":") {
			notes = append(notes, LocalizedText{Code: dnsNoteFilterUnresolved, Arg: raw})
			uncertain = true
			continue
		}

		if fakeIPFilterMatch(pattern, domain) {
			// 已经命中一条可判定的规则，结论确定，不再受未展开条目影响
			return raw, notes, false
		}
	}

	return "", notes, uncertain
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
