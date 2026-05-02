package backup

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"goclashz/core/clash"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Export 打包数据到指定的目标路径
func Export(dataDir, destPath string) error {
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("创建备份文件失败: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	clash.IndexLock.RLock()
	defer clash.IndexLock.RUnlock()

	targets := []string{"settings", "subscriptions", "profiles", "theme_setting.txt"}
	for _, target := range targets {
		fullPath := filepath.Join(dataDir, target)
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}

		if info.IsDir() {
			if walkErr := filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				relPath, _ := filepath.Rel(dataDir, path)
				w, err := zw.Create(filepath.ToSlash(relPath))
				if err != nil {
					return err
				}
				content, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if _, err := w.Write(content); err != nil {
					return fmt.Errorf("写入压缩包失败: %v", err)
				}
				return nil
			}); walkErr != nil {
				return walkErr
			}
		} else {
			w, err := zw.Create(filepath.ToSlash(target))
			if err != nil {
				return err
			}
			content, err := os.ReadFile(fullPath)
			if err == nil {
				if _, err := w.Write(content); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Restore 从备份文件中恢复数据
func Restore(dataDir, archivePath, mode string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("解析备份文件失败: %v", err)
	}
	defer zr.Close()

	cleanDataDir := filepath.Clean(dataDir)
	var backupIndex []clash.SubIndexItem

	clash.IndexLock.Lock()
	defer clash.IndexLock.Unlock()

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		isSettingFile := strings.HasPrefix(f.Name, "settings/")
		isSubFile := strings.HasPrefix(f.Name, "subscriptions/")
		isProfileFile := strings.HasPrefix(f.Name, "profiles/")
		isThemeFile := f.Name == "theme_setting.txt"

		if mode == "subs" && !isSubFile && !isProfileFile {
			continue
		}
		if mode == "settings" && !isSettingFile && !isThemeFile {
			continue
		}
		if mode == "settings" && isProfileFile {
			continue
		}

		if f.Name == "profiles/index.json" && (mode == "all" || mode == "subs") {
			rc, err := f.Open()
			if err == nil {
				_ = json.NewDecoder(rc).Decode(&backupIndex)
				rc.Close()
			}
			continue
		}

		destPath := filepath.Join(dataDir, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(filepath.Clean(destPath), cleanDataDir+string(os.PathSeparator)) {
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		dstFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(dstFile, rc)
		dstFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}

	if (mode == "all" || mode == "subs") && len(backupIndex) > 0 {
		clash.LoadIndex()
		localIndexMap := make(map[string]int)
		for i, item := range clash.SubIndex {
			localIndexMap[item.ID] = i
		}
		changed := false
		for _, bItem := range backupIndex {
			if idx, exists := localIndexMap[bItem.ID]; exists {
				clash.SubIndex[idx] = bItem
				changed = true
			} else {
				clash.SubIndex = append(clash.SubIndex, bItem)
				changed = true
			}
		}
		if changed {
			clash.SaveIndex()
		}
	}

	return nil
}
