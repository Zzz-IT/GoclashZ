//go:build windows

package appcore

import (
	"context"
	"fmt"
	"goclashz/core/clash"
)

// 跨进程错误码（前端按前缀分支，不再匹配中文文案）。
//
// 约定：错误信息以 "CODE: 中文说明: 原始错误" 的形式返回；前端只认前缀，
// 文案可以随便改而不影响判定。新增 code 时同步 frontend/src/utils/errors.ts 的 ErrorCode。
const (
	// ErrCodeConfigSavedReloadFailed 文件已落盘，但运行配置重载失败（用户不用重存）。
	ErrCodeConfigSavedReloadFailed = "CONFIG_SAVED_RELOAD_FAILED"
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
			// 运行中配置内容变化：走 ControlCoordinator 的配置重载事务
			// （stop → 载入新配置 → start → 等 API/代理组 → 恢复 TUN/系统代理）
			if err := c.controls.ReloadConfig(ctx, "config-save"); err != nil {
				// 前缀是稳定契约（前端只解析它）；后面的中文说明可以改。
				return fmt.Errorf("%s: 保存成功，但重载运行配置失败: %w", ErrCodeConfigSavedReloadFailed, err)
			}
		}

		c.events.Emit("config-changed", id)
		c.SyncState()
	}

	return nil
}
