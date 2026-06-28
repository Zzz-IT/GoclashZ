//go:build windows

package appcore

import (
	"goclashz/core/logger"
	"goclashz/core/utils"
	"goclashz/core/version"
	"os"
	"path/filepath"
)

// CleanLegacyFiles 清理早期版本遗留的废弃配置文件及更新残留
func CleanLegacyFiles(currentAppVersion string) {
	binDir := utils.GetCoreBinDir()
	_ = os.Remove(filepath.Join(binDir, "active_config.txt"))
	_ = os.Remove(filepath.Join(binDir, "active_mode.txt"))

	// 启动时静默清理上次内核更新产生的 .old 垃圾文件
	_ = os.Remove(filepath.Join(binDir, "mihomo-windows-amd64.exe.old"))
	_ = os.Remove(filepath.Join(binDir, "clash.exe.old"))

	// 每次启动软件，清理上个版本可能残留的更新文件
	updateTmp := filepath.Join(utils.GetDataDir(), "GoclashZ_update.exe.tmp")
	updateExe := filepath.Join(utils.GetDataDir(), "GoclashZ_update.exe")
	updateVer := filepath.Join(utils.GetDataDir(), "GoclashZ_update.version")
	_ = os.Remove(updateTmp)

	// 如果本地存在的更新包版本已经等于当前运行的版本，则说明是旧包，清理掉
	if cachedVer, err := os.ReadFile(updateVer); err == nil {
		if version.NormalizeVersion(string(cachedVer)) == version.NormalizeVersion(currentAppVersion) {
			_ = os.Remove(updateExe)
			_ = os.Remove(updateVer)
		}
	}

	// 遍历并清理可能由于异常退出导致的 .tmp 和 .zip 残留
	if tmpFiles, err := filepath.Glob(filepath.Join(binDir, "*.tmp")); err == nil {
		for _, f := range tmpFiles {
			_ = os.Remove(f)
		}
	}
	if zipFiles, err := filepath.Glob(filepath.Join(binDir, "*.zip")); err == nil {
		for _, f := range zipFiles {
			_ = os.Remove(f)
		}
	}
	if metaFiles, err := filepath.Glob(filepath.Join(binDir, "*.meta.json")); err == nil {
		for _, f := range metaFiles {
			_ = os.Remove(f)
		}
	}
}

func moveOrCopyFile(oldPath, newPath string) error {
	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		return err
	}

	if err := os.Rename(oldPath, newPath); err == nil {
		return nil
	}

	data, err := os.ReadFile(oldPath)
	if err != nil {
		return err
	}

	if err := utils.WriteFileAtomic(newPath, data, 0644); err != nil {
		return err
	}

	return os.Remove(oldPath)
}

// MigrateLegacyRootSettings migrates legacy settings from root to Settings/
func MigrateLegacyRootSettings() {
	mappings := map[string]string{
		"behavior.json": "user_behavior.json",
		"dns.json":      "user_dns.json",
		"network.json":  "user_network.json",
		"tun.json":      "user_tun.json",
	}

	settingsDir := utils.GetSettingsDir()
	_ = os.MkdirAll(settingsDir, 0755)

	for oldName, newName := range mappings {
		oldPath := filepath.Join(utils.GetDataDir(), oldName)
		newPath := filepath.Join(settingsDir, newName)

		if _, err := os.Stat(oldPath); err != nil {
			continue
		}

		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			if err := moveOrCopyFile(oldPath, newPath); err != nil {
				logger.Warnf("迁移旧设置失败 %s -> %s: %v", oldPath, newPath, err)
				continue
			}
		} else {
			// 新文件已存在，以新文件为准，删除旧文件
			_ = os.Remove(oldPath)
		}
	}
}
