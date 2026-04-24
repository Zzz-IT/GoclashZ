package clash

import (
	"encoding/json"
	"goclashz/core/utils"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// 定义全局读写锁，保护伴生规则文件的读写安全
var rulesMutex sync.RWMutex

type CustomRuleSet struct {
	CustomRules []string `json:"customRules"`
}

func GetCustomRules(id string) ([]string, error) {
	rulesMutex.RLock() // 加读锁
	defer rulesMutex.RUnlock()

	path := filepath.Join(utils.GetSubscriptionsDir(), id+"_rules.json")
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
	rulesMutex.Lock() // 加写锁
	defer rulesMutex.Unlock()

	path := filepath.Join(utils.GetSubscriptionsDir(), id+"_rules.json")
	set := CustomRuleSet{CustomRules: rules}
	// 修改：去掉 MarshalIndent，改用 Marshal 压缩文件体积和提升 IO 速度
	data, _ := json.Marshal(set)
	return os.WriteFile(path, data, 0644)
}

// GetOriginalRules 读取底层 YAML 文件，提取内置规则
func GetOriginalRules(id string) ([]string, error) {
	yamlPath := filepath.Join(utils.GetSubscriptionsDir(), id+".yaml")
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
