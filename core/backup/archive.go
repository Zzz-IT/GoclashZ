//go:build windows

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
	"time"
	"context"
)

// Manifest 备份包元数据
type Manifest struct {
	App           string   `json:"app"`
	BackupVersion int      `json:"backupVersion"`
	AppVersion    string   `json:"appVersion"`
	CreatedAt     int64    `json:"createdAt"`
	Contains      []string `json:"contains"`
}

const CurrentBackupVersion = 2

// Export 打包数据到指定的目标路径，使用 staging 临时快照方式
func Export(dataDir, destPath string, appVersion string) error {
	// 1. 创建临时目录进行快照
	stagingDir, err := os.MkdirTemp(filepath.Dir(dataDir), ".goclashz-export-*")
	if err != nil {
		return fmt.Errorf("创建临时导出目录失败: %v", err)
	}
	defer os.RemoveAll(stagingDir)

	targets := []string{
		"Settings",
		"Subscriptions",
		"profiles",
		"config.yaml",
		"theme_setting.txt",
	}

	contains := []string{}

	// 2. 复制目标文件到 staging
	for _, target := range targets {
		src := filepath.Join(dataDir, target)
		dst := filepath.Join(stagingDir, target)

		info, err := os.Stat(src)
		if err != nil {
			continue
		}

		if target == "profiles" {
			// 🛡️ 针对索引文件夹，持有极短时间的读锁进行快照，防止 index.json 损坏
			clash.IndexLock.RLock()
			err = copyDir(src, dst)
			clash.IndexLock.RUnlock()
		} else if info.IsDir() {
			err = copyDir(src, dst)
		} else {
			err = copyFile(src, dst)
		}

		if err == nil {
			contains = append(contains, strings.ToLower(target))
		}
	}

	// 3. 生成 manifest.json
	manifest := Manifest{
		App:           "GoclashZ",
		BackupVersion: CurrentBackupVersion,
		AppVersion:    appVersion,
		CreatedAt:     time.Now().Unix(),
		Contains:      contains,
	}
	mBytes, _ := json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile(filepath.Join(stagingDir, "manifest.json"), mBytes, 0644)

	// 4. 执行压缩打包
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("创建备份文件失败: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	return filepath.Walk(stagingDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(stagingDir, path)
		w, err := zw.Create(filepath.ToSlash(relPath))
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = w.Write(content)
		return err
	})
}

