//go:build windows

package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

var settingsMu sync.Mutex

// LoadSetting 泛型读取：优先 user -> 其次 default -> 兜底生成 default
func LoadSetting[T any](fileName string, defaultData T) (*T, error) {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	dir := GetSettingsDir()
	userPath := filepath.Join(dir, "user_"+fileName+".json")
	defaultPath := filepath.Join(dir, "default_"+fileName+".json")

	var result T

	// 1. 尝试读取用户的自定义修改
	if data, err := os.ReadFile(userPath); err == nil {
		if json.Unmarshal(data, &result) == nil {
			return &result, nil
		}
	}

	// 2. 尝试读取默认配置
	if data, err := os.ReadFile(defaultPath); err == nil {
		if json.Unmarshal(data, &result) == nil {
			return &result, nil
		}
	}

	// 3. 都没找到，初始化默认配置文件 (只在第一次运行时触发)
	defaultBytes, _ := json.MarshalIndent(defaultData, "", "  ")
	os.WriteFile(defaultPath, defaultBytes, 0644)

	return &defaultData, nil
}

// SaveSetting 保存设置，永远只写入 user 文件中
func SaveSetting[T any](fileName string, data *T) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	userPath := filepath.Join(GetSettingsDir(), "user_"+fileName+".json")
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(userPath, bytes, 0644)
}

// ResetSetting 恢复默认：直接删除 user 文件即可
func ResetSetting(fileName string) {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	os.Remove(filepath.Join(GetSettingsDir(), "user_"+fileName+".json"))
}
