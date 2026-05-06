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
	"time"
)

type appUpdateDownloadStrategy int

const (
	appUpdateDirectThenProxy appUpdateDownloadStrategy = iota
	appUpdateCurrentProxyOnly
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
		local := clash.GetLocalCoreVersion(ctx)

		remote, assetURL, releaseURL, err := clash.CheckLatestCore(ctx, resolveLocalProxyURL())
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
	// ⚠️ 建议废弃此入口，前端统一改用 CheckAppUpdateAsync
	info, err := downloader.CheckAppUpdate(ctx, c.version)
	if err != nil {
		return "", err
	}
	if info.HasUpdate {
		return info.Version, nil
	}
	return "", nil
}

func (c *Controller) CheckAppUpdateAsync(ctx context.Context, currentVersion string, manual bool) {
	ok := c.Tasks.RunIfIdle(ctx, "app-update-flow", false, func(ctx context.Context) error {
		if manual {
			c.events.Emit("app-update-check-start")
		}

		info, err := downloader.CheckAppUpdate(ctx, currentVersion)
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

	// 🚀 自适应策略选择
	strategy := c.getAppUpdateDownloadStrategy()

	switch strategy {
	case appUpdateCurrentProxyOnly:
		fmt.Println("检测到系统代理/TUN 已开启，进入自适应模式：代理优先下载")
		return c.downloadAppUpdateViaCurrentProxy(ctx, info, destDir)
	default:
		fmt.Println("检测到代理/TUN 未开启，进入自适应模式：直连优先，代理兜底")
		return c.downloadAppUpdateDirectThenProxy(ctx, info, destDir)
	}
}

func (c *Controller) getAppUpdateDownloadStrategy() appUpdateDownloadStrategy {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 如果用户已经手动开启了系统代理或 TUN，说明用户当前网络意图是走代理
	if c.sysProxyActive || c.tunActive {
		return appUpdateCurrentProxyOnly
	}

	return appUpdateDirectThenProxy
}

func (c *Controller) downloadAppUpdateViaCurrentProxy(
	ctx context.Context,
	info *downloader.AppUpdateInfo,
	destDir string,
) error {
	proxyURL := c.currentAppUpdateProxyURL(ctx)
	if proxyURL == "" {
		err := fmt.Errorf("当前代理不可用")
		c.events.Emit("app-update-error", "下载软件更新失败：当前代理不可用，请检查内核或切换节点后重试。")
		return err
	}

	path, err := downloader.DownloadAppUpdate(ctx, info, destDir, proxyURL, c.appUpdateBandwidthLimit)
	if err != nil {
		fmt.Printf("软件下载代理下载失败: %v\n", err)
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

func (c *Controller) downloadAppUpdateDirectThenProxy(
	ctx context.Context,
	info *downloader.AppUpdateInfo,
	destDir string,
) error {
	// 第一轮：直连
	path, err := downloader.DownloadAppUpdate(ctx, info, destDir, "", c.appUpdateBandwidthLimit)
	if err == nil {
		c.SetDownloadedAppUpdate(path, info.Version)
		c.events.Emit("app-update-downloaded", map[string]string{
			"version": info.Version,
			"path":    path,
		})
		return nil
	}

	fmt.Printf("软件更新直连下载失败，准备尝试本地代理兜底: %v\n", err)

	// 第二轮：代理兜底
	proxyURL := c.currentAppUpdateProxyURL(ctx)
	if proxyURL != "" {
		fmt.Printf("开始使用本地代理重试下载: %s\n", proxyURL)
		path, err = downloader.DownloadAppUpdate(ctx, info, destDir, proxyURL, c.appUpdateBandwidthLimit)
		if err == nil {
			c.SetDownloadedAppUpdate(path, info.Version)
			c.events.Emit("app-update-downloaded", map[string]string{
				"version": info.Version,
				"path":    path,
			})
			return nil
		}
		fmt.Printf("软件更新代理重试也失败: %v\n", err)
	}

	c.events.Emit("app-update-error", userFacingAppUpdateDownloadError(err))
	return err
}

func (c *Controller) currentAppUpdateProxyURL(ctx context.Context) string {
	// 1. 如果当前已经有可用的代理端口，直接返回
	if proxyURL := resolveLocalProxyURL(); proxyURL != "" {
		return proxyURL
	}

	// 🚀 优化：只有用户已经开启代理/TUN 时，才尝试恢复/启动当前代理用于更新
	if c.getAppUpdateDownloadStrategy() == appUpdateCurrentProxyOnly {
		proxyCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		defer cancel()

		c.coreLifecycleMu.Lock()
		// 注意：这里使用 ensureCoreRunningLocked 会设置 userCoreRunning=true，
		// 因为用户当前本身就在代理/TUN 模式，内核理应在线。
		err := c.ensureCoreRunningLocked(proxyCtx)
		c.coreLifecycleMu.Unlock()

		if err != nil {
			fmt.Printf("获取软件下载代理失败 (内核启动失败): %v\n", err)
			return ""
		}

		return resolveLocalProxyURL()
	}

	// 未开启代理/TUN 时，不主动启动内核，保持直连
	return ""
}

func (c *Controller) appUpdateBandwidthLimit() int64 {
	if c.isAutoDelayRunning() {
		// 🚀 测速竞争模式：将下载限速至 384 KB/s，确保测速包能挤过去
		return 384 * 1024
	}

	// 正常后台模式：限速 2 MB/s，既保证下载速度又不至于吃满代理连接
	return 2 * 1024 * 1024
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