// RestoreTransactional 事务化恢复流程
func RestoreTransactional(ctx context.Context, dataDir, archivePath, mode string) error {
	// 1. 模式校验
	if err := validateRestoreMode(mode); err != nil {
		return err
	}

	// 2. 创建工作空间 (staging 为待恢复数据，rollback 为旧数据备份)
	workDir, err := os.MkdirTemp(filepath.Dir(dataDir), ".goclashz-restore-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	stagingDir := filepath.Join(workDir, "staging")
	rollbackDir := filepath.Join(workDir, "rollback")
	os.MkdirAll(stagingDir, 0755)
	os.MkdirAll(rollbackDir, 0755)

	// 3. 解压并归一化到 staging (Zip Bomb 防护在内部执行)
	backupIndex, err := extractAndNormalizeToStaging(archivePath, stagingDir)
	if err != nil {
		return err
	}

	// 4. 校验 manifest (针对非 GoclashZ 备份进行拦截)
	if err := validateManifest(stagingDir); err != nil {
		return err
	}

	// 5. 构建恢复计划
	plan := buildRestorePlan(mode)

	// 6. 备份当前受影响的目标到 rollback 目录，用于失败回滚
	if err := backupCurrentTargets(dataDir, plan, rollbackDir); err != nil {
		return fmt.Errorf("备份当前数据失败，取消恢复: %v", err)
	}

	// 7. 执行原子替换逻辑
	if err := applyRestorePlan(dataDir, stagingDir, plan, mode, backupIndex); err != nil {
		// 8. 执行失败，尝试从 rollback 目录恢复
		_ = rollbackRestorePlan(dataDir, rollbackDir, plan)
		return fmt.Errorf("恢复执行失败，已尝试自动回滚: %v", err)
	}

	return nil
}

type RestorePlan struct {
	ReplaceDirs  []string
	ReplaceFiles []string
	MergeDirs    []string
}

func buildRestorePlan(mode string) *RestorePlan {
	plan := &RestorePlan{}
	switch mode {
	case "all":
		plan.ReplaceDirs = []string{"Settings", "Subscriptions", "profiles"}
		plan.ReplaceFiles = []string{"config.yaml", "theme_setting.txt"}
	case "settings":
		plan.ReplaceDirs = []string{"Settings"}
		plan.ReplaceFiles = []string{"theme_setting.txt"}
	case "subs":
		plan.ReplaceDirs = []string{"Subscriptions", "profiles"}
		plan.ReplaceFiles = []string{"config.yaml"}
	case "subs-merge":
		plan.MergeDirs = []string{"Subscriptions"}
	}
	return plan
}

func applyRestorePlan(dataDir, stagingDir string, plan *RestorePlan, mode string, backupIndex []clash.SubIndexItem) error {
	// A. 替换式目录：先删再考
	for _, dir := range plan.ReplaceDirs {
		src := filepath.Join(stagingDir, dir)
		dst := filepath.Join(dataDir, dir)
		if _, err := os.Stat(src); err == nil {
			_ = os.RemoveAll(dst)
			if err := copyDir(src, dst); err != nil {
				return err
			}
		}
	}

	// B. 替换式文件：直接覆盖
	for _, file := range plan.ReplaceFiles {
		src := filepath.Join(stagingDir, file)
		dst := filepath.Join(dataDir, file)
		if _, err := os.Stat(src); err == nil {
			if err := copyFile(src, dst); err != nil {
				return err
			}
		}
	}

	// C. 合并式目录：仅 Subscriptions 目录在 subs-merge 模式下执行增量复制
	if mode == "subs-merge" {
		src := filepath.Join(stagingDir, "Subscriptions")
		dst := filepath.Join(dataDir, "Subscriptions")
		if _, err := os.Stat(src); err == nil {
			if err := copyDir(src, dst); err != nil { 
				return err
			}
		}
	}

	// D. 索引状态恢复：单独处理内存索引，防止与磁盘状态不一致
	switch mode {
	case "all", "subs":
		// 替换语义：丢弃本地，全量使用备份
		clash.IndexLock.Lock()
		clash.SubIndex = backupIndex
		clash.IndexLock.Unlock()
		return clash.SaveIndex()
	case "subs-merge":
		// 合并语义：将备份索引项合并进本地
		return mergeBackupIndex(backupIndex)
	}

	return nil
}

func backupCurrentTargets(dataDir string, plan *RestorePlan, rollbackDir string) error {
	allTargets := append(plan.ReplaceDirs, plan.ReplaceFiles...)
	if len(plan.MergeDirs) > 0 {
		allTargets = append(allTargets, plan.MergeDirs...)
	}

	for _, target := range allTargets {
		src := filepath.Join(dataDir, target)
		dst := filepath.Join(rollbackDir, target)
		if _, err := os.Stat(src); err == nil {
			if info, _ := os.Stat(src); info.IsDir() {
				_ = copyDir(src, dst)
			} else {
				_ = copyFile(src, dst)
			}
		}
	}
	return nil
}

func rollbackRestorePlan(dataDir, rollbackDir string, plan *RestorePlan) error {
	allTargets := append([]string{}, plan.ReplaceDirs...)
	allTargets = append(allTargets, plan.ReplaceFiles...)
	allTargets = append(allTargets, plan.MergeDirs...)

	for _, target := range allTargets {
		src := filepath.Join(rollbackDir, target)
		dst := filepath.Join(dataDir, target)
		if _, err := os.Stat(src); err == nil {
			_ = os.RemoveAll(dst)
			info, _ := os.Stat(src)
			if info.IsDir() {
				_ = copyDir(src, dst)
			} else {
				_ = copyFile(src, dst)
			}
		}
	}
	return nil
}

func validateRestoreMode(mode string) error {
	switch mode {
	case "all", "settings", "subs", "subs-merge":
		return nil
	default:
		return fmt.Errorf("不支持的恢复模式: %s", mode)
	}
}

func validateManifest(stagingDir string) error {
	path := filepath.Join(stagingDir, "manifest.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // 允许旧版备份
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	if m.App != "" && m.App != "GoclashZ" {
		return fmt.Errorf("备份文件校验失败: 归属应用不匹配 (%s)", m.App)
	}
	return nil
}

func extractAndNormalizeToStaging(archivePath, stagingDir string) ([]clash.SubIndexItem, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	var backupIndex []clash.SubIndexItem

	const (
		maxRestoreFiles  = 1000
		maxRestoreTotal  = 300 * 1024 * 1024
		maxRestoreSingle = 50 * 1024 * 1024
	)

	if len(zr.File) > maxRestoreFiles {
		return nil, fmt.Errorf("备份包内文件过多")
	}

	var totalUncompressed uint64

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}

		destRel, _, ok := normalizeBackupEntry(f.Name)
		if !ok {
			continue
		}

		if f.UncompressedSize64 > maxRestoreSingle {
			return nil, fmt.Errorf("文件过大: %s", f.Name)
		}
		totalUncompressed += f.UncompressedSize64
		if totalUncompressed > maxRestoreTotal {
			return nil, fmt.Errorf("备份包总体积超出限制")
		}

		// 提前解析索引文件
		if destRel == "profiles/index.json" {
			rc, err := f.Open()
			if err == nil {
				_ = json.NewDecoder(rc).Decode(&backupIndex)
				rc.Close()
			}
		}

		destPath := filepath.Join(stagingDir, filepath.FromSlash(destRel))
		os.MkdirAll(filepath.Dir(destPath), 0755)

		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		dstFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return nil, err
		}
		_, err = io.Copy(dstFile, rc)
		dstFile.Close()
		rc.Close()
		if err != nil {
			return nil, err
		}
	}
	return backupIndex, nil
}

