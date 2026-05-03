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
	clash.IndexLock.RLock()
	var url, name string
	for _, item := range clash.SubIndex {
		if item.ID == id {
			url = item.URL
			name = item.Name
			break
		}
	}
	clash.IndexLock.RUnlock()
	if url == "" {
		return fmt.Errorf("subscription not found")
	}

	ua := c.Behavior.Get().SubUA
	_, err := clash.DownloadSub(ctx, name, url, id, ua)
	if err == nil {
		state := c.GetAppState()
		if state.ActiveConfig == id && state.IsRunning {
			return c.RestartCoreWithReason(ctx, "subscription-update")
		}
	}
	return err
}

func (c *Controller) UpdateAllSubsAsync(ctx context.Context) {
	c.Tasks.Run(ctx, "sub-update-all", true, func(ctx context.Context) error {
		clash.IndexLock.RLock()
		items := make([]clash.SubIndexItem, len(clash.SubIndex))
		copy(items, clash.SubIndex)
		clash.IndexLock.RUnlock()

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

func (c *Controller) DeleteConfig(id string) error {
	if err := clash.DeleteConfig(id); err != nil {
		return err
	}
	state := c.GetAppState()
	if state.ActiveConfig == id {
		c.Behavior.SetActiveConfig("")
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
