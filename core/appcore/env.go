//go:build windows

package appcore

import (
	"context"
	"fmt"
	"goclashz/core/downloader"
	"goclashz/core/utils"
	"os"
	"path/filepath"
	"strings"
)

func EnsureRuntimeAssets(ctx context.Context, behavior AppBehavior) error {
	binDir := utils.GetCoreBinDir()

	required := []struct {
		Name string
		URL  string
	}{
		{"geoip.metadb", behavior.GeoIpLink},
		{"geosite.dat", behavior.GeoSiteLink},
		{"country.mmdb", behavior.MmdbLink},
		{"asn.dat", behavior.AsnLink},
	}

	for _, item := range required {
		path := filepath.Join(binDir, item.Name)
		if st, err := os.Stat(path); err == nil && !st.IsDir() && st.Size() > 0 {
			continue
		}

		if strings.TrimSpace(item.URL) == "" {
			continue
		}

		if err := downloader.DownloadLargeAssetAtomic(ctx, downloader.Options{
			URLs:                []string{item.URL},
			DestPath:            path,
			Strategy:            func() downloader.DownloadStrategy { return downloader.DownloadStrategy{} },
			MaxBytes:            200 << 20,
			UserAgent:           "GoclashZ-EnvUpdater",
			AttemptsPerEndpoint: 3,
		}); err != nil {
			return fmt.Errorf("下载 %s 失败: %w", item.Name, err)
		}
	}

	return nil
}
