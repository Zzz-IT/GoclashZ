//go:build windows

package appcore

import (
	"context"
	"fmt"
	"goclashz/core/clash"
	"goclashz/core/downloader"
	"goclashz/core/utils"
	"path/filepath"
)

func (c *Controller) updateGeoDatabase(ctx context.Context, key string) error {
	var url string
	behavior := c.Behavior.Get()

	switch key {
	case "geoip":
		url = behavior.GeoIpLink
	case "geosite":
		url = behavior.GeoSiteLink
	case "mmdb":
		url = behavior.MmdbLink
	case "asn":
		url = behavior.AsnLink
	default:
		return fmt.Errorf("unknown geo key: %s", key)
	}

	if url == "" {
		return fmt.Errorf("下载链接未配置")
	}

	return clash.UpdateGeoDB(ctx, key, url, resolveLocalProxyURL())
}

func (c *Controller) UpdateGeoDatabaseAsync(ctx context.Context, key string) {
	c.GeoUpdates.UpdateOneAsync(ctx, key)
}

func (c *Controller) UpdateAllGeoDatabasesAsync(ctx context.Context) {
	c.GeoUpdates.UpdateAllAsync(ctx)
}

func (c *Controller) GetActiveGeoUpdates() []string {
	if c.GeoUpdates == nil {
		return nil
	}
	return c.GeoUpdates.ActiveKeys()
}

func (c *Controller) UpdateCoreComponentAsync(ctx context.Context) {
	c.runComponentUpdateTransaction(ctx, "core-update", ComponentUpdateOptions{
		Name:        "Mihomo 内核更新",
		StopCore:    true,
		RestartCore: true,
		Prepare: func(ctx context.Context) (map[string]string, error) {
			return clash.PrepareCoreUpdate(ctx, resolveLocalProxyURL())
		},
		Commit: func(ctx context.Context, prepared map[string]string) (map[string]string, error) {
			version, err := clash.CommitCoreUpdate(ctx, prepared)
			if err != nil {
				return nil, err
			}
			return map[string]string{
				"version": version,
			}, nil
		},
		AfterSuccess: func(result map[string]string) {
			if version := result["version"]; version != "" {
				c.events.Emit("core-version-updated", map[string]string{
					"version": version,
				})
			}

			// 内核二进制更新后，连接和延迟状态不应沿用旧进程
			c.events.Emit("delay-cache-clear", "core-update")
		},
	})
}

func (c *Controller) CheckCoreUpdateAsync(ctx context.Context) {
	c.Tasks.Run(ctx, "core-update-check", true, func(ctx context.Context) error {
		local := clash.GetLocalCoreVersion(ctx)

		remote, releaseURL, err := clash.CheckLatestCoreVersion(ctx, resolveLocalProxyURL())
		if err != nil {
			return err
		}

		if clash.CompareCoreVersion(remote, local) <= 0 {
			c.events.Emit("core-update-none", map[string]string{
				"local":  local,
				"remote": remote,
			})
			return nil
		}

		c.events.Emit("core-update-available", map[string]string{
			"local":      local,
			"remote":     remote,
			"releaseUrl": releaseURL,
		})
		return nil
	})
}

func (c *Controller) InstallTunDriverAsync(ctx context.Context) {
	c.runComponentUpdateTransaction(ctx, "driver-install", ComponentUpdateOptions{
		Name:        "Wintun 重装",
		StopCore:    true,
		RestartCore: true,
		Prepare: func(ctx context.Context) (map[string]string, error) {
			return clash.PrepareWintunRuntime(ctx, resolveLocalProxyURL())
		},
		Commit: func(ctx context.Context, prepared map[string]string) (map[string]string, error) {
			version, err := clash.CommitWintunRuntime(ctx, prepared)
			if err != nil {
				return nil, err
			}
			return map[string]string{
				"version": version,
			}, nil
		},
		AfterSuccess: func(result map[string]string) {
			if version := result["version"]; version != "" {
				c.events.Emit("wintun-version-updated", map[string]string{
					"version": version,
				})
			}
		},
	})
}


func (c *Controller) GetCoreVersion(ctx context.Context) string {
	return clash.GetLocalCoreVersion(ctx)
}

func (c *Controller) ManualCheckAppUpdate(ctx context.Context) (string, error) {
	info, err := downloader.CheckAppUpdate(ctx, c.version)
	if err != nil {
		return "", err
	}
	if info.HasUpdate {
		return info.Version, nil
	}
	return "", nil
}

func (c *Controller) CheckAndDownloadAppUpdateAsync(ctx context.Context, currentVersion string) {
	c.Tasks.Run(ctx, "app-update", true, func(ctx context.Context) error {
		c.events.Emit("app-update-check-start")
		info, err := downloader.CheckAppUpdate(ctx, currentVersion)
		if err != nil {
			c.events.Emit("app-update-check-error", err.Error())
			return err
		}

		if !info.HasUpdate {
			c.events.Emit("app-update-no-new")
			return nil
		}

		c.SetUpdateStatus(true, info.Version)
		c.events.Emit("app-update-ready", info.Version)

		// 开始后台下载
		c.events.Emit("app-update-download-start", info.Version)
		destDir := filepath.Join(utils.GetDataDir(), "updates")
		path, err := downloader.DownloadAppUpdate(ctx, info, destDir)
		if err != nil {
			c.events.Emit("app-update-download-error", err.Error())
			return err
		}

		c.SetDownloadedAppUpdate(path, info.Version)
		c.events.Emit("app-update-download-success", map[string]string{
			"version": info.Version,
			"path":    path,
		})
		return nil
	})
}

func (c *Controller) AutoCheckAndDownloadAppUpdateAsync(ctx context.Context, currentVersion string) {
	go func() {
		info, err := downloader.CheckAppUpdate(ctx, currentVersion)
		if err == nil && info.HasUpdate {
			c.CheckAndDownloadAppUpdateAsync(ctx, currentVersion)
		}
	}()
}
