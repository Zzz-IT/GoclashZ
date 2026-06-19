//go:build windows

package clash

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"goclashz/core/utils"

	"gopkg.in/yaml.v3"
)

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
func ApplyRuleOverlay(originRules []string, overlay RuleOverlay) []string {
	deleteSet := map[string]bool{}
	for _, r := range overlay.Delete {
		deleteSet[NormalizeRule(r)] = true
	}

	kept := make([]string, 0, len(originRules))
	for _, r := range originRules {
		if !deleteSet[NormalizeRule(r)] {
			kept = append(kept, r)
		}
	}

	result := make([]string, 0, len(overlay.Add)+len(kept))
	result = append(result, overlay.Add...)
	result = append(result, kept...)
	return result
}

// BuildRuntimeRules 运行时核心规则生成
func BuildRuntimeRules(id string, workingRoot map[string]interface{}) ([]string, error) {
	IndexLock.RLock()
	var itemType string
	for _, item := range SubIndex {
		if item.ID == id {
			itemType = item.Type
			break
		}
	}
	IndexLock.RUnlock()

	// local config directly uses rules from its YAML
	if itemType == "local" || itemType == "" {
		return ExtractRulesFromRootPublic(workingRoot), nil
	}

	// remote config uses origin + overlay
	originRoot, err := readYamlRootFromOrigin(id)
	if err != nil {
		// if origin is missing or corrupted, fallback to current working root rules
		return ExtractRulesFromRootPublic(workingRoot), nil
	}

	originRules := ExtractRulesFromRootPublic(originRoot)

	overlay, err := LoadRuleOverlay(id)
	if err != nil {
		return nil, err
	}

	return ApplyRuleOverlay(originRules, overlay), nil
}
