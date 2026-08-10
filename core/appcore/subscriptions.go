//go:build windows

package appcore

import (
	"context"
	"fmt"
	"goclashz/core/clash"
)

func (c *Controller) UpdateSub(ctx context.Context, name, url string) error {
	ua := c.Behavior.Get().SubUA
	id, err := clash.DownloadSub(ctx, name, url, "", ua)
	if err != nil {
		return err
	}

	// 统一重启编排
	state := c.GetAppState()
	if state.ActiveConfig == id && state.IsRunning {
		return c.RestartCoreWithReason(ctx, "subscription-update")
	}
	return nil
}

func (c *Controller) UpdateSingleSub(ctx context.Context, id string) error {
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
			return c.RestartCoreWithReason(ctx, "subscription-update")
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

		for _, item := range items {
			if item.URL != "" && item.Type == "remote" {
				id, err := clash.DownloadSub(ctx, item.Name, item.URL, item.ID, ua)
				if err == nil && id == state.ActiveConfig {
					needsRestart = true
				}
			}
		}

		if needsRestart && state.IsRunning {
			return c.RestartCoreWithReason(ctx, "subscription-update")
		}
		return nil
	})
}

func (c *Controller) DeleteConfig(ctx context.Context, id string) error {
	state := c.GetAppState()
	if state.ActiveConfig == id {
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
		return err
	}
	if state.ActiveConfig == id {
		c.Behavior.SetActiveConfig("")
		desired := c.Desired.Get()
		desired.ActiveConfig = ""
		desired.CoreRunning = false
		desired.PortProxy = false
		_ = c.Desired.SetAndSave(desired)
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

	if err := c.Behavior.SetActiveConfig(id); err != nil {
		return err
	}

	if err := c.Desired.Update(func(d *DesiredState) {
		c.fillDesiredTarget(d)
	}); err != nil {
		// Behavior (active config) is already persisted; a desired-state save
		// failure would leave the UI showing config B while the kernel restarts
		// with config A on the next run.
		c.SyncState()
		return fmt.Errorf("配置切换 desired state 保存失败: %w", err)
	}

	if state.IsRunning {
		return c.RestartCoreWithReason(ctx, "config-switch")
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
