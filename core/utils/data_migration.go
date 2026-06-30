//go:build windows

package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"goclashz/core/logger"
)

type MigrationMeta struct {
	Version       int    `json:"version"`
	From          string `json:"from"`
	To            string `json:"to"`
	MigratedAt    int64  `json:"migratedAt"`
	SourceDeleted bool   `json:"sourceDeleted"`
	DeleteError   string `json:"deleteError,omitempty"`
}

type MigrationErrorMeta struct {
	From        string `json:"from"`
	To          string `json:"to"`
	At          int64  `json:"at"`
	Error       string `json:"error"`
	Recoverable bool   `json:"recoverable"`
}

func MigrateLegacyAppDataToInstallData() error {
	return migrateLegacyAppDataToInstallData(false)
}

func ForceMigrateLegacyAppDataToInstallData() error {
	return migrateLegacyAppDataToInstallData(true)
}

func migrateLegacyAppDataToInstallData(force bool) error {
	legacy := GetLegacyDataDir()
	target := GetDataDir()

	if legacy == "" || target == "" {
		return nil
	}

	if strings.EqualFold(filepath.Clean(legacy), filepath.Clean(target)) {
		return nil
	}

	if !force {
		// Only automatically migrate if it's an installed version (marker exists)
		appDir := GetAppDir()
		markerPath := filepath.Join(appDir, ".installed")
		if _, err := os.Stat(markerPath); os.IsNotExist(err) {
			// Not an installed version, avoid migrating portable usages
			return nil
		}
	}

	if !hasMeaningfulData(legacy) {
		return nil
	}

	if err := os.MkdirAll(target, 0755); err != nil {
		return fmt.Errorf("创建目标数据目录失败: %w", err)
	}

	lock, err := acquireMigrationLock(target)
	if err != nil {
		return fmt.Errorf("数据迁移已在进行中或锁文件被占用: %w", err)
	}
	defer func() {
		lock.Close()
		_ = os.Remove(filepath.Join(target, ".migration.lock"))
	}()

	// Check if already migrated
	metaPath := filepath.Join(target, ".migration.json")
	if _, err := os.Stat(metaPath); err == nil && !force {
		// Already migrated. Do not migrate again unless forced.
		tryDeleteLegacyDir(legacy, target)
		return nil
	}

	// Backup existing target data if any
	if hasMeaningfulData(target) {
		backupDir := migrationBackupDir(target)
		if err := copyDir(target, backupDir, shouldSkipMigrationFile); err != nil {
			writeMigrationError(target, legacy, fmt.Sprintf("备份目标数据目录失败: %v", err))
			return fmt.Errorf("备份目标数据目录失败: %w", err)
		}
	}

	// Copy data
	if err := copyDir(legacy, target, shouldSkipMigrationFile); err != nil {
		writeMigrationError(target, legacy, fmt.Sprintf("复制旧 AppData 数据失败: %v", err))
		return fmt.Errorf("复制旧 AppData 数据失败: %w", err)
	}

	// Verify copied data
	if err := verifyCopied(legacy, target, shouldSkipMigrationFile); err != nil {
		writeMigrationError(target, legacy, fmt.Sprintf("迁移校验失败: %v", err))
		return fmt.Errorf("迁移校验失败: %w", err)
	}

	// Write migration meta
	meta := MigrationMeta{
		Version:       1,
		From:          legacy,
		To:            target,
		MigratedAt:    time.Now().Unix(),
		SourceDeleted: false,
	}

	if err := saveMigrationMeta(target, meta); err != nil {
		return err
	}

	// Clean any previous error since we succeeded
	_ = os.Remove(filepath.Join(target, ".migration_error.json"))

	// Attempt to delete source
	if err := os.RemoveAll(legacy); err != nil {
		renamed := legacy + ".migrated_delete_me"
		_ = os.Rename(legacy, renamed)

		meta.SourceDeleted = false
		meta.DeleteError = err.Error()
		_ = saveMigrationMeta(target, meta)
		logger.Warnf("旧 AppData 目录删除失败，请手动删除: %s, err=%v", legacy, err)
		return nil
	}

	meta.SourceDeleted = true
	return saveMigrationMeta(target, meta)
}

func migrationBackupDir(target string) string {
	base := filepath.Join(filepath.Dir(target), "GoclashZ_data_migration_backup")
	return filepath.Join(base, time.Now().Format("20060102_150405"))
}

