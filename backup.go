package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"goclashz/core/clash"
	"goclashz/core/utils"
)

// ExportBackup 导出备份：将 Settings, Subscriptions 文件夹及 theme_setting.txt 打包为 .gocz
func (a *App) ExportBackup() (string, error) {
	// 1. 弹出保存对话框
	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "选择备份保存位置",
		DefaultFilename: fmt.Sprintf("GoclashZ_Backup_%s.gocz", time.Now().Format("20060102")),
		Filters: []runtime.FileFilter{
			{DisplayName: "GoclashZ 备份文件 (*.gocz)", Pattern: "*.gocz"},
		},
	})
	if err != nil {
		return "", err
	}
	if savePath == "" {
		return "CANCELLED", nil
	}

	if !strings.HasSuffix(savePath, ".gocz") {
		savePath += ".gocz"
	}

	// 2. 创建压缩文件
	f, err := os.Create(savePath)
	if err != nil {
		return "", fmt.Errorf("创建备份文件失败: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	a.behaviorIOMu.Lock()
	defer a.behaviorIOMu.Unlock()

	// 🚀 修复：2. 跨包调用并锁定订阅索引文件的 IO，确保备份期间节点列表绝对静止！
	clash.IndexLock.RLock()
	defer clash.IndexLock.RUnlock()

	dataDir := utils.GetDataDir()
	// 🚀 核心修复：加入 profiles 目录，确保 index.json 能够被打包！
	targets := []string{"settings", "subscriptions", "profiles", "theme_setting.txt"}

	for _, target := range targets {
		fullPath := filepath.Join(dataDir, target)
		info, err := os.Stat(fullPath)
		if err != nil {
			continue // 如果目标不存在（如首次安装没有主题文件），则安全跳过
		}

		if info.IsDir() {
			// 递归压缩文件夹
			if walkErr := filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				relPath, _ := filepath.Rel(dataDir, path)
				w, err := zw.Create(filepath.ToSlash(relPath)) // 保证路径在 zip 内使用斜杠
				if err != nil {
					return err
				}
				content, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				// 🚀 核心修复：增加写入字节数与错误校验，杜绝静默损坏
				if _, err := w.Write(content); err != nil {
					return fmt.Errorf("写入压缩包失败: %v", err)
				}
				return nil
			}); walkErr != nil {
				return "", fmt.Errorf("备份打包 %s 目录时发生错误: %v", target, walkErr)
			}
		} else {
			// 压缩单个文件
			w, err := zw.Create(filepath.ToSlash(target))
			if err != nil {
				return "", err
			}
			content, err := os.ReadFile(fullPath)
			if err == nil {
				if _, err := w.Write(content); err != nil {
					return "", fmt.Errorf("写入文件 %s 失败: %v", target, err)
				}
			}
		}
	}

	return "SUCCESS", nil
}

// SelectBackupFile 供前端调用：仅弹出文件选择框并返回路径，不执行还原
func (a *App) SelectBackupFile() (string, error) {
	selected, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择要还原的备份文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "GoclashZ 备份文件 (*.gocz)", Pattern: "*.gocz"},
			{DisplayName: "Zip 压缩包 (*.zip)", Pattern: "*.zip"}, // 兼容用户手动改后缀的情况
		},
	})
	if err != nil {
		return "", fmt.Errorf("选择文件失败: %v", err)
	}
	return selected, nil
}

