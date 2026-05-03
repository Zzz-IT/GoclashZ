//go:build windows

package appcore

import (
	"context"
	"fmt"
	"goclashz/core/clash"
	"goclashz/core/downloader"
	"goclashz/core/utils"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

	return clash.UpdateGeoDB(ctx, key, url)
}

func (c *Controller) UpdateGeoDatabaseAsync(ctx context.Context, key string) {
	title, _ := clash.GeoDBFileName(key)
	if title == "" {
		title = key
	}

	c.Tasks.Run(ctx, "geo-update-"+key, true, func(ctx context.Context) error {
		c.events.Emit("geo-update-"+key+"-start")

		if err := c.updateGeoDatabase(ctx, key); err != nil {
			c.events.Emit("geo-update-"+key+"-error", err.Error())
			return err
		}

		c.events.Emit("geo-update-"+key+"-success")
		return nil
	})
}

func (c *Controller) UpdateAllGeoDatabasesAsync(ctx context.Context) {
	c.Tasks.Run(ctx, "geo-update-all", true, func(ctx context.Context) error {
		keys := []string{"geoip", "geosite", "mmdb", "asn"}

		sem := make(chan struct{}, 2) // 建议 2 并发，不要 4 个大文件一起打满
		var wg sync.WaitGroup
		var mu sync.Mutex
		failed := make([]string, 0)

		for _, key := range keys {
			key := key
			wg.Add(1)

			go func() {
				defer wg.Done()

				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					mu.Lock()
					failed = append(failed, key+": "+ctx.Err().Error())
					mu.Unlock()
					return
				}

				c.events.Emit("geo-update-" + key + "-start")

				if err := c.updateGeoDatabase(ctx, key); err != nil {
					c.events.Emit("geo-update-"+key+"-error", err.Error())

					mu.Lock()
					failed = append(failed, fmt.Sprintf("%s: %v", key, err))
					mu.Unlock()
					return
				}

				c.events.Emit("geo-update-" + key + "-success")
			}()
		}

		wg.Wait()

		if len(failed) > 0 {
			return fmt.Errorf("%s", strings.Join(failed, "; "))
		}

		return nil
	})
}

func (c *Controller) UpdateCoreComponentAsync(ctx context.Context) {
	c.Tasks.Run(ctx, "core-update", true, func(ctx context.Context) error {
		c.events.Emit("core-update-start")

		newVer, err := clash.UpdateCore(ctx)
		if err != nil {
			c.events.Emit("core-update-error", err.Error())
			return err
		}

		c.events.Emit("core-version-updated", map[string]string{"version": newVer})
		c.events.Emit("core-update-success")
		return nil
	})
}

func (c *Controller) InstallTunDriverAsync(ctx context.Context) {
	c.Tasks.Run(ctx, "driver-install", true, func(ctx context.Context) error {
		c.events.Emit("driver-install-start")

		// 🎯 核心逻辑：下载并替换 wintun.dll
		url := "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/wintun-amd64.zip"
		destPath := filepath.Join(utils.GetCoreBinDir(), "wintun.dll")

		// 下载 Zip 并提取
		zipPath := destPath + ".zip"
		defer os.Remove(zipPath)

		err := downloader.DownloadAtomic(ctx, downloader.Options{
			URLs:     []string{url},
			DestPath: zipPath,
		})
		if err != nil {
			c.events.Emit("driver-install-error", err.Error())
			return err
		}

		// 提取 (简化处理，假设 zip 里就是 dll)
		if err := downloader.ExtractFileFromZip(zipPath, "wintun.dll", destPath); err != nil {
			c.events.Emit("driver-install-error", err.Error())
			return err
		}

		c.events.Emit("driver-install-success")
		return nil
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
