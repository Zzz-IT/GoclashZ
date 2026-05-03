//go:build windows

package appcore

import (
	"context"
	"fmt"

	"goclashz/core/backup"
	"goclashz/core/clash"
	"goclashz/core/utils"
	"os"
	"path/filepath"
)

// ExportBackup 业务级导出：调用底层归档逻辑
func (c *Controller) ExportBackup(destPath string) error {
	return backup.Export(utils.GetDataDir(), destPath)
}

// RestoreBackup 业务级恢复编排：文件恢复 + 内存状态重载 + 内核热重载
func (c *Controller) RestoreBackup(ctx context.Context, selected string, mode string) error {
	if selected == "" {
		return fmt.Errorf("未选择有效的备份文件")
	}

	// 1. 执行物理文件还原
	if err := backup.Restore(utils.GetDataDir(), selected, mode); err != nil {
		return err
	}

	// 2. 显式重新加载订阅索引 (从磁盘到内存)
	_ = clash.LoadIndex()

	// 3. 热重载应用行为配置
	if err := c.Behavior.Load(); err != nil {
		return err
	}

	// 4. 同步自动测速任务状态
	c.RefreshAutoDelayTest(AutoDelayRefreshOptions{
		Immediate: true,
		Reason:    "restore",
	})

	// 5. 根据恢复后的状态决定是否重启内核
	state := c.GetAppState()
	if state.ActiveConfig != "" {
		// 🛡️ 核心修复：检查配置是否存在，防止因配置缺失导致恢复失败
		configPath := clash.GetConfigPath() // 默认
		if state.ActiveConfig != "" && state.ActiveConfig != "config.yaml" {
			configPath = filepath.Join(utils.GetSubscriptionsDir(), state.ActiveConfig+".yaml")
		}

		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			c.SyncState()
			return nil
		}

		// 🚀 核心修复：如果内核正在运行，执行完整重启而不是 ReloadConfig
		// 因为备份可能改了 API 地址、DNS 或 TUN 等核心组件
		if state.IsRunning {
			return c.RestartCoreWithReason(ctx, "restore")
		}

		// 仅构建运行时配置
		return clash.BuildRuntimeConfig(
			state.ActiveConfig,
			state.Mode,
			c.Behavior.Get().LogLevel,
		)
	}

	// 6. 全局状态同步
	c.SyncState()
	return nil
}
