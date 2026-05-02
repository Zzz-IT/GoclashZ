package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"goclashz/core/backup"
	"goclashz/core/clash"
	"goclashz/core/utils"
)

func (a *App) ExportBackup() (string, error) {
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

	if !strings.HasSuffix(strings.ToLower(savePath), ".gocz") {
		savePath += ".gocz"
	}

	err = backup.Export(utils.GetDataDir(), savePath)
	if err != nil {
		return "", err
	}
	return "SUCCESS", nil
}

func (a *App) SelectBackupFile() (string, error) {
	selected, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择要还原的备份文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "GoclashZ 备份文件 (*.gocz)", Pattern: "*.gocz"},
			{DisplayName: "Zip 压缩包 (*.zip)", Pattern: "*.zip"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("选择文件失败: %v", err)
	}
	return selected, nil
}

func (a *App) ExecuteRestore(selected string, mode string) (string, error) {
	if selected == "" {
		return "", fmt.Errorf("未选择有效的备份文件")
	}

	err := backup.Restore(utils.GetDataDir(), selected, mode)
	if err != nil {
		return "", err
	}

	// 🚀 核心修复：还原后显式重新加载订阅索引
	_ = clash.LoadIndex()

	// 热重载内存与系统状态
	a.core.Behavior.Load()
	a.core.RefreshAutoDelayTest() // 🚀 核心修复：还原后同步自动测速状态

	state := a.core.GetAppState()
	if state.ActiveConfig != "" {
		clash.BuildRuntimeConfig(state.ActiveConfig, state.Mode, a.core.Behavior.Get().LogLevel)
		if state.IsRunning {
			clash.ReloadConfig()
		}
	}
	a.SyncState()

	return "SUCCESS", nil
}
