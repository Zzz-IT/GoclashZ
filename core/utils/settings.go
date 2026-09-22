//go:build windows

package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var settingsMu sync.Mutex

// LoadSetting 泛型读取：优先 user -> 若未自定义则直接使用最新默认配置并覆盖更新 default
func LoadSetting[T any](fileName string, defaultData T) (*T, error) {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	dir := GetSettingsDir()
	userPath := filepath.Join(dir, "user_"+fileName+".json")
	defaultPath := filepath.Join(dir, "default_"+fileName+".json")

	result := defaultData

	// 1. 尝试读取用户的自定义修改
	if data, err := os.ReadFile(userPath); err == nil {
		if json.Unmarshal(data, &result) == nil {
			// 自动修补：将合并了默认值的完整结构重新写回硬盘，补齐缺失字段
			patchedBytes, _ := json.MarshalIndent(result, "", "  ")
			_ = WriteFileAtomic(userPath, patchedBytes, 0644)
			return &result, nil
		}
	}

	// 2. 若用户之前未手动修改过（即未生成 user_*.json），直接使用最新默认配置并覆盖写入 default 文件
	defaultBytes, _ := json.MarshalIndent(defaultData, "", "  ")
	_ = WriteFileAtomic(defaultPath, defaultBytes, 0644)

	return &defaultData, nil
}

// SaveSetting 保存设置，永远只写入 user 文件中
func SaveSetting[T any](fileName string, data *T) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	if data == nil {
		return fmt.Errorf("setting %q cannot be nil", fileName)
	}

	safeName, err := SanitizeFilename(fileName)
	if err != nil {
		return err
	}
	if safeName != fileName {
		return fmt.Errorf("非法设置文件名: %q", fileName)
	}

	userPath := filepath.Join(GetSettingsDir(), "user_"+safeName+".json")

	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return WriteFileAtomic(userPath, bytes, 0644)
}

// ResetSetting 恢复默认：直接删除 user 文件即可
func ResetSetting(fileName string) {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	os.Remove(filepath.Join(GetSettingsDir(), "user_"+fileName+".json"))
}
