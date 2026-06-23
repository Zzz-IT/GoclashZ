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

// MigrateLegacyAppDataToInstallData is the main entry point to migrate data
// from the legacy AppData directory to the new stable installation DataDir.
func MigrateLegacyAppDataToInstallData() error {
	legacy := GetLegacyDataDir()
	target := GetDataDir()

	if legacy == "" || target == "" {
		return nil
	}

	if strings.EqualFold(filepath.Clean(legacy), filepath.Clean(target)) {
		return nil
	}

	// Only automatically migrate if it's an installed version (marker exists)
	appDir := GetAppDir()
	markerPath := filepath.Join(appDir, ".installed")
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		// Not an installed version, avoid migrating portable usages
		return nil
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
	if _, err := os.Stat(metaPath); err == nil {
		// Already migrated. Do not migrate again.
		// However, we should try to delete the legacy dir if it was not deleted successfully.
		tryDeleteLegacyDir(legacy, target)
		return nil
	}

	// Backup existing target data if any
	if hasMeaningfulData(target) {
		backupDir := filepath.Join(target, "_migration_backup", time.Now().Format("20060102_150405"))
		if err := copyDir(target, backupDir, nil); err != nil {
			return fmt.Errorf("备份目标数据目录失败: %w", err)
		}
	}

	// Copy data
	if err := copyDir(legacy, target, shouldSkipMigrationFile); err != nil {
		return fmt.Errorf("复制旧 AppData 数据失败: %w", err)
	}

	// Verify copied data
	if err := verifyCopied(legacy, target, shouldSkipMigrationFile); err != nil {
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
	candidates := []string{
		filepath.Join(dir, "Settings"),
		filepath.Join(dir, "Subscriptions"),
		filepath.Join(dir, "profiles"),
		filepath.Join(dir, "config.yaml"),
		filepath.Join(dir, "desired_state.json"),
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func acquireMigrationLock(dataDir string) (*os.File, error) {
	lockPath := filepath.Join(dataDir, ".migration.lock")
	return os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
}

func shouldSkipMigrationFile(path string) bool {
	lower := strings.ToLower(path)

	skipSuffixes := []string{
		".tmp", ".old", ".zip", ".db", ".metadb", ".meta.json",
	}
	for _, s := range skipSuffixes {
		if strings.HasSuffix(lower, s) {
			return true
		}
	}

	if strings.Contains(lower, string(filepath.Separator)+"logs"+string(filepath.Separator)) {
		return true
	}

	// Skip backup dirs
	if strings.Contains(lower, string(filepath.Separator)+"_migration_backup"+string(filepath.Separator)) {
		return true
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
	// If target exists and is a core component, we might skip based on timestamp.
	// But generally, for migration, we want to copy user configurations.
	if _, err := os.Stat(dst); err == nil {
		// Existing file. For settings/subscriptions, override.
		// For core/bin, could check modify time, but let's just override for simplicity,
		// or skip if we want newer. We override everything from AppData since it's the "truth".
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// Ensure parent dir exists
	_ = os.MkdirAll(filepath.Dir(dst), 0755)

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
// Normally the installer handles this, but if the user installed to Program Files and then runs as a normal user,
// they might not have permissions.
func RepairDataDirPermission() error {
	// Simple implementation: this requires admin. 
	// The frontend will invoke this via RequestAdmin / sys.RunElevatedWithArgsWait if not admin.
	// We can use icacls to grant Users modify permissions.
	appDir := GetAppDir()
	dataDir := filepath.Join(appDir, "data")
	
	_ = os.MkdirAll(dataDir, 0755)
	
	// Example of granting modify rights to BUILTIN\Users using icacls
	// Note: in a real implementation, you might want to use syscalls or proper sid handling.
	// For simplicity, granting 'Users' (or 'BUILTIN\Users') (M) rights.
	cmd := "icacls"
	args := []string{dataDir, "/grant", "Users:(OI)(CI)M", "/T", "/C", "/Q"}
	
	// Exec the command
	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("icacls failed: %v, output: %s", err, string(out))
	}
	
	return nil
}
