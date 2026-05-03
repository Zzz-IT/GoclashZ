//go:build windows

package appcore

import (
	"context"
	"fmt"

	"goclashz/core/backup"
	"goclashz/core/clash"
	"goclashz/core/utils"
)

// ExportBackup 业务级导出：使用 staging 快照方式打包
func (c *Controller) ExportBackup(destPath string) error {
	return backup.Export(utils.GetDataDir(), destPath, c.version)
}

// RestoreBackup 事务化业务恢复编排：停核 -> 事务还原文件 -> 重载状态 -> 重启或构建运行时配置
func (c *Controller) RestoreBackup(ctx context.Context, selected string, mode string) error {
	if selected == "" {
		return fmt.Errorf("未选择有效的备份文件")
	}

	// 1. 获取核心并发锁与运行状态锁
	c.componentUpdateMu.Lock()
	defer c.componentUpdateMu.Unlock()

	c.coreLifecycleMu.Lock()
	wasRunning := clash.IsRunning()

	c.mu.RLock()
	wantSysProxy := c.sysProxyActive
	wantTun := c.tunActive
	c.mu.RUnlock()

	// 决定是否需要在操作完成后恢复运行状态
	shouldRestart := wasRunning || wantSysProxy || wantTun

	// 2. 预处理：停止内核释放文件句柄
	if wasRunning {
		_ = c.stopCoreProcessLocked()
		c.coreLifecycleMu.Unlock() // 停止后释放生命周期锁，允许还原逻辑执行
		c.SyncState()
	} else {
		c.coreLifecycleMu.Unlock()
	}

	// 3. 执行事务化底层还原
	if err := backup.RestoreTransactional(ctx, utils.GetDataDir(), selected, mode); err != nil {
		// 还原失败：如果原先在运行，尝试恢复运行状态
		if shouldRestart {
			c.coreLifecycleMu.Lock()
			_ = c.ensureCoreRunningLocked(ctx)
			c.coreLifecycleMu.Unlock()
		}
		c.SyncState()
		return err
	}

	// 4. 后处理：状态重载
	// 显式重新加载订阅索引 (从磁盘到内存)
	_ = clash.LoadIndex()

	// 热重载应用行为配置
	if err := c.Behavior.Load(); err != nil {
		c.SyncState()
		return fmt.Errorf("配置文件还原成功但重载失败: %v", err)
	}

	// 5. 恢复运行态或重载配置
	if shouldRestart {
		c.coreLifecycleMu.Lock()
		err := c.ensureCoreRunningLocked(ctx)
		c.coreLifecycleMu.Unlock()

		if err != nil {
			c.SyncState()
			return fmt.Errorf("还原成功但启动内核失败: %v", err)
		}
	} else {
		// 未运行时，仅静默更新一次 runtime config 以确保下一次启动使用的是新配置
		state := c.GetAppState()
		if state.ActiveConfig != "" {
			_ = clash.BuildRuntimeConfig(
				state.ActiveConfig,
				state.Mode,
				c.Behavior.Get().LogLevel,
			)
		}
	}

	// 6. 刷新副作用与同步 UI
	c.RefreshAutoDelayTest(AutoDelayRefreshOptions{
		Immediate: false, // 恢复后通常会有较大的变动，不建议立即触发高频测速，由定时器接管
		Reason:    "restore",
	})
	c.RefreshAppAutoUpdate()
	c.SyncState()

	return nil
}
