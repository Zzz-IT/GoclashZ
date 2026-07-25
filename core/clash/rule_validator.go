//go:build windows

package clash

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidateClashReferencesBytes 解析并校验配置引用的合法性
func ValidateClashReferencesBytes(data []byte) error {
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("YAML 格式错误: %v", err)
	}
	return ValidateClashReferences(root)
}

// ValidateClashReferences 校验 clash 配置引用的合法性，确保删除代理或策略组后能够捕获错误
func ValidateClashReferences(root map[string]interface{}) error {
	builtinPolicies := map[string]bool{
		"DIRECT":      true,
		"REJECT":      true,
		"REJECT-DROP": true,
		"PASS":        true,
		"GLOBAL":      true,
		"COMPAT":      true,
	}

	proxyNames := map[string]bool{}
	groupNames := map[string]bool{}
	proxyProviderNames := map[string]bool{}
	ruleProviderNames := map[string]bool{}

	// 1. 记录所有的代理节点名字
	if proxiesNode, ok := root["proxies"].([]interface{}); ok {
		for _, p := range proxiesNode {
			if proxy, isMap := p.(map[string]interface{}); isMap {
				if name, _ := proxy["name"].(string); name != "" {
					proxyNames[name] = true
				}
			}
		}
	}

	// 2. 记录所有的代理组名字
	if groupsNode, ok := root["proxy-groups"].([]interface{}); ok {
		for _, g := range groupsNode {
			if group, isMap := g.(map[string]interface{}); isMap {
				if name, _ := group["name"].(string); name != "" {
					groupNames[name] = true
				}
			}
		}
	}

	// 3. 记录所有的 proxy-providers 名字
	if providersNode, ok := root["proxy-providers"].(map[string]interface{}); ok {
		for name := range providersNode {
			proxyProviderNames[name] = true
		}
	}

	// 4. 记录所有的 rule-providers 名字
	if ruleProvidersNode, ok := root["rule-providers"].(map[string]interface{}); ok {
		for name := range ruleProvidersNode {
			ruleProviderNames[name] = true
		}
	}

	isValidPolicyTarget := func(name string) bool {
		return builtinPolicies[name] || proxyNames[name] || groupNames[name]
	}

	// 5. 校验所有的 proxy-groups 引用
	if groupsNode, ok := root["proxy-groups"].([]interface{}); ok {
		for _, g := range groupsNode {
			if group, isMap := g.(map[string]interface{}); isMap {
				groupName, _ := group["name"].(string)

				// 检查 use (proxy-providers)
				if useNode, ok := group["use"].([]interface{}); ok {
					for _, u := range useNode {
						if providerName, ok := u.(string); ok {
							if !proxyProviderNames[providerName] {
								return fmt.Errorf("策略组 [%s] 的 use 引用了不存在的 provider: %s", groupName, providerName)
							}
						}
					}
				}

				// 检查 proxies
				if pList, ok := group["proxies"].([]interface{}); ok {
					for _, p := range pList {
						if proxyName, ok := p.(string); ok {
							if !isValidPolicyTarget(proxyName) {
								return fmt.Errorf("策略组 [%s] 引用了不存在的节点/策略组: %s", groupName, proxyName)
							}
						}
					}
				}
			}
		}
	}

	// 6. 校验规则 (rules) 的目标
	if rulesNode, ok := root["rules"].([]interface{}); ok {
		for _, r := range rulesNode {
			if ruleStr, ok := r.(string); ok {
				parts := strings.Split(ruleStr, ",")
				for i := range parts {
					parts[i] = strings.TrimSpace(parts[i])
				}

				if len(parts) >= 2 {
					ruleType := strings.ToUpper(parts[0])

					if ruleType == "MATCH" || ruleType == "FINAL" {
						target := parts[1]
						if !isValidPolicyTarget(target) {
							return fmt.Errorf("规则 [%s] 引用了不存在的策略组/节点: %s", ruleStr, target)
						}
					} else if ruleType == "RULE-SET" {
						if len(parts) >= 3 {
							provider := parts[1]
							target := parts[2]
							if !ruleProviderNames[provider] {
								return fmt.Errorf("规则 [%s] 引用了不存在的 rule-provider: %s", ruleStr, provider)
							}
							if !isValidPolicyTarget(target) {
								return fmt.Errorf("规则 [%s] 引用了不存在的策略组/节点: %s", ruleStr, target)
							}
						}
					} else if ruleType == "AND" || ruleType == "OR" || ruleType == "NOT" || ruleType == "SUB-RULE" {
						// 复杂规则跳过深度校验
						continue
					} else if len(parts) >= 3 {
						target := parts[2]
						if !isValidPolicyTarget(target) {
							return fmt.Errorf("规则 [%s] 引用了不存在的策略组/节点: %s", ruleStr, target)
						}
					}
				}
			}
		}
	}

	return nil
}

// SanitizeRuleLine 规范化和校验单条规则
func SanitizeRuleLine(rule string) (string, error) {
	rule = NormalizeRule(rule)
	if rule == "" {
		return "", fmt.Errorf("规则不可为空")
	}

	rawParts := strings.Split(rule, ",")
	parts := make([]string, 0, len(rawParts))
	for _, p := range rawParts {
		p = strings.TrimSpace(p)
		if p == "" {
			return "", fmt.Errorf("规则格式无效，存在空段: %s", rule)
		}
		parts = append(parts, p)
	}

	ruleType := strings.ToUpper(parts[0])
	parts[0] = ruleType

	switch ruleType {
	case "MATCH", "FINAL":
		if len(parts) != 2 {
			return "", fmt.Errorf("%s 规则格式应为 %s,策略", ruleType, ruleType)
		}
	case "AND", "OR", "NOT", "SUB-RULE":
		if len(parts) < 2 {
			return "", fmt.Errorf("%s 规则结构不完整", ruleType)
		}
	default:
		if len(parts) < 3 {
			return "", fmt.Errorf("%s 规则格式应至少为 类型,内容,策略", ruleType)
		}
	}

	return strings.Join(parts, ","), nil
}

// SanitizeRuleList 规范化和校验多条规则
func SanitizeRuleList(rules []string) ([]string, error) {
	var valid []string
	for _, r := range rules {
		cleaned, err := SanitizeRuleLine(r)
		if err != nil {
			return nil, err
		}
		valid = append(valid, cleaned)
	}
	return valid, nil
}
