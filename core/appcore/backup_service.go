package appcore

import (
	"context"
	"fmt"

	"goclashz/core/backup"
	"goclashz/core/clash"
	"goclashz/core/utils"
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
	c.RefreshAutoDelayTest()

	// 5. 根据恢复后的状态决定是否重启内核
	state := c.GetAppState()
	if state.ActiveConfig != "" {
		// 构建运行时配置并应用
		err := clash.BuildRuntimeConfig(
			state.ActiveConfig,
			state.Mode,
			c.Behavior.Get().LogLevel,
		)
		if err != nil {
			return err
		}

		if state.IsRunning {
			// 如果内核正在运行，执行热重载
			if err := clash.ReloadConfig(); err != nil {
				return err
			}
		}
	}

	// 6. 全局状态同步
	c.SyncState()
	return nil
}
