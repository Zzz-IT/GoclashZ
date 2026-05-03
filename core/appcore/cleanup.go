//go:build windows

package appcore

import (
	"goclashz/core/updater"
	"goclashz/core/utils"
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
		if updater.NormalizeVersion(string(cachedVer)) == updater.NormalizeVersion(currentAppVersion) {
			_ = os.Remove(updateExe)
			_ = os.Remove(updateVer)
		}
	}
}
