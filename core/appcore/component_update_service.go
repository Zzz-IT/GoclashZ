//go:build windows

package appcore

import (
	"context"
	"fmt"
	"goclashz/core/clash"
)

type ComponentUpdateOptions struct {
	Name         string
	StopCore     bool
	RestartCore  bool
	Prepare      func(ctx context.Context) (map[string]string, error)
	Commit       func(ctx context.Context, prepared map[string]string) (map[string]string, error)
	AfterSuccess func(map[string]string)
}

// runComponentUpdateTransaction 统一处理运行时组件（内核、驱动）的更新事务
func (c *Controller) runComponentUpdateTransaction(
	ctx context.Context,
	taskName string,
	opt ComponentUpdateOptions,
) {
	c.Tasks.Run(ctx, taskName, true, func(ctx context.Context) error {
		// 1. 获取组件更新全局锁，避免多个组件同时更新
		c.componentUpdateMu.Lock()
		defer c.componentUpdateMu.Unlock()

		// 2. Prepare 阶段：内核仍然运行，允许使用本地代理下载大文件
		prepared, err := opt.Prepare(ctx)
		if err != nil {
			c.SyncState()
			return fmt.Errorf("%s准备失败: %w", opt.Name, err)
		}

		// 3. 获取内核生命周期锁，准备短暂停机替换文件
		c.coreLifecycleMu.Lock()

		wasRunning := clash.IsRunning()

		c.mu.RLock()
		wantSysProxy := c.sysProxyActive
		wantTun := c.tunActive
		c.mu.RUnlock()

		// 判定是否需要恢复运行。
		// 如果内核正在运行，或者逻辑上开启了代理/TUN，更新完成后应尝试恢复。
		shouldRestart := wasRunning || wantSysProxy || wantTun

		if opt.StopCore && wasRunning {
			_ = c.stopCoreProcessLocked()
			c.coreLifecycleMu.Unlock()

			// 停止后立刻同步状态，停掉 traffic/proxy monitor
			c.SyncState()
		} else {
			c.coreLifecycleMu.Unlock()
		}

		// 4. Commit 阶段：仅执行极快的文件替换（此时内核已停）
		result, err := opt.Commit(ctx, prepared)
		if err != nil {
			// 更新失败，尝试恢复原有的运行状态
			if shouldRestart {
				c.coreLifecycleMu.Lock()
				_ = c.ensureCoreRunningLocked(ctx)
				c.coreLifecycleMu.Unlock()
			}

			c.SyncState()
			return fmt.Errorf("%s失败: %w", opt.Name, err)
		}

		// 4. 更新成功后的回调
		if opt.AfterSuccess != nil {
			opt.AfterSuccess(result)
		}

		// 5. 恢复运行
		if opt.RestartCore && shouldRestart {
			c.coreLifecycleMu.Lock()
			err = c.ensureCoreRunningLocked(ctx)
			c.coreLifecycleMu.Unlock()

			if err != nil {
				c.SyncState()
				return fmt.Errorf("%s成功，但内核恢复启动失败: %w", opt.Name, err)
			}
		}

		c.SyncState()
		return nil
	})
}
