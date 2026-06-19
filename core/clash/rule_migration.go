//go:build windows

package clash

import (
	"encoding/json"
	"os"
	"path/filepath"

	"goclashz/core/utils"

	"gopkg.in/yaml.v3"
)

// EnsureRuleStorageMigrated 为单个配置进行迁移兜底检查
func EnsureRuleStorageMigrated(id string) error {
	rulesPath := filepath.Join(SubscriptionsDir(), id+"_rules.json")
	if _, err := os.Stat(rulesPath); os.IsNotExist(err) {
		return nil // 已经不存在，或者已经迁移并删除了
	}

	IndexLock.RLock()
	var itemType string
	for _, item := range SubIndex {
		if item.ID == id {
			itemType = item.Type
			break
		}
	}
	IndexLock.RUnlock()

	if itemType == "local" || itemType == "" {
		return migrateLocalRules(id, rulesPath)
	}

	return migrateRemoteRules(id, rulesPath)
}

// MigrateRuleStorageV2 在应用启动时进行全局批量迁移
func MigrateRuleStorageV2() error {
	IndexLock.RLock()
	ids := make([]string, 0, len(SubIndex))
	for _, item := range SubIndex {
		ids = append(ids, item.ID)
	}
	IndexLock.RUnlock()

	var firstErr error
	for _, id := range ids {
		if err := EnsureRuleStorageMigrated(id); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// migrateLocalRules 迁移本地配置：将 JSON 的 rules 覆盖写入 YAML rules
func migrateLocalRules(id, legacyRulesPath string) error {
	workingPath, _ := WorkingConfigPath(id)
	originPath, _ := OriginConfigPath(id)

	root, err := ReadYamlRoot(workingPath)
	if err != nil {
		return err
	}

	oldRules, err := readLegacyRulesJson(legacyRulesPath)
	if err != nil {
		return err
	}

	if oldRules != nil {
		root["rules"] = oldRules

		out, err := yaml.Marshal(root)
		if err != nil {
			return err
		}

		if err := utils.WriteFileAtomic(workingPath, out, 0644); err != nil {
			return err
		}
	}

	// 确保 origin 备份存在
	if _, err := os.Stat(originPath); os.IsNotExist(err) {
		data, _ := os.ReadFile(workingPath)
		utils.WriteFileAtomic(originPath, data, 0644)
	}

	return deleteLegacyRulesJson(legacyRulesPath)
}

// migrateRemoteRules 迁移远程配置：生成 origin，对比 rules 生成 overlay
func migrateRemoteRules(id, legacyRulesPath string) error {
	workingPath, _ := WorkingConfigPath(id)
	originPath, _ := OriginConfigPath(id)

	if _, err := os.Stat(originPath); os.IsNotExist(err) {
		data, _ := os.ReadFile(workingPath)
		utils.WriteFileAtomic(originPath, data, 0644)
	}

	originRoot, err := ReadYamlRoot(originPath)
	if err != nil {
		return err
	}

	originRules := ExtractRulesFromRootPublic(originRoot)

	oldRules, err := readLegacyRulesJson(legacyRulesPath)
	if err != nil {
		return err
	}

	if oldRules == nil {
		EnsureEmptyOverlay(id)
		return deleteLegacyRulesJson(legacyRulesPath)
	}

	originSet := map[string]bool{}
	for _, r := range originRules {
		originSet[NormalizeRule(r)] = true
	}

	oldSet := map[string]bool{}
	for _, r := range oldRules {
		oldSet[NormalizeRule(r)] = true
	}

	overlay := RuleOverlay{
		Add:    []string{},
		Delete: []string{},
	}

	for _, r := range oldRules {
		if !originSet[NormalizeRule(r)] {
			overlay.Add = append(overlay.Add, r)
		}
	}

	for _, r := range originRules {
		if !oldSet[NormalizeRule(r)] {
			overlay.Delete = append(overlay.Delete, r)
		}
	}

	if err := SaveRuleOverlay(id, overlay); err != nil {
		return err
	}

	return deleteLegacyRulesJson(legacyRulesPath)
}

func readLegacyRulesJson(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var set CustomRuleSet
	if err := json.Unmarshal(data, &set); err != nil {
		return nil, nil // corrupted, ignore and return nil
	}
	return set.CustomRules, nil
}

func deleteLegacyRulesJson(path string) error {
	return os.Remove(path)
}
