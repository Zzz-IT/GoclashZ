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

	// 清理本程序自己产生的更新残留（F-A5-24）。
	//
	// 这里以前用 *.tmp / *.zip / *.meta.json 三个通配：corein 同时是内核、驱动、
	// geo 三方更新的工作目录，通配删除与下载产物共用同一命名空间——一旦出现并发
	// 下载或多开，就会把别人的断点/临时文件当成垃圾删掉。改成显式白名单。
	staleUpdateFiles := []string{
		"clash.update.zip", "clash.update.zip.tmp", "clash.update.zip.tmp.meta.json",
		"wintun.dll.zip", "wintun.dll.zip.tmp", "wintun.dll.zip.tmp.meta.json",
		"clash.exe.new", "wintun.dll.new",
	}
	for _, name := range staleUpdateFiles {
		_ = os.Remove(filepath.Join(binDir, name))
	}
	for _, db := range []string{"geoip.metadb", "geosite.dat", "country.mmdb", "asn.dat"} {
		for _, suffix := range []string{".tmp", ".tmp.meta.json"} {
			_ = os.Remove(filepath.Join(binDir, db+suffix))
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
