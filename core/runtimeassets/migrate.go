package runtimeassets

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"goclashz/core/utils"
)

// MigrateLegacyAssets 迁移旧版本的核心资产
func MigrateLegacyAssets() {
	targetDir := utils.GetCoreBinDir()
	_ = os.MkdirAll(targetDir, 0755)

	legacyDirs := []string{
		utils.GetLegacyDataCoreBinDir(), // 旧的 DataDir/core/bin (只读，仅用于迁移)
		utils.GetDataDir(),              // 旧的 DataDir 根目录
	}

	assets := []string{
		"clash.exe", "wintun.dll",
		"geoip.metadb", "geosite.dat", "country.mmdb", "asn.dat",
	}

	for _, name := range assets {
		target := filepath.Join(targetDir, name)

		targetExists := false
		if st, err := os.Stat(target); err == nil && !st.IsDir() {
			targetExists = true
		}

		for _, legacyDir := range legacyDirs {
			oldPath := filepath.Join(legacyDir, name)
			if _, err := os.Stat(oldPath); err != nil {
				continue
			}

			// 如果目标已经有同名文件，我们把老文件改名为 .old 稍后清理，或者直接清理
			if targetExists {
				_ = os.Rename(oldPath, oldPath+".old")
				continue
			}

			// 尝试重命名移动
			if err := os.Rename(oldPath, target); err == nil {
				targetExists = true
				break
			}

			// 失败则复制后删除
			if data, err := os.ReadFile(oldPath); err == nil {
				if err := os.WriteFile(target, data, 0755); err == nil {
					_ = os.Remove(oldPath)
					targetExists = true
					break
				}
			}
		}
	}
}

// CleanupLegacyAssets 安全清理残余的旧二进制目录及旧文件
func CleanupLegacyAssets() {
	appDir := utils.GetAppDir()
	dataDir := utils.GetDataDir()

	// 1. 判断是否是开发环境。如果在 appDir 下存在 core/appcore/controller.go 源码，绝对不能删除 core 文件夹！
	isDev := false
	if _, err := os.Stat(filepath.Join(appDir, "core", "appcore", "controller.go")); err == nil {
		isDev = true
	}

	legacyAssets := []string{
		"clash.exe", "wintun.dll",
		"geoip.metadb", "geosite.dat", "country.mmdb", "asn.dat",
		"clash.exe.old", "wintun.dll.old",
		"geoip.metadb.old", "geosite.dat.old", "country.mmdb.old", "asn.dat.old",
	}

	backupDir := filepath.Join(dataDir, "backups", "legacy-core", time.Now().Format("20060102150405"))

	// 辅助函数：安全删除或备份文件
	cleanOrBackupFile := func(path string, activePath string) {
		if _, err := os.Stat(path); err != nil {
			return
		}

		// 检查它是否和 activePath 的文件哈希相同。如果相同，可以直接删除。
		same := false
		h1, err1 := hashFile(path)
		h2, err2 := hashFile(activePath)
		if err1 == nil && err2 == nil && h1 == h2 {
			same = true
		}

		if same {
			if err := os.Remove(path); err == nil {
				log.Printf("[runtimeassets] 成功清除旧同名残留文件: %s", path)
				return
			}
		}

		// 如果不同或者是 .old 之外的有用二进制，备份到 backups 目录
		_ = os.MkdirAll(backupDir, 0755)
		backupPath := filepath.Join(backupDir, filepath.Base(path))
		if err := os.Rename(path, backupPath); err == nil {
			log.Printf("[runtimeassets] 备份并归档冲突旧资产: %s -> %s", path, backupPath)
		} else {
			_ = os.Remove(path) // 退而求其次，直接删掉
		}
	}

	// 2. 清理旧版 AppDir 中的 core/bin
	appCoreBin := filepath.Join(appDir, "core", "bin")
	for _, name := range legacyAssets {
		cleanOrBackupFile(filepath.Join(appCoreBin, name), filepath.Join(utils.GetCoreBinDir(), name))
	}
	_ = os.Remove(appCoreBin) // 尝试删除空目录

	// 3. 清理旧版 AppDir 中的 core 目录 (仅在非开发环境下！)
	if !isDev {
		appCore := filepath.Join(appDir, "core")
		_ = os.Remove(appCore) // 只有在它变空时才会成功删除，安全防呆
	}

	// 4. 清理旧版 DataDir 根目录下的残留文件 (旧架构会把文件直接下发到 DataDir)
	for _, name := range legacyAssets {
		cleanOrBackupFile(filepath.Join(dataDir, name), filepath.Join(utils.GetCoreBinDir(), name))
	}

	// 5. 如果是标准模式，且根目录下有非激活的 {app}\data 文件夹残留，也进行清理
	if !strings.EqualFold(dataDir, filepath.Join(appDir, "data")) {
		legacyDataCoreBin := filepath.Join(appDir, "data", "core", "bin")
		for _, name := range legacyAssets {
			cleanOrBackupFile(filepath.Join(legacyDataCoreBin, name), filepath.Join(utils.GetCoreBinDir(), name))
		}
		_ = os.Remove(legacyDataCoreBin)
		_ = os.Remove(filepath.Join(appDir, "data", "core"))
		_ = os.Remove(filepath.Join(appDir, "data")) // 尝试删除多余空目录
	}
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
