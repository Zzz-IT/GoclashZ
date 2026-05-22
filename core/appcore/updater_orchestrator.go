//go:build windows

package appcore

import (
	"context"
	"fmt"
	"goclashz/core/clash"
	"goclashz/core/downloader"
	"goclashz/core/utils"
	"path/filepath"
	"strings"
)

func (c *Controller) updateGeoDatabase(ctx context.Context, key string, onProgress func(bytesDone, totalBytes, speedBps, etaSec int64)) error {
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

	return clash.UpdateGeoDB(ctx, key, url, c.getDynamicStrategy, onProgress)
}

func (c *Controller) UpdateGeoDatabaseAsync(ctx context.Context, key string) {
	c.GeoUpdates.UpdateOneAsync(ctx, key)
}

func (c *Controller) UpdateAllGeoDatabasesAsync(ctx context.Context) {
	c.GeoUpdates.UpdateAllAsync(ctx)
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
				_, discoveredURL, _, err := clash.CheckLatestCore(ctx, c.getDynamicStrategy)
				if err != nil {
					return nil, err
				}
				assetURL = discoveredURL
			}

			return clash.PrepareCoreUpdate(ctx, assetURL, c.getDynamicStrategy)
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
		local := clash.GetLocalCoreVersion(ctx)

		remote, assetURL, releaseURL, err := clash.CheckLatestCore(ctx, c.getDynamicStrategy)
		if err != nil {
			return err
		}

		cmp, err := clash.CompareCoreVersion(remote, local)
		if err != nil {
			return err
		}

		if cmp <= 0 {
			c.mu.Lock()
			c.pendingCoreUpdateAssetURL = ""
			c.pendingCoreUpdateVersion = ""
			c.mu.Unlock()

			c.events.Emit("core-update-none", map[string]string{
				"local":  local,
				"remote": remote,
			})
			return nil
		}

		c.mu.Lock()
		c.pendingCoreUpdateAssetURL = assetURL
		c.pendingCoreUpdateVersion = remote
		c.mu.Unlock()

		c.events.Emit("core-update-available", map[string]string{
			"local":      local,
			"remote":     remote,
			"assetUrl":   assetURL,
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
			return clash.PrepareWintunRuntime(ctx, c.getDynamicStrategy)
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


func (c *Controller) CheckAppUpdateAsync(ctx context.Context, currentVersion string, manual bool) {
	ok := c.Tasks.RunIfIdle(ctx, "app-update-flow", false, func(ctx context.Context) error {
		if manual {
			c.events.Emit("app-update-check-start")
		}

		info, err := downloader.CheckAppUpdate(ctx, currentVersion, c.getDynamicStrategy)
		if err != nil {
			if manual {
				c.events.Emit("app-update-error", "检查软件更新失败: "+err.Error())
			}
			return nil
		}

		if info == nil || !info.HasUpdate {
			if manual {
				c.events.Emit("app-update-none", map[string]string{
					"message": "当前已经是最新版本。",
				})
			}
			return nil
		}

		if info.DownloadURL == "" {
			if manual {
				c.events.Emit("app-update-error", fmt.Sprintf(
					"发现新版本 %s，但 Release 中没有匹配的软件本体安装包",
					info.Version,
				))
			}
			return nil
		}

		c.mu.Lock()
		c.pendingAppUpdateInfo = info
		c.mu.Unlock()

		c.events.Emit("app-update-available", map[string]any{
			"version": info.Version,
			"manual":  manual,
		})

		return nil
	})

	if !ok && manual {
		c.events.Emit("app-update-busy")
	}
}

func (c *Controller) DownloadPendingAppUpdateAsync(ctx context.Context) {
	ok := c.Tasks.RunIfIdle(ctx, "app-update-flow", false, func(ctx context.Context) error {
		c.mu.RLock()
		info := c.pendingAppUpdateInfo
		c.mu.RUnlock()

		if info == nil || !info.HasUpdate || info.DownloadURL == "" {
			c.events.Emit("app-update-error", "没有可下载的软件更新，请重新检查更新。")
			return nil
		}

		return c.downloadAppUpdateWithInfo(ctx, info)
	})

	if !ok {
		c.events.Emit("app-update-busy")
	}
}

func (c *Controller) CheckAndDownloadAppUpdateAsync(ctx context.Context, currentVersion string) {
	// 兼容接口：现在改为仅检查
	c.CheckAppUpdateAsync(ctx, currentVersion, true)
}

func (c *Controller) AutoCheckAndDownloadAppUpdateAsync(ctx context.Context, currentVersion string) {
	// 启动自动检查也改为仅检查
	c.CheckAppUpdateAsync(ctx, currentVersion, false)
}

func (c *Controller) downloadAppUpdateWithInfo(ctx context.Context, info *downloader.AppUpdateInfo) error {
	c.SetUpdateStatus(true, info.Version)
	c.setAppUpdateDownloading(true)
	defer c.setAppUpdateDownloading(false)

	c.events.Emit("app-update-start", map[string]string{
		"version": info.Version,
	})

	destDir := filepath.Join(utils.GetDataDir(), "updates")

	onProgress := func(bytesDone, totalBytes, speedBps, etaSec int64) {
		c.events.Emit("app-update-progress", map[string]interface{}{
			"bytesDone":  bytesDone,
			"totalBytes": totalBytes,
			"speedBps":   speedBps,
			"etaSec":     etaSec,
		})
	}

	// 🚀 这里只调用一次，底层的 DownloadLargeAssetAtomic 会通过 Strategy() 回调动态感知代理切换并自动进行内部无感断点续传！
	path, err := downloader.DownloadAppUpdate(ctx, info, destDir, onProgress, c.getDynamicStrategy)
	if err != nil {
		fmt.Printf("软件更新下载失败: %v\n", err)
		c.events.Emit("app-update-error", userFacingAppUpdateDownloadError(err))
		return err
	}

	c.SetDownloadedAppUpdate(path, info.Version)
	c.events.Emit("app-update-downloaded", map[string]string{
		"version": info.Version,
		"path":    path,
	})
	return nil
}

func (c *Controller) getDynamicStrategy() downloader.DownloadStrategy {
	c.mu.RLock()
	preferProxy := c.sysProxyActive || c.tunActive
	c.mu.RUnlock()

	// 注意：resolveLocalProxyURL() 只是获取格式化字符串，它不启动内核。
	// 如果用户当前没有开启代理（preferProxy 为 false），且直连失败了，
	// 下载机将会尝试走代理兜底。如果此时内核未启动，代理请求也会迅速失败（connection refused）。
	// 这是合理的行为，因为我们不希望在用户完全没有开启代理时偷偷在后台启动内核耗电。
	return downloader.DownloadStrategy{
		ProxyURL:    resolveLocalProxyURL(),
		PreferProxy: preferProxy,
	}
}

func userFacingAppUpdateDownloadError(err error) string {
	if err == nil {
		return "下载软件更新失败，请稍后重试。"
	}

	msg := strings.ToLower(err.Error())

	// 映射网络类错误
	if strings.Contains(msg, "wsarecv") ||
		strings.Contains(msg, "connection was aborted") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "所有网络环境") ||
		strings.Contains(msg, "github") {
		return "下载软件更新失败：当前网络无法连接下载地址，请检查网络或切换可用节点后重试。"
	}

	// 映射文件效验类错误
	if strings.Contains(msg, "mz") ||
		strings.Contains(msg, "不是有效的 windows 可执行文件") ||
		strings.Contains(msg, "体积异常") {
		return "下载的软件安装包无效，请稍后重新下载。"
	}

	return "下载软件更新失败，请稍后重试。"
}
