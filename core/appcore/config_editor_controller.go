//go:build windows

package appcore

import (
	"context"
	"fmt"
	"goclashz/core/clash"
)

// SaveConfigText 保存配置文件文本，并在必要时触发重载与事件
func (c *Controller) SaveConfigText(ctx context.Context, id string, content string) error {
	// 调用底层无副作用的纯保存方法（已改为 Strict 路径）
	if err := clash.SaveConfigText(id, content); err != nil {
		return err
	}

	state := c.GetAppState()

	c.events.Emit("config-file-saved", map[string]any{
		"id": id,
	})

	if state.ActiveConfig == id {
		c.events.Emit("delay-cache-clear", "config-save")

		if state.IsRunning || state.SystemProxy || state.Tun {
			if err := c.RestartCoreWithReason(ctx, "config-save"); err != nil {
				return fmt.Errorf("保存成功，但重载运行配置失败: %w", err)
			}
		}

		c.events.Emit("config-changed", id)
		c.SyncState()
	}

	return nil
}
