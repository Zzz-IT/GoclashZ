//go:build windows

package clash

import (
	"fmt"
	"strings"
)

type RuleTypeOption struct {
	Value        string `json:"value"`
	Label        string `json:"label"`
	Count        int    `json:"count"`
	NeedPayload  bool   `json:"needPayload"`
	NeedPolicy   bool   `json:"needPolicy"`
	PayloadLabel string `json:"payloadLabel"`
	PayloadHint  string `json:"payloadHint"`
	Example      string `json:"example"`
}

type PolicyOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type RuleFormOptions struct {
	Types    []RuleTypeOption `json:"types"`
	Policies []PolicyOption   `json:"policies"`
}

type BuildRuleRequest struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Policy  string `json:"policy"`
}

var defaultRuleTypeOrder = []string{
	"DOMAIN-SUFFIX",
	"DOMAIN",
	"DOMAIN-KEYWORD",
	"GEOSITE",
	"RULE-SET",
	"IP-CIDR",
	"IP-CIDR6",
	"GEOIP",
	"PROCESS-NAME",
	"PROCESS-PATH",
	"PROCESS-PATH-REGEX",
	"DST-PORT",
	"SRC-IP-CIDR",
	"SRC-PORT",
	"IN-PORT",
	"IN-TYPE",
	"NETWORK",
	"UID",
	"MATCH",
}

var ruleTypeMeta = map[string]RuleTypeOption{
	"DOMAIN": {
		Value:        "DOMAIN",
		Label:        "DOMAIN",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "域名",
		PayloadHint:  "example.com",
		Example:      "DOMAIN,example.com,DIRECT",
	},
	"DOMAIN-SUFFIX": {
		Value:        "DOMAIN-SUFFIX",
		Label:        "DOMAIN-SUFFIX",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "域名后缀",
		PayloadHint:  "example.com",
		Example:      "DOMAIN-SUFFIX,example.com,DIRECT",
	},
	"DOMAIN-KEYWORD": {
		Value:        "DOMAIN-KEYWORD",
		Label:        "DOMAIN-KEYWORD",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "关键词",
		PayloadHint:  "google",
		Example:      "DOMAIN-KEYWORD,google,DIRECT",
	},
	"GEOSITE": {
		Value:        "GEOSITE",
		Label:        "GEOSITE",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "规则集",
		PayloadHint:  "geolocation-cn",
		Example:      "GEOSITE,geolocation-cn,DIRECT",
	},
	"RULE-SET": {
		Value:        "RULE-SET",
		Label:        "RULE-SET",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "Rule Provider",
		PayloadHint:  "reject",
		Example:      "RULE-SET,reject,DIRECT",
	},
	"IP-CIDR": {
		Value:        "IP-CIDR",
		Label:        "IP-CIDR",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "CIDR",
		PayloadHint:  "1.1.1.1/32",
		Example:      "IP-CIDR,1.1.1.1/32,DIRECT",
	},
	"IP-CIDR6": {
		Value:        "IP-CIDR6",
		Label:        "IP-CIDR6",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "IPv6 CIDR",
		PayloadHint:  "2001:db8::/32",
		Example:      "IP-CIDR6,2001:db8::/32,DIRECT",
	},
	"GEOIP": {
		Value:        "GEOIP",
		Label:        "GEOIP",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "国家/地区",
		PayloadHint:  "CN",
		Example:      "GEOIP,CN,DIRECT",
	},
	"PROCESS-NAME": {
		Value:        "PROCESS-NAME",
		Label:        "PROCESS-NAME",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "进程名",
		PayloadHint:  "chrome.exe",
		Example:      "PROCESS-NAME,chrome.exe,DIRECT",
	},
	"PROCESS-PATH": {
		Value:        "PROCESS-PATH",
		Label:        "PROCESS-PATH",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "进程路径",
		PayloadHint:  "C:\\Program Files\\chrome.exe",
		Example:      "PROCESS-PATH,C:\\Program Files\\chrome.exe,DIRECT",
	},
	"PROCESS-PATH-REGEX": {
		Value:        "PROCESS-PATH-REGEX",
		Label:        "PROCESS-PATH-REGEX",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "路径正则",
		PayloadHint:  ".*chrome\\.exe",
		Example:      "PROCESS-PATH-REGEX,.*chrome\\.exe,DIRECT",
	},
	"DST-PORT": {
		Value:        "DST-PORT",
		Label:        "DST-PORT",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "端口",
		PayloadHint:  "443",
		Example:      "DST-PORT,443,DIRECT",
	},
	"SRC-PORT": {
		Value:        "SRC-PORT",
		Label:        "SRC-PORT",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "源端口",
		PayloadHint:  "7890",
		Example:      "SRC-PORT,7890,DIRECT",
	},
	"SRC-IP-CIDR": {
		Value:        "SRC-IP-CIDR",
		Label:        "SRC-IP-CIDR",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "源 IP",
		PayloadHint:  "192.168.1.0/24",
		Example:      "SRC-IP-CIDR,192.168.1.0/24,DIRECT",
	},
	"IN-PORT": {
		Value:        "IN-PORT",
		Label:        "IN-PORT",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "入站端口",
		PayloadHint:  "7890",
		Example:      "IN-PORT,7890,DIRECT",
	},
	"IN-TYPE": {
		Value:        "IN-TYPE",
		Label:        "IN-TYPE",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "入站类型",
		PayloadHint:  "SOCKS",
		Example:      "IN-TYPE,SOCKS,DIRECT",
	},
	"NETWORK": {
		Value:        "NETWORK",
		Label:        "NETWORK",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "网络类型",
		PayloadHint:  "tcp",
		Example:      "NETWORK,tcp,DIRECT",
	},
	"UID": {
		Value:        "UID",
		Label:        "UID",
		NeedPayload:  true,
		NeedPolicy:   true,
		PayloadLabel: "用户ID",
		PayloadHint:  "1000",
		Example:      "UID,1000,DIRECT",
	},
	"MATCH": {
		Value:        "MATCH",
		Label:        "MATCH",
		NeedPayload:  false,
		NeedPolicy:   true,
		PayloadLabel: "",
		PayloadHint:  "",
		Example:      "MATCH,DIRECT",
	},
}

