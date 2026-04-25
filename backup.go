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

	dataDir := utils.GetDataDir()
	// 指定需要备份的相对目标
	targets := []string{"settings", "subscriptions", "theme_setting.txt"}

	for _, target := range targets {
		fullPath := filepath.Join(dataDir, target)
		info, err := os.Stat(fullPath)
		if err != nil {
			continue // 如果目标不存在（如首次安装没有主题文件），则安全跳过
		}

		if info.IsDir() {
			// 递归压缩文件夹
			filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				relPath, _ := filepath.Rel(dataDir, path)
				w, err := zw.Create(filepath.ToSlash(relPath)) // 保证路径在 zip 内使用斜杠
				if err != nil {
					return err
				}
				content, _ := os.ReadFile(path)
				w.Write(content)
				return nil
			})
		} else {
			// 压缩单个文件
			w, err := zw.Create(filepath.ToSlash(target))
			if err != nil {
				return "", err
			}
			content, _ := os.ReadFile(fullPath)
			w.Write(content)
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
	var backupIndex []clash.SubIndexItem

	for _, f := range zr.File {
		// 1. 判断文件类型归属
		isSettingFile := strings.HasPrefix(f.Name, "settings/")
		isSubFile := strings.HasPrefix(f.Name, "subscriptions/")
		isThemeFile := f.Name == "theme_setting.txt"

		// 2. 按照还原模式过滤不需要的文件
		if mode == "subs" && !isSubFile && f.Name != "settings/user_index.json" {
			continue // 仅恢复订阅时，只放行订阅文件夹和索引
		}
		if mode == "settings" && !isSettingFile && !isThemeFile {
			continue // 仅恢复设置时，放行设置和主题
		}
		// 特殊阻截：如果是“仅恢复设置”，绝不能覆盖现有的订阅索引
		if mode == "settings" && f.Name == "settings/user_index.json" {
			continue
		}

		// 3. 拦截订阅索引文件，提取到内存进行合并运算，不直接物理覆盖
		if f.Name == "settings/user_index.json" && (mode == "all" || mode == "subs") {
			rc, err := f.Open()
			if err == nil {
				_ = json.NewDecoder(rc).Decode(&backupIndex)
				rc.Close()
			}
			continue 
		}

		// 4. 将被放行的文件物理解压覆盖到本地
		destPath := filepath.Join(dataDir, filepath.FromSlash(f.Name))
		os.MkdirAll(filepath.Dir(destPath), 0755)

		rc, err := f.Open()
		if err != nil {
			continue
		}
		
		dstFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err == nil {
			io.Copy(dstFile, rc)
			dstFile.Close()
		}
		rc.Close()
	}

	// 5. 核心逻辑：智能合并订阅索引
	if (mode == "all" || mode == "subs") && len(backupIndex) > 0 {
		clash.LoadIndex() // 重新加载当前磁盘的最新索引
		clash.IndexLock.Lock()

		// 记录当前已存在的订阅 ID，防止重复导入
		currentMap := make(map[string]bool)
		for _, item := range clash.SubIndex {
			currentMap[item.ID] = true
		}

		// 执行合并：仅追加本地不存在的订阅配置
		addedCount := 0
		for _, bItem := range backupIndex {
			if !currentMap[bItem.ID] {
				clash.SubIndex = append(clash.SubIndex, bItem)
				addedCount++
			}
		}
		clash.IndexLock.Unlock()

		if addedCount > 0 {
			clash.SaveIndex() // 有新增才写入磁盘
		}
	}

	// 6. 热重载系统状态
	a.initBehaviorCache() // 重新读取最新的 settings 进内存
	
	// 若内核正在运行，则重载配置让新设置生效
	active := a.getActiveConfig()
	if active != "" {
		clash.BuildRuntimeConfig(active, a.getActiveMode(), a.GetAppBehavior().LogLevel)
		if clash.IsRunning() {
			clash.ReloadConfig()
		}
	}
	a.SyncState() // 同步最新状态给 Vue 前端

	return "SUCCESS", nil
}
