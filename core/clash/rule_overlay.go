//go:build windows

package clash

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"goclashz/core/utils"

	"gopkg.in/yaml.v3"
)

var ruleStorageMu sync.Mutex

// WithRuleStorageLock 安全执行需要锁定规则存储库的操作
func WithRuleStorageLock(fn func() error) error {
	ruleStorageMu.Lock()
	defer ruleStorageMu.Unlock()
	return fn()
}

type RuleOverlay struct {
	Add    []string `json:"add"`    // 附加规则，运行时插到最前面
	Delete []string `json:"delete"` // 附加删除，运行时匹配删除订阅规则
}

type RulePageData struct {
	ConfigType        string   `json:"configType"`        // local / remote
	SubscriptionRules []string `json:"subscriptionRules"` // 远程 origin rules；本地为空
	LocalRules        []string `json:"localRules"`        // 本地 YAML rules；远程为空
	AddRules          []string `json:"addRules"`          // 远程 overlay.add
	DeleteRules       []string `json:"deleteRules"`       // 远程 overlay.delete
	EffectiveRules    []string `json:"effectiveRules"`    // 实际导入内核前 rules，可用于诊断
}

func SubscriptionsDir() string {
	return utils.GetSubscriptionsDir()
}

func OriginDir() string {
	return filepath.Join(utils.GetSubscriptionsDir(), "origin")
}

func WorkingConfigPath(id string) (string, error) {
	safeId, err := utils.SanitizeFilename(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(SubscriptionsDir(), safeId+".yaml"), nil
}

func OriginConfigPath(id string) (string, error) {
	safeId, err := utils.SanitizeFilename(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(OriginDir(), safeId+".yaml"), nil
}

func RuleOverlayPath(id string) (string, error) {
	safeId, err := utils.SanitizeFilename(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(SubscriptionsDir(), safeId+"_overlay.json"), nil
}

// LoadRuleOverlay 读取 overlay 规则，如果文件损坏或不存在，返回空 overlay
func LoadRuleOverlay(id string) (RuleOverlay, error) {
	path, err := RuleOverlayPath(id)
	if err != nil {
		return RuleOverlay{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		// Not exist is not an error for us, just return empty
		return RuleOverlay{Add: []string{}, Delete: []string{}}, nil
	}

	var overlay RuleOverlay
	if err := json.Unmarshal(data, &overlay); err != nil {
		// If corrupted, return empty overlay
		return RuleOverlay{Add: []string{}, Delete: []string{}}, nil
	}

	if overlay.Add == nil {
		overlay.Add = []string{}
	}
	if overlay.Delete == nil {
		overlay.Delete = []string{}
	}

	return overlay, nil
}

func SaveRuleOverlay(id string, overlay RuleOverlay) error {
	path, err := RuleOverlayPath(id)
	if err != nil {
		return err
	}

	data, err := json.Marshal(overlay)
	if err != nil {
		return err
	}

	return utils.WriteFileAtomic(path, data, 0644)
}

// EnsureEmptyOverlay 确保指定的订阅存在一个空的 overlay，如果不存就创建
func EnsureEmptyOverlay(id string) error {
	path, err := RuleOverlayPath(id)
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}

	return SaveRuleOverlay(id, RuleOverlay{Add: []string{}, Delete: []string{}})
}

// ExtractRulesFromRootPublic 从解析好的 YAML root 中提取 rules 数组
func ExtractRulesFromRootPublic(root map[string]interface{}) []string {
	var rules []string
	if rawRules, ok := root["rules"].([]interface{}); ok {
		for _, r := range rawRules {
			if strRule, ok := r.(string); ok {
				rules = append(rules, strRule)
			}
		}
	}
	return rules
}

// ReadYamlRoot 读取任意指定路径的 yaml root
func ReadYamlRoot(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	return root, nil
}

// ReadWorkingRootWithRecovery 读取 working yaml，如果损坏尝试从 origin 恢复
func ReadWorkingRootWithRecovery(id string) (map[string]interface{}, error) {
	workingPath, err := WorkingConfigPath(id)
	if err != nil {
		return nil, err
	}

	root, err := ReadYamlRoot(workingPath)
	if err == nil {
		return root, nil
	}

	originPath, originErr := OriginConfigPath(id)
	if originErr != nil {
		return nil, err
	}

	originRoot, originReadErr := ReadYamlRoot(originPath)
	if originReadErr != nil {
		return nil, fmt.Errorf("工作配置损坏且 origin 无法恢复: working=%v origin=%v", err, originReadErr)
	}

	originData, readErr := os.ReadFile(originPath)
	if readErr != nil {
		return nil, readErr
	}

	if writeErr := utils.WriteFileAtomic(workingPath, originData, 0644); writeErr != nil {
		return nil, fmt.Errorf("从 origin 恢复 working 失败: %w", writeErr)
	}

	return originRoot, nil
}

// readYamlRootFromOrigin 从 origin 备份读取
func readYamlRootFromOrigin(id string) (map[string]interface{}, error) {
	path, err := OriginConfigPath(id)
	if err != nil {
		return nil, err
	}
	return ReadYamlRoot(path)
}

// NormalizeRule 规范化规则，去除空格并将第一个类型转为大写，便于对比
func NormalizeRule(rule string) string {
	parts := strings.Split(rule, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) > 0 {
		parts[0] = strings.ToUpper(parts[0])
	}
	return strings.Join(parts, ",")
}

// ApplyRuleOverlay 将 overlay 中的增删应用到 origin rules
func ApplyRuleOverlay(baseRules []string, overlay RuleOverlay) []string {
	deleteSet := map[string]bool{}
	for _, r := range overlay.Delete {
		deleteSet[NormalizeRule(r)] = true
	}

	finalRules := make([]string, 0, len(overlay.Add)+len(baseRules))
	finalRules = append(finalRules, overlay.Add...)

	for _, r := range baseRules {
		if !deleteSet[NormalizeRule(r)] {
			finalRules = append(finalRules, r)
		}
	}
	return finalRules
}

// ApplyDeleteOnly 仅对规则列表应用删除操作
func ApplyDeleteOnly(baseRules []string, deleteRules []string) []string {
	deleteSet := map[string]bool{}
	for _, r := range deleteRules {
		deleteSet[NormalizeRule(r)] = true
	}

	kept := make([]string, 0, len(baseRules))
	for _, r := range baseRules {
		if !deleteSet[NormalizeRule(r)] {
			kept = append(kept, r)
		}
	}
	return kept
}

// BuildRuntimeRules 根据订阅类型合并生成最终运行时规则
// 本地：working.rules
// 远程：overlay.add + (working.rules - overlay.delete)
func BuildRuntimeRules(id string, workingRoot map[string]interface{}) ([]string, error) {
	item, _ := FindSubIndexByID(id)

	baseRules := ExtractRulesFromRootPublic(workingRoot)

	// 如果是 local 配置，直接返回 working 中的 rules
	if item.Type != "remote" {
		return baseRules, nil
	}

	// 远程配置读取 overlay，并在 working baseRules 上应用
	overlay, err := LoadRuleOverlay(id)
	if err != nil {
		return nil, err
	}

	return ApplyRuleOverlay(baseRules, overlay), nil
}