// ExecuteRestore 核心还原逻辑：支持按照 mode 过滤并合并订阅
// mode: "all" (全部), "subs" (仅订阅), "settings" (仅设置)
// ExecuteRestore 核心还原逻辑：支持按照 mode 过滤，强制以备份包状态为准进行覆盖/合并
func (a *App) ExecuteRestore(selected string, mode string) (string, error) {
	if selected == "" {
		return "", fmt.Errorf("未选择有效的备份文件")
	}

	zr, err := zip.OpenReader(selected)
	if err != nil {
		return "", fmt.Errorf("解析备份文件失败: %v", err)
	}
	defer zr.Close()

	dataDir := utils.GetDataDir()
	cleanDataDir := filepath.Clean(dataDir)
	var backupIndex []clash.SubIndexItem

	// 获取全局 IO LOCK，防止与后台自动保存冲突
	a.behaviorIOMu.Lock()
	defer a.behaviorIOMu.Unlock()

	const (
		maxRestoreFiles  = 1000
		maxRestoreTotal  = 300 * 1024 * 1024
		maxRestoreSingle = 50 * 1024 * 1024
	)

	if len(zr.File) > maxRestoreFiles {
		return "", fmt.Errorf("备份文件数量过多，拒绝恢复")
	}

	var totalUncompressed uint64

	for _, f := range zr.File {
		// 🚀 修复：遇到压缩包中的纯目录节点直接跳过，因为后续文件解压时会通过 MkdirAll 自动创建所需目录
		if f.FileInfo().IsDir() {
			continue
		}
		isSettingFile := strings.HasPrefix(f.Name, "settings/")
		isSubFile := strings.HasPrefix(f.Name, "subscriptions/")
		isProfileFile := strings.HasPrefix(f.Name, "profiles/")
		isThemeFile := f.Name == "theme_setting.txt"

		// 订阅模式：仅恢复 subscriptions 文件夹和 profiles 文件夹（含 index.json）
		if mode == "subs" && !isSubFile && !isProfileFile {
			continue
		}
		// 设置模式：仅恢复 settings 文件夹和主题文件
		if mode == "settings" && !isSettingFile && !isThemeFile {
			continue
		}
		// 设置模式下不处理索引合并（索引合并属于 subs 或 all 范畴）
		if mode == "settings" && isProfileFile {
			continue
		}

		if f.Name == "profiles/index.json" && (mode == "all" || mode == "subs") {
			rc, err := f.Open()
			if err == nil {
				if err := json.NewDecoder(rc).Decode(&backupIndex); err != nil {
					rc.Close()
					return "", fmt.Errorf("解析备份索引失败: %v", err)
				}
				rc.Close()
			} else {
				return "", fmt.Errorf("读取备份索引失败: %v", err)
			}
			continue
		}

		// Zip Bomb 防护
		if f.UncompressedSize64 > maxRestoreSingle {
			return "", fmt.Errorf("备份内单文件过大: %s", f.Name)
		}
		totalUncompressed += f.UncompressedSize64
		if totalUncompressed > maxRestoreTotal {
			return "", fmt.Errorf("备份总体积过大，拒绝恢复")
		}

		destPath := filepath.Join(dataDir, filepath.FromSlash(f.Name))
		
		// Zip Slip 防护
		if !strings.HasPrefix(filepath.Clean(destPath), cleanDataDir+string(os.PathSeparator)) {
			continue 
		}

		// 无条件覆盖本地，实现“备份是什么样就还原成什么样”
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return "", err
		}

		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		
		dstFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return "", err
		}

		if _, err := io.Copy(dstFile, rc); err != nil {
			dstFile.Close()
			rc.Close()
			return "", err
		}

		if err := dstFile.Close(); err != nil {
			rc.Close()
			return "", err
		}
		rc.Close()
	}

	// 核心逻辑：强力合并订阅索引
	if (mode == "all" || mode == "subs") && len(backupIndex) > 0 {
		clash.LoadIndex()
		clash.IndexLock.Lock()

		// 记录本地订阅的索引位置
		localIndexMap := make(map[string]int)
		for i, item := range clash.SubIndex {
			localIndexMap[item.ID] = i
		}

		changed := false
		for _, bItem := range backupIndex {
			if idx, exists := localIndexMap[bItem.ID]; exists {
				// 1. 本地已存在：强制将元数据覆写为备份里的状态
				clash.SubIndex[idx] = bItem
				changed = true
			} else {
				// 2. 本地不存在：追加备份里的新订阅
				clash.SubIndex = append(clash.SubIndex, bItem)
				changed = true
			}
		}
		clash.IndexLock.Unlock()

		if changed {
			clash.SaveIndex()
		}
	}

	// 热重载内存与系统状态
	a.initBehaviorCache()

	// 🚀 核心修复：重新读取并刷新主题内存缓存，避免 UI 状态滞后
	themeData, err := os.ReadFile(filepath.Join(dataDir, "theme_setting.txt"))
	if err == nil && len(themeData) > 0 {
		a.mu.Lock()
		a.themeCache = strings.TrimSpace(string(themeData))
		a.mu.Unlock()
	}

	active := a.getActiveConfig()
	if active != "" {
		clash.BuildRuntimeConfig(active, a.getActiveMode(), a.GetAppBehavior().LogLevel)
		if clash.IsRunning() {
			clash.ReloadConfig()
		}
	}
	a.SyncState()

	return "SUCCESS", nil
}