// --- Utils ---

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()
	_, err = io.Copy(destFile, sourceFile)
	return err
}

func copyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, info.Mode()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeBackupEntry(name string) (destRel string, kind string, ok bool) {
	n := filepath.ToSlash(filepath.Clean(name))
	lower := strings.ToLower(n)
	switch {
	case lower == "theme_setting.txt":
		return "theme_setting.txt", "theme", true
	case lower == "config.yaml":
		return "config.yaml", "config", true
	case lower == "manifest.json":
		return "manifest.json", "manifest", true
	case lower == "behavior.json":
		return filepath.ToSlash(filepath.Join("Settings", "user_behavior.json")), "settings", true
	case lower == "dns.json":
		return filepath.ToSlash(filepath.Join("Settings", "user_dns.json")), "settings", true
	case lower == "network.json":
		return filepath.ToSlash(filepath.Join("Settings", "user_network.json")), "settings", true
	case lower == "tun.json":
		return filepath.ToSlash(filepath.Join("Settings", "user_tun.json")), "settings", true
	case strings.HasPrefix(lower, "settings/"):
		parts := strings.Split(n, "/")
		rest := strings.Join(parts[1:], "/")
		switch strings.ToLower(rest) {
		case "behavior.json": rest = "user_behavior.json"
		case "dns.json": rest = "user_dns.json"
		case "network.json": rest = "user_network.json"
		case "tun.json": rest = "user_tun.json"
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

func mergeBackupIndex(backupIndex []clash.SubIndexItem) error {
	if len(backupIndex) == 0 {
		return nil
	}
	if err := clash.LoadIndex(); err != nil {
		return err
	}
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
	if changed {
		return clash.SaveIndex()
	}
	return nil
}
