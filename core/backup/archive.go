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

	// 🚀 核心修复：使用标准目录名并包含 config.yaml
	targets := []string{
		"Settings",
		"Subscriptions",
		"profiles",
		"config.yaml",
		"theme_setting.txt",
	}

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

	// Zip Bomb 防护常量
	const (
		maxRestoreFiles  = 1000
		maxRestoreTotal  = 300 * 1024 * 1024
		maxRestoreSingle = 50 * 1024 * 1024
	)

	if len(zr.File) > maxRestoreFiles {
		return fmt.Errorf("备份文件数量过多，拒绝恢复")
	}

	var totalUncompressed uint64

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}

		// 1. 路径规范化与大小写/旧版本兼容
		destRel, kind, ok := normalizeBackupEntry(f.Name)
		if !ok {
			continue
		}

		// 2. 根据模式过滤
		switch mode {
		case "settings":
			if kind != "settings" && kind != "theme" {
				continue
			}
		case "subs":
			if kind != "subs" && kind != "profiles" && kind != "config" {
				continue
			}
		}

		// 3. 特殊处理索引文件
		if destRel == "profiles/index.json" && (mode == "all" || mode == "subs") {
			rc, err := f.Open()
			if err == nil {
				_ = json.NewDecoder(rc).Decode(&backupIndex)
				rc.Close()
			}
			continue
		}

		// 4. Zip Bomb 防护
		if f.UncompressedSize64 > maxRestoreSingle {
			return fmt.Errorf("备份内单文件过大: %s", f.Name)
		}
		totalUncompressed += f.UncompressedSize64
		if totalUncompressed > maxRestoreTotal {
			return fmt.Errorf("备份总体积过大，拒绝恢复")
		}

		// 5. 目标路径安全校验
		destPath := filepath.Join(dataDir, filepath.FromSlash(destRel))
		if !strings.HasPrefix(filepath.Clean(destPath), cleanDataDir+string(os.PathSeparator)) {
			continue
		}

		// 6. 执行解压覆盖
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

	// 7. 合并订阅索引 (单独处理锁，防止死锁)
	return mergeBackupIndex(backupIndex)
}

// normalizeBackupEntry 处理备份路径的规范化、大小写兼容以及旧版本映射
func normalizeBackupEntry(name string) (destRel string, kind string, ok bool) {
	n := filepath.ToSlash(filepath.Clean(name))
	lower := strings.ToLower(n)

	switch {
	case lower == "theme_setting.txt":
		return "theme_setting.txt", "theme", true

	case lower == "config.yaml":
		return "config.yaml", "config", true

	// 旧版本根目录 behavior/dns/network/tun 映射
	case lower == "behavior.json":
		return filepath.ToSlash(filepath.Join("Settings", "user_behavior.json")), "settings", true
	case lower == "dns.json":
		return filepath.ToSlash(filepath.Join("Settings", "user_dns.json")), "settings", true
	case lower == "network.json":
		return filepath.ToSlash(filepath.Join("Settings", "user_network.json")), "settings", true
	case lower == "tun.json":
		return filepath.ToSlash(filepath.Join("Settings", "user_tun.json")), "settings", true

	// 文件夹前缀匹配 (处理大小写不一致问题)
	case strings.HasPrefix(lower, "settings/"):
		parts := strings.Split(n, "/")
		rest := strings.Join(parts[1:], "/")
		// 映射旧文件名到新文件名 (如果需要)
		switch strings.ToLower(rest) {
		case "behavior.json":
			rest = "user_behavior.json"
		case "dns.json":
			rest = "user_dns.json"
		case "network.json":
			rest = "user_network.json"
		case "tun.json":
			rest = "user_tun.json"
		}
		return filepath.ToSlash(filepath.Join("Settings", rest)), "settings", true

	case strings.HasPrefix(lower, "subscriptions/"):
		parts := strings.Split(n, "/")
		rest := strings.Join(parts[1:], "/")
		return filepath.ToSlash(filepath.Join("Subscriptions", rest)), "subs", true

	case strings.HasPrefix(lower, "profiles/"):
		parts := strings.Split(n, "/")
		rest := strings.Join(parts[1:], "/")
		return filepath.ToSlash(filepath.Join("profiles", rest)), "profiles", true
	}

	return "", "", false
}

// mergeBackupIndex 合并订阅索引，采用细粒度锁防止死锁
func mergeBackupIndex(backupIndex []clash.SubIndexItem) error {
	if len(backupIndex) == 0 {
		return nil
	}

	// 1. 先加载当前磁盘索引
	if err := clash.LoadIndex(); err != nil {
		return err
	}

	// 2. 短时间持有锁进行内存合并
	clash.IndexLock.Lock()
	localIndexMap := make(map[string]int)
	for i, item := range clash.SubIndex {
		localIndexMap[item.ID] = i
	}

	changed := false
	for _, bItem := range backupIndex {
		if idx, exists := localIndexMap[bItem.ID]; exists {
			clash.SubIndex[idx] = bItem
		} else {
			clash.SubIndex = append(clash.SubIndex, bItem)
		}
		changed = true
	}
	clash.IndexLock.Unlock()

	// 3. 释放锁后再执行持久化
	if changed {
		return clash.SaveIndex()
	}
	return nil
}
