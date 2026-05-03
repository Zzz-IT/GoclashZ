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
			assetURL := ""
			// 优先使用前端检查更新时缓存的下载地址
			c.mu.RLock()
			cachedURL := c.pendingCoreUpdateAssetURL
			c.mu.RUnlock()

			if cachedURL != "" {
				assetURL = cachedURL
			} else {
				_, discoveredURL, _, err := clash.CheckLatestCore(ctx, resolveLocalProxyURL())
				if err != nil {
					return nil, err
				}
				assetURL = discoveredURL
			}

			return clash.PrepareCoreUpdate(ctx, assetURL, resolveLocalProxyURL())
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

			c.mu.Lock()
			c.pendingCoreUpdateAssetURL = ""
			c.pendingCoreUpdateVersion = ""
			c.mu.Unlock()
		},
	})
}

func (c *Controller) CheckCoreUpdateAsync(ctx context.Context) {
	c.Tasks.Run(ctx, "core-update-check", true, func(ctx context.Context) error {
		c.events.Emit("core-update-check-start")
		local := clash.GetLocalCoreVersion(ctx)

		remote, assetURL, releaseURL, err := clash.CheckLatestCore(ctx, resolveLocalProxyURL())
		if err != nil {
			c.events.Emit("core-update-check-error", err.Error())
			return err
		}

		c.mu.Lock()
		c.pendingCoreUpdateAssetURL = assetURL
		c.pendingCoreUpdateVersion = remote
		c.mu.Unlock()

		cmp, _ := clash.CompareCoreVersion(remote, local)
		if cmp <= 0 {
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
	// ⚠️ 建议废弃此入口，前端统一改用 CheckAndDownloadAppUpdateAsync
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
	c.Tasks.Run(ctx, "app-update-flow", false, func(ctx context.Context) error {
		c.events.Emit("app-update-check-start")

		info, err := downloader.CheckAppUpdate(ctx, currentVersion)
		if err != nil {
			c.events.Emit("app-update-error", "检查软件更新失败: "+err.Error())
			return nil
		}

		if !info.HasUpdate {
			c.events.Emit("app-update-none", map[string]string{
				"message": "当前已经是最新版本。",
			})
			return nil
		}

		if info.DownloadURL == "" {
			c.events.Emit("app-update-error", fmt.Sprintf(
				"发现新版本 %s，但 Release 中没有匹配的软件本体安装包。",
				info.Version,
			))
			return nil
		}

		c.SetUpdateStatus(true, info.Version)

		// 发现新版本：前端弹提示卡片
		c.events.Emit("app-update-available", map[string]any{
			"version":      info.Version,
			"releaseNotes": info.Body,
			"releaseUrl":   info.ReleaseURL,
			"downloadUrl":  info.DownloadURL,
		})

		// 后台静默下载开始
		c.events.Emit("app-update-start", map[string]string{
			"version": info.Version,
		})

		destDir := filepath.Join(utils.GetDataDir(), "updates")
		path, err := downloader.DownloadAppUpdate(ctx, info, destDir)
		if err != nil {
			c.events.Emit("app-update-error", "下载软件更新失败: "+err.Error())
			return nil
		}

		c.SetDownloadedAppUpdate(path, info.Version)

		c.events.Emit("app-update-downloaded", map[string]string{
			"version": info.Version,
			"path":    path,
		})

		return nil
	})
}

func (c *Controller) AutoCheckAndDownloadAppUpdateAsync(ctx context.Context, currentVersion string) {
	go func() {
		info, err := downloader.CheckAppUpdate(ctx, currentVersion)
		if err != nil || !info.HasUpdate {
			return
		}
		// 复用完整流程
		c.CheckAndDownloadAppUpdateAsync(ctx, currentVersion)
	}()
}
