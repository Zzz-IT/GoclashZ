//go:build windows

package appcore

import (
	"context"
	"fmt"

	"goclashz/core/backup"
	"goclashz/core/clash"
	"goclashz/core/sys"
	"goclashz/core/utils"
)

// ExportBackup 业务级导出：使用 staging 快照方式打包
func (c *Controller) ExportBackup(destPath string) error {
	return backup.Export(utils.GetDataDir(), destPath, c.version)
}

// RestoreBackup 事务化业务恢复编排：停核 -> 事务还原文件 -> 重载状态 -> 统一调和运行态
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

	// 2. 预处理：停止内核释放文件句柄，并临时关闭系统代理避免悬空断网
	if wasRunning || wantSysProxy {
		if wasRunning {
			c.stopCoreProcessLocked()
		}
		if wantSysProxy {
			_ = sys.DisableSystemProxy()
			c.mu.Lock()
			c.sysProxyActive = false
			c.mu.Unlock()
		}
		c.coreLifecycleMu.Unlock()
		c.SyncState()
	} else {
		c.coreLifecycleMu.Unlock()
	}

	// 3. 执行事务化底层还原
	if err := backup.RestoreTransactional(ctx, utils.GetDataDir(), selected, mode); err != nil {
		// 还原失败：如果原先在运行或需要代理，使用 Supervisor 统一调和恢复原状态
		if shouldRestart {
			_ = c.Supervisor.Reconcile(ctx, "restore-failed-rollback")
		}
		c.SyncState()
		return err
	}

	// 4. 后处理：状态重载
	// 显式重新加载订阅索引 (从磁盘到内存)
	_ = clash.LoadIndex()

	if err := clash.MigrateRuleStorageV2(); err != nil {
		c.SyncState()
		return fmt.Errorf("备份已恢复，但规则存储迁移失败: %w", err)
	}

	// 热重载应用行为配置
	if err := c.Behavior.Load(); err != nil {
		c.SyncState()
		return fmt.Errorf("配置文件还原成功但重载失败: %v", err)
	}

	if err := c.Desired.Load(); err != nil {
		c.SyncState()
		return fmt.Errorf("配置文件还原成功但意图重载失败: %v", err)
	}

	// 重新载入策略组节点记忆
	if c.Offline != nil {
		c.Offline.Load(c.Behavior.Get().ActiveConfig)
	}

	// 🛡️ 核心自愈：若当前活跃配置在还原后的订阅中不存在，平滑切换至首个可用配置或清空
	activeConfig := c.Behavior.Get().ActiveConfig
	if activeConfig != "" && activeConfig != "config.yaml" {
		if _, ok := clash.FindSubIndexByID(activeConfig); !ok {
			clash.IndexLock.RLock()
			items := clash.SubIndex
			clash.IndexLock.RUnlock()
			if len(items) > 0 {
				_ = c.Behavior.SetActiveConfig(items[0].ID)
				_ = c.Desired.Update(func(d *DesiredState) {
					d.ActiveConfig = items[0].ID
				})
			} else {
				_ = c.Behavior.SetActiveConfig("")
				_ = c.Desired.Update(func(d *DesiredState) {
					d.ActiveConfig = ""
					d.CoreRunning = false
				})
			}
		}
	}

	// 5. 恢复运行态：统一交由 Supervisor 调和器接管
	// 调和器会根据加载后的 DesiredState 自动判定是否需要内核、TUN 与系统代理，
	// 若不需要则自动 DisableAll，若需要则自动编译配置并拉起全部对应服务。
	if err := c.Supervisor.Reconcile(ctx, "restore"); err != nil {
		c.SyncState()
		return fmt.Errorf("还原成功但状态调和失败: %v", err)
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
