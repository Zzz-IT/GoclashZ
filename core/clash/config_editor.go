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
	Content string `json:"content"`
	Path    string `json:"path"`
}

// ReadConfigText 读取配置文件文本内容
func ReadConfigText(id string) (ConfigTextResult, error) {
	safeID, err := utils.SanitizeFilename(id)
	if err != nil {
		return ConfigTextResult{}, err
	}
	if safeID != id {
		return ConfigTextResult{}, fmt.Errorf("非法配置 ID: %q", id)
	}

	var configPath string
	var name string

	if id == "" || id == "config.yaml" {
		configPath = GetConfigPath()
		name = "config.yaml"
	} else {
		configPath = filepath.Join(utils.GetSubscriptionsDir(), id+".yaml")
		name = id + ".yaml"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return ConfigTextResult{}, fmt.Errorf("读取配置文件失败: %w", err)
	}

	return ConfigTextResult{
		ID:      id,
		Name:    name,
		Content: string(data),
		Path:    configPath,
	}, nil
}

// SaveConfigText 保存配置文件文本内容
func SaveConfigText(id string, content string) error {
	safeID, err := utils.SanitizeFilename(id)
	if err != nil {
		return err
	}
	if safeID != id {
		return fmt.Errorf("非法配置 ID: %q", id)
	}

	var configPath string

	if id == "" || id == "config.yaml" {
		configPath = GetConfigPath()
	} else {
		configPath = filepath.Join(utils.GetSubscriptionsDir(), id+".yaml")
	}

	if err := utils.WriteFileAtomic(configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("保存配置文件失败: %w", err)
	}

	return nil
}

// ValidateConfigText 校验 YAML 语法
func ValidateConfigText(content string) error {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(content), &node); err != nil {
		return fmt.Errorf("YAML 语法错误: %w", err)
	}
	return nil
}

// GetConfigFilePath 获取配置文件路径
func GetConfigFilePath(id string) string {
	safeID, err := utils.SanitizeFilename(id)
	if err != nil || safeID != id {
		return ""
	}

	if id == "" || id == "config.yaml" {
		return GetConfigPath()
	}

	return filepath.Join(utils.GetSubscriptionsDir(), id+".yaml")
}

// IsConfigEditable 判断配置是否可编辑
func IsConfigEditable(id string) bool {
	if id == "" || id == "config.yaml" {
		path := GetConfigPath()
		_, err := os.Stat(path)
		return err == nil
	}

	safeID, err := utils.SanitizeFilename(id)
	if err != nil || safeID != id {
		return false
	}

	path := filepath.Join(utils.GetSubscriptionsDir(), safeID+".yaml")
	_, err = os.Stat(path)
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
