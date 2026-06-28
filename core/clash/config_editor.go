//go:build windows

package clash

import (
	"fmt"
	"goclashz/core/utils"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ConfigTextResult 配置文本读取结果
type ConfigTextResult struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"` // local / remote / runtime
	Content string `json:"content"`
	Path    string `json:"path"`
}

func resolveConfigType(id string) string {
	if item, ok := FindSubIndexByID(id); ok {
		return item.Type
	}
	if id == MainConfigID || id == "" {
		return "runtime"
	}
	return "local"
}

// ReadConfigText 读取配置文件文本内容
func ReadConfigText(id string) (ConfigTextResult, error) {
	normalizedID, configPath, err := ProfilePathByIDStrict(id)
	if err != nil {
		return ConfigTextResult{}, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return ConfigTextResult{}, fmt.Errorf("读取配置文件失败: %w", err)
	}

	return ConfigTextResult{
		ID:      normalizedID,
		Name:    filepath.Base(configPath),
		Type:    resolveConfigType(id),
		Content: string(data),
		Path:    configPath,
	}, nil
}

// SaveConfigText 保存配置文件文本内容
func SaveConfigText(id string, content string) error {
	return WithRuleStorageLock(func() error {
		item, _ := FindSubIndexByID(id)

		var root map[string]interface{}
		if err := yaml.Unmarshal([]byte(content), &root); err != nil {
			return fmt.Errorf("YAML 语法错误: %w", err)
		}

		if item.Type == "remote" {
			overlay, err := LoadRuleOverlay(id)
			if err != nil {
				return err
			}

			baseRules := ExtractRulesFromRootPublic(root)
			runtimeRules := ApplyRuleOverlay(baseRules, overlay)

			rootForValidation := make(map[string]interface{})
			for k, v := range root {
				rootForValidation[k] = v
			}
			rootForValidation["rules"] = runtimeRules

			if err := ValidateClashReferences(rootForValidation); err != nil {
				return fmt.Errorf("远程配置运行时引用完整性错误: %w", err)
			}
		} else {
			if err := ValidateClashReferences(root); err != nil {
				return fmt.Errorf("引用完整性错误: %w", err)
			}
		}

		_, configPath, err := ProfilePathByIDStrict(id)
		if err != nil {
			return err
		}

		if err := utils.WriteFileAtomic(configPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("保存配置文件失败: %w", err)
		}

		return nil
	})
}

// ValidateConfigText 校验 YAML 语法
func ValidateConfigText(content string) error {
	var root map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return fmt.Errorf("YAML 语法错误: %w", err)
	}
	if err := ValidateClashReferences(root); err != nil {
		return fmt.Errorf("引用完整性错误: %w", err)
	}
	return nil
}

// GetConfigFilePath 获取配置文件路径
func GetConfigFilePath(id string) string {
	_, configPath, err := ProfilePathByIDStrict(id)
	if err != nil {
		return ""
	}
	return configPath
}

// IsConfigEditable 判断配置是否可编辑
func IsConfigEditable(id string) bool {
	_, configPath, err := ProfilePathByIDStrict(id)
	if err != nil {
		return false
	}
	_, err = os.Stat(configPath)
	return err == nil
}

// GetEditableConfigs 获取所有可编辑的配置列表
func GetEditableConfigs() []ConfigTextResult {
	var results []ConfigTextResult

	// 主配置
	if IsConfigEditable("") {
		res, err := ReadConfigText("")
		if err == nil {
			results = append(results, res)
		}
	}

	// 订阅配置
	items := ListSubIndex()
	for _, item := range items {
		if IsConfigEditable(item.ID) {
			res, err := ReadConfigText(item.ID)
			if err == nil {
				res.Name = item.Name + ".yaml"
				results = append(results, res)
			}
		}
	}

	return results
}
