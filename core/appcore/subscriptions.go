//go:build windows

package appcore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"goclashz/core/clash"
	"goclashz/core/utils"
)

// subscriptionUpdateTimeout 是「单个订阅更新」这条**同步**调用链的总预算，
// 覆盖下载 + 必要时重载运行配置。
//
// UpdateSub / UpdateSingleSub 由 Wails 同步绑定直接 await，调用方给的 ctx 是
// 应用生命周期 ctx（无 deadline）。以前最坏会跟着下载机的 10 分钟 client timeout
// 一起把 UI 挂住（F-A5-08），这里补一个硬上限。
const subscriptionUpdateTimeout = 4 * time.Minute

// withSubscriptionTimeout 给同步订阅更新链路套上统一预算（ctx 为 nil 时兜底）。
func withSubscriptionTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, subscriptionUpdateTimeout)
}

func (c *Controller) UpdateSub(ctx context.Context, name, url string) error {
	ctx, cancel := withSubscriptionTimeout(ctx)
	defer cancel()

	ua := c.Behavior.Get().SubUA
	id, err := clash.DownloadSub(ctx, name, url, "", ua)
	if err != nil {
		return err
	}

	// 统一重启编排：运行中配置内容变化走 ControlCoordinator 的配置重载事务
	state := c.GetAppState()
	if state.ActiveConfig == id && state.IsRunning {
		return c.controls.ReloadConfig(ctx, "subscription-update")
	}
	return nil
}

func (c *Controller) UpdateSingleSub(ctx context.Context, id string) error {
	ctx, cancel := withSubscriptionTimeout(ctx)
	defer cancel()

	item, ok := clash.FindSubIndexByID(id)
	if !ok {
		return fmt.Errorf("subscription not found")
	}
	if item.URL == "" {
		return fmt.Errorf("subscription not found")
	}

	ua := c.Behavior.Get().SubUA
	_, err := clash.DownloadSub(ctx, item.Name, item.URL, id, ua)
	if err == nil {
		state := c.GetAppState()
		if state.ActiveConfig == id && state.IsRunning {
			return c.controls.ReloadConfig(ctx, "subscription-update")
		}
	}
	return err
}

func (c *Controller) UpdateAllSubsAsync(ctx context.Context) {
	c.Tasks.Run(ctx, "subs-update", true, func(ctx context.Context) error {
		items := clash.ListSubIndex()

		ua := c.Behavior.Get().SubUA
		needsRestart := false
		state := c.GetAppState()

		// 逐个收集失败原因：任何一个订阅失败都不能再让上层误报「全部成功」。
		// 之前这里的 err 只用来判断是否需要重载，最后一个都不返回，任务框架恒发
		// subs-update-success，用户会以为拿到了新节点。
		var failed []error
		for _, item := range items {
			if item.URL != "" && item.Type == "remote" {
				id, err := clash.DownloadSub(ctx, item.Name, item.URL, item.ID, ua)
				if err != nil {
					failed = append(failed, fmt.Errorf("%s: %w", item.Name, err))
					continue
				}
				if id == state.ActiveConfig {
					needsRestart = true
				}
			}
		}

		// 部分订阅更新成功时仍要重载运行配置，避免「成功的生效不了」。
		var reloadErr error
		if needsRestart && state.IsRunning {
			reloadErr = c.controls.ReloadConfig(ctx, "subscription-update")
		}

		if len(failed) == 0 {
			return reloadErr
		}

		joined := errors.Join(failed...)
		if reloadErr != nil {
			return errors.Join(fmt.Errorf("%d 个订阅更新失败: %w", len(failed), joined), reloadErr)
		}
		return fmt.Errorf("%d 个订阅更新失败: %w", len(failed), joined)
	})
}