func GetEffectiveRulesForForm(id string) ([]string, error) {
	root, err := ReadWorkingRootWithRecovery(id)
	if err != nil {
		return nil, err
	}
	return BuildRuntimeRules(id, root)
}

func CountRuleTypes(rules []string) map[string]int {
	counts := map[string]int{}
	for _, r := range rules {
		parts := strings.Split(r, ",")
		if len(parts) == 0 {
			continue
		}
		t := strings.ToUpper(strings.TrimSpace(parts[0]))
		if t != "" {
			counts[t]++
		}
	}
	return counts
}

func ExtractPolicyOptions(root map[string]interface{}) []PolicyOption {
	seen := map[string]bool{}
	var result []PolicyOption

	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		result = append(result, PolicyOption{
			Value: name,
			Label: name,
		})
	}

	add("DIRECT")
	add("REJECT")
	add("REJECT-DROP")
	add("PASS")

	if groups, ok := root["proxy-groups"].([]interface{}); ok {
		for _, g := range groups {
			group, ok := g.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := group["name"].(string)
			add(name)
		}
	}

	return result
}

func BuildRuleFromForm(req BuildRuleRequest) (string, error) {
	ruleType := strings.ToUpper(strings.TrimSpace(req.Type))
	payload := strings.TrimSpace(req.Payload)
	policy := strings.TrimSpace(req.Policy)

	meta, ok := ruleTypeMeta[ruleType]
	if !ok {
		return "", fmt.Errorf("不支持的规则类型: %s", ruleType)
	}

	var parts []string
	parts = append(parts, ruleType)

	if meta.NeedPayload {
		if payload == "" {
			return "", fmt.Errorf("%s 不能为空", meta.PayloadLabel)
		}
		parts = append(parts, payload)
	}

	if meta.NeedPolicy {
		if policy == "" {
			return "", fmt.Errorf("策略不能为空")
		}
		parts = append(parts, policy)
	}

	rule := strings.Join(parts, ",")
	return SanitizeRuleLine(rule)
}

// GetRuleFormOptions 获取弹窗渲染所需选项数据
func GetRuleFormOptionsData(id string) (RuleFormOptions, error) {
	var opts RuleFormOptions

	root, err := ReadWorkingRootWithRecovery(id)
	if err != nil {
		return opts, err
	}

	// Read effective rules to count types
	rules, err := BuildRuntimeRules(id, root)
	if err != nil {
		return opts, err
	}
	counts := CountRuleTypes(rules)

	opts.Policies = ExtractPolicyOptions(root)

	// Combine default order with dynamically encountered types, and sort by count
	var types []RuleTypeOption
	typeSet := map[string]bool{}

	for _, t := range defaultRuleTypeOrder {
		if meta, ok := ruleTypeMeta[t]; ok {
			meta.Count = counts[t]
			types = append(types, meta)
			typeSet[t] = true
		}
	}

	for t, count := range counts {
		if !typeSet[t] {
			if meta, ok := ruleTypeMeta[t]; ok {
				meta.Count = count
				types = append(types, meta)
				typeSet[t] = true
			} else {
				// Fallback generic info for unseen types
				types = append(types, RuleTypeOption{
					Value:        t,
					Label:        t,
					Count:        count,
					NeedPayload:  true,
					NeedPolicy:   true,
					PayloadLabel: "内容",
					PayloadHint:  "",
				})
			}
		}
	}

	// Sort types: highest count first. 
	// (Using simple bubble sort for short array, but let's use sort slice later if needed. 
	// Since order is important, we just bubble sort by count descending, keeping default order for ties.)
	for i := 0; i < len(types); i++ {
		for j := i + 1; j < len(types); j++ {
			if types[j].Count > types[i].Count {
				types[i], types[j] = types[j], types[i]
			}
		}
	}

	opts.Types = types
	return opts, nil
}