func writeMigrationError(target, from, errMsg string) {
	errFile := filepath.Join(target, ".migration_error.json")
	meta := MigrationErrorMeta{
		From:        from,
		To:          target,
		At:          time.Now().Unix(),
		Error:       errMsg,
		Recoverable: true,
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err == nil {
		_ = os.WriteFile(errFile, data, 0644)
	}
}

func tryDeleteLegacyDir(legacy, target string) {
	metaPath := filepath.Join(target, ".migration.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return
	}
	var meta MigrationMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return
	}
	if !meta.SourceDeleted {
		if err := os.RemoveAll(legacy); err == nil {
			meta.SourceDeleted = true
			meta.DeleteError = ""
			_ = saveMigrationMeta(target, meta)
		} else {
			renamed := legacy + ".migrated_delete_me"
			_ = os.Rename(legacy, renamed)
		}
	}
}

func hasMeaningfulData(dir string) bool {
	if dir == "" {
		return false
	}

	meaningfulFiles := []string{
		filepath.Join(dir, "config.yaml"),
		filepath.Join(dir, "desired_state.json"),
		filepath.Join(dir, "theme_setting.txt"),
		filepath.Join(dir, ".migration.json"),
	}

	for _, p := range meaningfulFiles {
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Size() > 0 {
			return true
		}
	}

	meaningfulDirs := []string{
		filepath.Join(dir, "Settings"),
		filepath.Join(dir, "Subscriptions"),
		filepath.Join(dir, "profiles"),
	}

	for _, p := range meaningfulDirs {
		if hasAnyMeaningfulFile(p) {
			return true
		}
	}

	return false
}

func hasAnyMeaningfulFile(dir string) bool {
	found := false

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}

		if info.IsDir() {
			base := strings.ToLower(info.Name())
			if base == "_migration_backup" || base == "logs" {
				return filepath.SkipDir
			}
			return nil
		}

		if info.Size() <= 0 {
			return nil
		}

		if shouldSkipMigrationFile(path) {
			return nil
		}

		found = true
		return filepath.SkipDir
	})

	return found
}

func acquireMigrationLock(dataDir string) (*os.File, error) {
	lockPath := filepath.Join(dataDir, ".migration.lock")

	if st, err := os.Stat(lockPath); err == nil {
		if time.Since(st.ModTime()) > 10*time.Minute {
			_ = os.Remove(lockPath)
		}
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}

	_, _ = f.WriteString(fmt.Sprintf("%d\n", os.Getpid()))
	return f, nil
}

func shouldSkipMigrationFile(path string) bool {
	clean := strings.ToLower(filepath.Clean(path))
	sep := string(filepath.Separator)

	skipFragments := []string{
		sep + "logs" + sep,
		sep + "_migration_backup" + sep,
		sep + "core" + sep + "bin" + sep,
	}

	for _, frag := range skipFragments {
		if strings.Contains(clean, frag) {
			return true
		}
	}

	skipSuffixes := []string{
		".tmp",
		".old",
		".zip",
		".db",
		".metadb",
		".meta.json",
		".lock",
	}

	for _, s := range skipSuffixes {
		if strings.HasSuffix(clean, s) {
			return true
		}
	}

	return false
}

func copyDir(src, dst string, skipFunc func(string) bool) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if skipFunc != nil && skipFunc(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return copyFile(path, dstPath, info)
	})
}

func copyFile(src, dst string, info os.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return nil
}

func verifyCopied(src, dst string, skipFunc func(string) bool) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if skipFunc != nil && skipFunc(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, relPath)

		dstInfo, err := os.Stat(dstPath)
		if err != nil {
			return fmt.Errorf("缺失文件: %s", relPath)
		}

		// Optionally check size, but size should be fine.
		if info.Size() != dstInfo.Size() {
			return fmt.Errorf("文件大小不匹配: %s", relPath)
		}

		return nil
	})
}

func saveMigrationMeta(target string, meta MigrationMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(target, ".migration.json"), data, 0644)
}

// RepairDataDirPermission is a utility function to be called with admin rights to repair directory ACLs.
func RepairDataDirPermission() error {
	appDir := GetAppDir()
	dataDir := filepath.Join(appDir, "data")

	_ = os.MkdirAll(dataDir, 0755)

	cmd := "icacls"
	args := []string{dataDir, "/grant", "*S-1-5-32-545:(OI)(CI)M", "/T", "/C", "/Q"}

	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("icacls failed: %v, output: %s", err, string(out))
	}

	return nil
}

// RepairCoreBinPermission is a utility function to be called with admin rights to repair core directory ACLs.
func RepairCoreBinPermission() error {
	coreDir := filepath.Join(GetAppDir(), "core")

	_ = os.MkdirAll(coreDir, 0755)

	cmd := "icacls"
	args := []string{coreDir, "/grant", "*S-1-5-32-545:(OI)(CI)RX", "/T", "/C", "/Q"}

	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("icacls (core RX) failed: %v, output: %s", err, string(out))
	}

	return nil
}