// DeleteConfig 删除配置；删除当前激活配置前会先关系统代理 / TUN。
//
// 事务化：clash.DeleteConfig 只负责「物理文件 + 订阅索引」，之后的 Behavior /
// DesiredState 持久化失败会留下「配置已删、界面与下次启动仍指向它」的脏状态。
// 所以删除前先做快照（索引副本 + yaml 原始字节），任一步失败就把文件与索引一起
// 还原，并把每一步的错误都返回（errors.Join），不再用 «_ =» 吞掉。
func (c *Controller) DeleteConfig(ctx context.Context, id string) error {
	state := c.GetAppState()
	wasActive := state.ActiveConfig == id

	safeID, err := utils.SanitizeFilename(id)
	if err != nil {
		return err
	}
	yamlPath := filepath.Join(utils.GetSubscriptionsDir(), safeID+".yaml")

	// 快照：ListSubIndex 返回的是副本，yaml 读原始字节（不存在时记下原因）。
	indexSnapshot := clash.ListSubIndex()
	yamlSnapshot, yamlSnapshotErr := os.ReadFile(yamlPath)
	if yamlSnapshotErr != nil && !os.IsNotExist(yamlSnapshotErr) {
		return fmt.Errorf("读取配置文件失败: %w", yamlSnapshotErr)
	}

	// rollback 把「已发生的删除」还原回去，并如实返回回滚结果。
	rollback := func(cause error) error {
		var errs []error

		if yamlSnapshotErr == nil {
			if err := os.MkdirAll(filepath.Dir(yamlPath), 0755); err != nil {
				errs = append(errs, fmt.Errorf("重建配置目录失败: %w", err))
			} else if err := os.WriteFile(yamlPath, yamlSnapshot, 0644); err != nil {
				errs = append(errs, fmt.Errorf("还原配置文件失败: %w", err))
			}
		}
		if err := clash.ReplaceSubIndex(indexSnapshot); err != nil {
			errs = append(errs, fmt.Errorf("还原订阅索引失败: %w", err))
		}
		if wasActive {
			if err := c.Behavior.SetActiveConfig(id); err != nil {
				errs = append(errs, fmt.Errorf("还原「当前配置」失败: %w", err))
			}
			if err := c.Desired.Update(func(d *DesiredState) {
				d.ActiveConfig = id
				d.CoreRunning = false
			}); err != nil {
				errs = append(errs, fmt.Errorf("还原期望状态失败: %w", err))
			}
		}
		c.SyncState()

		if len(errs) > 0 {
			return fmt.Errorf("%w（回滚也失败，数据可能不一致: %v）", cause, errors.Join(errs...))
		}
		return fmt.Errorf("%w（已回滚本次删除）", cause)
	}

	if wasActive {
		// Deleting the active profile must leave no proxy endpoint pointing at a
		// stopped core. Use the same control commands as every other entry point.
		if err := c.ToggleSystemProxy(ctx, false); err != nil {
			return err
		}
		if err := c.ToggleTunMode(ctx, false); err != nil {
			return err
		}
	}

	if err := clash.DeleteConfig(id); err != nil {
		// 物理删除失败（文件仍被占用/只读）时索引尚未变更，直接返回；
		// 文件已消失说明失败发生在索引落盘阶段，必须把文件与内存索引一起还原。
		if _, statErr := os.Stat(yamlPath); os.IsNotExist(statErr) {
			return rollback(fmt.Errorf("删除配置文件失败: %w", err))
		}
		return err
	}

	if wasActive {
		if err := c.Behavior.SetActiveConfig(""); err != nil {
			return rollback(fmt.Errorf("配置已删除，但保存「当前配置」失败: %w", err))
		}
		if err := c.Desired.Update(func(d *DesiredState) {
			d.ActiveConfig = ""
			d.CoreRunning = false
			d.PortProxy = false
		}); err != nil {
			return rollback(fmt.Errorf("配置已删除，但保存期望状态失败: %w", err))
		}
		if state.IsRunning {
			c.StopCoreProcess()
		}
	}
	c.SyncState()
	return nil
}

func (c *Controller) SelectLocalConfig(ctx context.Context, id string) error {
	state := c.GetAppState()
	if state.ActiveConfig == id {
		return nil
	}

	previousConfig := state.ActiveConfig

	// 配置选择写入（Behavior + desired state）——事务中段执行
	apply := func() error {
		if err := c.Behavior.SetActiveConfig(id); err != nil {
			return err
		}
		return c.Desired.Update(func(d *DesiredState) {
			c.fillDesiredTarget(d)
		})
	}

	// 事务失败时的反向操作：恢复原配置选择
	rollback := func() error {
		if err := c.Behavior.SetActiveConfig(previousConfig); err != nil {
			return err
		}
		behavior := c.Behavior.Get()
		return c.Desired.Update(func(d *DesiredState) {
			d.ActiveConfig = previousConfig
			d.Mode = behavior.ActiveMode
		})
	}

	// 运行中：走 ControlCoordinator 的配置切换事务
	// （关系统代理 → 停 TUN → 停内核 → 换配置 → 起内核 → 等 API+代理组 → 恢复 TUN/系统代理）
	if state.IsRunning {
		return c.controls.SwitchConfig(ctx, ConfigTransitionOptions{
			Reason:   "config-switch",
			Apply:    apply,
			Rollback: rollback,
		})
	}

	// 未运行：只持久化选择，不启动内核（与既有行为一致）
	if err := apply(); err != nil {
		// Behavior (active config) 可能已持久化；desired 保存失败会让 UI 显示
		// 配置 B 而下次启动仍用配置 A，必须把错误表面化。
		c.SyncState()
		return fmt.Errorf("配置切换 desired state 保存失败: %w", err)
	}

	c.SyncState()
	return nil
}

func (c *Controller) RenameConfig(id, newName string) error {
	if err := clash.RenameConfig(id, newName); err != nil {
		return err
	}
	c.SyncState()
	return nil
}

func (c *Controller) DoLocalImport(srcPath, name string) (string, error) {
	return clash.ImportLocalConfig(srcPath, name)
}
