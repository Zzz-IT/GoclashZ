package appcore

import (
	"context"
	"fmt"
	"goclashz/core/clash"
	"goclashz/core/downloader"
	"goclashz/core/utils"
	"path/filepath"
)

func (c *Controller) UpdateCoreComponentAsync(ctx context.Context) {
	c.Tasks.Run(ctx, "core-update", true, func(ctx context.Context) error {
		state := c.GetAppState()
		isActive := state.SystemProxy || state.Tun

		if isActive {
			c.StopCoreProcess()
		}

		_, err := clash.UpdateCore(ctx)
		if err != nil {
			return err
		}

		if isActive {
			return c.RestartCore(ctx)
		}
		return nil
	})
}

func (c *Controller) UpdateGeoDatabaseAsync(ctx context.Context, key string) {
	c.Tasks.Run(ctx, "geo-update-"+key, true, func(ctx context.Context) error {
		behavior := c.Behavior.Get()
		url := ""
		switch key {
		case "geoip":
			url = behavior.GeoIpLink
		case "geosite":
			url = behavior.GeoSiteLink
		case "mmdb":
			url = behavior.MmdbLink
		case "asn":
			url = behavior.AsnLink
		}
		if url == "" {
			return fmt.Errorf("no URL configured for %s", key)
		}
		return clash.UpdateGeoDB(ctx, key, url)
	})
}

func (c *Controller) UpdateAllGeoDatabasesAsync(ctx context.Context) {
	c.UpdateGeoDatabaseAsync(ctx, "geoip")
	c.UpdateGeoDatabaseAsync(ctx, "geosite")
	c.UpdateGeoDatabaseAsync(ctx, "mmdb")
	c.UpdateGeoDatabaseAsync(ctx, "asn")
}

func (c *Controller) CheckAndDownloadAppUpdateAsync(ctx context.Context, currentVersion string) {
	// 手动检查时，如果已经是最新版，也要提示用户
	c.checkAndDownloadAppUpdate(ctx, currentVersion, true)
}

func (c *Controller) AutoCheckAndDownloadAppUpdateAsync(ctx context.Context, currentVersion string) {
	// 自动检查时，如果已经是最新版，则保持静默
	c.checkAndDownloadAppUpdate(ctx, currentVersion, false)
}

func (c *Controller) checkAndDownloadAppUpdate(ctx context.Context, currentVersion string, notifyLatest bool) {
	c.Tasks.Run(ctx, "app-update", false, func(ctx context.Context) error {
		info, err := downloader.CheckAppUpdate(ctx, currentVersion)
		if err != nil {
			c.events.Emit("app-update-error", "检查更新失败: "+err.Error())
			return nil // 不再返回 err 避免 Tasks 框架二次抛错
		}

		if info == nil || !info.HasUpdate {
			if notifyLatest {
				c.events.Emit("app-update-none", map[string]any{
					"message": "当前已经是最新版本",
				})
			}
			return nil
		}

		// 🚀 核心改进：先确认是否有可下载资产，再决定弹窗内容
		if info.DownloadURL == "" {
			c.events.Emit("app-update-error", fmt.Sprintf(
				"发现新版本 %s，但 Release 中没有匹配的 Windows .exe 资产。\n当前资产列表: %v",
				info.Version,
				info.Assets,
			))
			return nil
		}

		// 1. 记录更新状态
		c.SetUpdateStatus(true, info.Version)

		// 2. 弹出发现新版本卡片，告知后台正在下载
		c.events.Emit("app-update-available", map[string]any{
			"version":      info.Version,
			"releaseNotes": info.Body,
			"releaseUrl":   info.ReleaseURL,
			"downloadUrl":  info.DownloadURL,
		})

		// 3. 开始后台静默下载
		c.events.Emit("app-update-start", map[string]any{
			"version": info.Version,
		})

		destDir := filepath.Join(utils.GetDataDir(), "updates")
		path, err := downloader.DownloadAppUpdate(ctx, info, destDir)
		if err != nil {
			c.events.Emit("app-update-error", "下载更新失败: "+err.Error())
			return nil
		}

		// 4. 下载成功，更新本地记录
		c.SetDownloadedAppUpdate(path, info.Version)
		
		c.events.Emit("app-update-downloaded", map[string]any{
			"version": info.Version,
			"path":    path,
		})

		return nil
	})
}

func (c *Controller) ManualCheckAppUpdate(ctx context.Context) (string, error) {
	info, err := downloader.CheckAppUpdate(ctx, c.version)
	if err != nil {
		return "", err
	}
	if info != nil && info.HasUpdate {
		return info.Version, nil
	}
	return "", nil
}

func (c *Controller) GetCoreVersion() string {
	return clash.GetVersion()
}
