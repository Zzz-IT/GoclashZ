package appcore

import (
	"context"
	"fmt"
	"goclashz/core/clash"
	"goclashz/core/downloader"
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

func (c *Controller) CheckAndDownloadAppUpdateAsync(ctx context.Context, currentVersion string) {
	c.Tasks.Run(ctx, "app-update-check", false, func(ctx context.Context) error {
		info, err := downloader.CheckAppUpdate(ctx, currentVersion)
		if err != nil {
			return err
		}
		if info != nil && info.HasUpdate {
			c.SetUpdateStatus(true, info.Version)
			// 🚀 核心修复：对齐事件名与 Payload
			c.events.Emit("app-update-available", map[string]any{
				"version":      info.Version,
				"releaseNotes": info.Body,
			})
		}
		return nil
	})
}
