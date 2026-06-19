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
	// 提取所有的 proxies
	validTargets := map[string]bool{
		"DIRECT": true,
		"REJECT": true,
		"GLOBAL": true,
		"COMPAT": true,
		"PASS":   true,
	}

	// 1. 记录所有的代理节点名字
	if proxiesNode, ok := root["proxies"].([]interface{}); ok {
		for _, p := range proxiesNode {
			if proxy, isMap := p.(map[string]interface{}); isMap {
				if name, _ := proxy["name"].(string); name != "" {
					validTargets[name] = true
				}
			}
		}
	}

	// 2. 记录所有的代理组名字
	if groupsNode, ok := root["proxy-groups"].([]interface{}); ok {
		for _, g := range groupsNode {
			if group, isMap := g.(map[string]interface{}); isMap {
				if name, _ := group["name"].(string); name != "" {
					validTargets[name] = true
				}
			}
		}
	}

	// 3. 记录所有的 proxy-providers 名字
	if providersNode, ok := root["proxy-providers"].(map[string]interface{}); ok {
		for name := range providersNode {
			validTargets[name] = true
		}
	}

	// 4. 记录所有的 rule-providers 名字
	validRuleProviders := map[string]bool{}
	if ruleProvidersNode, ok := root["rule-providers"].(map[string]interface{}); ok {
		for name := range ruleProvidersNode {
			validRuleProviders[name] = true
		}
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
							if !validTargets[providerName] {
								return fmt.Errorf("策略组 [%s] 的 use 引用了不存在的 provider: %s", groupName, providerName)
							}
						}
					}
				}

				// 检查 proxies
				if pList, ok := group["proxies"].([]interface{}); ok {
					for _, p := range pList {
						if proxyName, ok := p.(string); ok {
							if !validTargets[proxyName] {
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
					
					// MATCH 规则目标
					if ruleType == "MATCH" {
						target := parts[1]
						if !validTargets[target] {
							return fmt.Errorf("规则 [%s] 引用了不存在的策略组/节点: %s", ruleStr, target)
						}
					} else if ruleType == "RULE-SET" {
						// RULE-SET 引用校验
						if len(parts) >= 3 {
							provider := parts[1]
							target := parts[2]
							if !validRuleProviders[provider] {
								return fmt.Errorf("规则 [%s] 引用了不存在的 rule-provider: %s", ruleStr, provider)
							}
							if !validTargets[target] {
								return fmt.Errorf("规则 [%s] 引用了不存在的策略组/节点: %s", ruleStr, target)
							}
						}
					} else if len(parts) >= 3 {
						// 其他规则目标 (第三段)
						target := parts[2]
						// 有些规则可能有 no-resolve 等附加参数，但第三个总是策略
						if !validTargets[target] {
							return fmt.Errorf("规则 [%s] 引用了不存在的策略组/节点: %s", ruleStr, target)
						}
					}
				}
			}
		}
	}

	return nil
}
