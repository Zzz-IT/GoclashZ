//go:build windows

package utils

import (
	jsonv2 "encoding/json/v2"

	"fmt"
	jsonutil "goclashz/core/jsonutil"
	"goclashz/core/logger"
	"os"
	"path/filepath"
	"sync"
	"time"
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
	if data, readErr := os.ReadFile(userPath); readErr == nil {
		if unmarshalErr := jsonv2.Unmarshal(data, &result); unmarshalErr == nil {
			// 自动修补：将合并了默认值的完整结构重新写回硬盘，补齐缺失字段
			patchedBytes, _ := jsonutil.MarshalIndent(result, "", "  ")
			_ = WriteFileAtomic(userPath, patchedBytes, 0644)
			return &result, nil
		} else {
			// F-A5-19：解析失败不再静默回落默认值——那样用户下一次保存会把原文件
			// 覆盖成「默认值 + 本次改动」，等于整份配置静默丢失，且事后无法复盘。
			// 这里把坏文件改名留档 + 落错误日志，再回退默认值。
			// 注意：*不*改变错误契约——LoadSetting 在起核路径上（GetTunConfig 等），
			// 返回 error 会让内核直接起不来，代价大于收益。
			backupPath := fmt.Sprintf("%s.corrupt-%s", userPath, time.Now().Format("20060102-150405"))
			renamed := backupPath
			if renameErr := os.Rename(userPath, backupPath); renameErr != nil {
				renamed = userPath
				logger.Warnf("配置文件 %s 解析失败且备份失败（下次保存将覆盖原文件）: %v", userPath, renameErr)
			}
			logger.Errorf("配置文件解析失败，已回退默认值；原文件备份为 %s；原因: %v", renamed, unmarshalErr)
		}
	}

	// 2. 若用户之前未手动修改过（即未生成 user_*.json），直接使用最新默认配置并覆盖写入 default 文件
	defaultBytes, _ := jsonutil.MarshalIndent(defaultData, "", "  ")
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

	bytes, err := jsonutil.MarshalIndent(data, "", "  ")
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
