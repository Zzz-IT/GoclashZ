package clash

import (
	"encoding/json"
	"fmt"
	"goclashz/core/utils"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// 定义全局读写锁，保护伴生规则文件的读写安全
var rulesMutex sync.RWMutex

// 🛡️ 终极防线 1：Clash 官方与 Meta (mihomo) 核心全量规则类型白名单
var validRuleTypes = map[string]bool{
	"DOMAIN":         true,
	"DOMAIN-SUFFIX":  true,
	"DOMAIN-KEYWORD": true,
	"DOMAIN-REGEX":   true,
	"GEOSITE":        true,
	"GEOIP":          true,
	"IP-CIDR":        true,
	"IP-CIDR6":       true,
	"IP-SUFFIX":      true,
	"IP-ASN":         true,
	"SRC-IP-CIDR":    true,
	"SRC-PORT":       true,
	"DST-PORT":       true,
	"IN-PORT":        true,
	"IN-TYPE":        true,
	"IN-USER":        true,
	"PROCESS-NAME":   true,
	"PROCESS-PATH":   true,
	"UID":            true,
	"NETWORK":        true,
	"DSCP":           true,
	"RULE-SET":       true,
	"AND":            true,
	"OR":             true,
	"NOT":            true,
	"SUB-RULE":       true,
	"MATCH":          true,
}

type CustomRuleSet struct {
	CustomRules []string `json:"customRules"`
}

func GetCustomRules(id string) ([]string, error) {
	// 🛡️ 防御路径穿越：强行提取纯文件名
	safeId := filepath.Base(filepath.Clean(id))
	if safeId == "." || safeId == "/" || safeId == "\\" {
		return nil, fmt.Errorf("非法的文件 ID 拒绝访问")
	}

	rulesMutex.RLock() // 加读锁
	defer rulesMutex.RUnlock()

	path := filepath.Join(utils.GetSubscriptionsDir(), safeId+"_rules.json")
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return []string{}, nil
	}

	var set CustomRuleSet
	// 修改：增加错误抛出，防止文件损坏时返回空切片导致前端覆写
	if err := json.Unmarshal(data, &set); err != nil {
		return nil, err
	}
	return set.CustomRules, nil
}

func SaveCustomRules(id string, rules []string) error {
	rulesMutex.Lock()
	defer rulesMutex.Unlock()

	if rules == nil {
		rules = []string{}
	}

	var sanitizedRules []string
	for _, r := range rules {
		cleanRule := strings.TrimSpace(r)
		if cleanRule == "" {
			continue
		}

		parts := strings.Split(cleanRule, ",")

		// 基础清理与遍历检查空缺
		var cleanedParts []string
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed == "" {
				return fmt.Errorf("格式拒绝：逗号之间不允许存在空缺值")
			}
			cleanedParts = append(cleanedParts, trimmed)
		}

		if len(cleanedParts) < 2 {
			return fmt.Errorf("格式拒绝：缺少逗号分隔，且至少需要两段")
		}

		// 🛡️ 终极防线 2：绝对白名单校验（自动容错用户的输入大小写，统一转为大写对比）
		ruleType := strings.ToUpper(cleanedParts[0])
		if !validRuleTypes[ruleType] {
			return fmt.Errorf("格式拒绝：[%s] 不是合法的 Clash 规则类型。支持的类型如 DOMAIN, IP-CIDR, MATCH 等", cleanedParts[0])
		}

		// 🛡️ 终极防线 3：动态语义段数校验
		// MATCH 规则特殊，只有两段 (MATCH,DIRECT) 或带附加参数 (MATCH,DIRECT,no-resolve)
		if ruleType == "MATCH" {
			if len(cleanedParts) < 2 {
				return fmt.Errorf("格式拒绝：MATCH 规则至少需要2段 (例如 MATCH,DIRECT)")
			}
		} else {
			// 除 MATCH 以外的所有规则，绝大多数必须至少 3 段 (类型, 载荷, 策略)
			if len(cleanedParts) < 3 {
				return fmt.Errorf("格式拒绝：[%s] 规则至少需要3段 (类型,目标,策略)，例如 %s,example.com,DIRECT", ruleType, ruleType)
			}
		}

		// 强制将类型转为标准大写，并重组写入
		cleanedParts[0] = ruleType
		sanitizedRules = append(sanitizedRules, strings.Join(cleanedParts, ","))
	}

	// 🛡️ 防御路径穿越：强行提取纯文件名
	safeId := filepath.Base(filepath.Clean(id))
	if safeId == "." || safeId == "/" || safeId == "\\" {
		return fmt.Errorf("非法的文件 ID 拒绝访问")
	}

	path := filepath.Join(utils.GetSubscriptionsDir(), safeId+"_rules.json")
	tmpPath := path + ".tmp"

	// 使用校验并清洗过的数据落盘
	set := CustomRuleSet{CustomRules: sanitizedRules}
	data, _ := json.Marshal(set)

	// 原子操作写入
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// GetOriginalRules 读取底层 YAML 文件，提取内置规则
func GetOriginalRules(id string) ([]string, error) {
	// 🛡️ 防御路径穿越：强行提取纯文件名
	safeId := filepath.Base(filepath.Clean(id))
	if safeId == "." || safeId == "/" || safeId == "\\" {
		return nil, fmt.Errorf("非法的文件 ID 拒绝访问")
	}

	yamlPath := filepath.Join(utils.GetSubscriptionsDir(), safeId+".yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, err
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	var rules []string
	if rawRules, ok := root["rules"].([]interface{}); ok {
		for _, r := range rawRules {
			if strRule, ok := r.(string); ok {
				rules = append(rules, strRule)
			}
		}
	}
	return rules, nil
}

// SyncRulesFromYaml 强制将原始 YAML 中的规则同步(覆盖)到用户的伴生 JSON 中
func SyncRulesFromYaml(id string) error {
	originalRules, err := GetOriginalRules(id)
	if err != nil {
		return err
	}
	// 兜底：如果机场配置文件连规则都没有，给一条默认的放行规则防止内核崩溃
	if len(originalRules) == 0 {
		originalRules = []string{"MATCH,DIRECT"}
	}
	return SaveCustomRules(id, originalRules)
}
