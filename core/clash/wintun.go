package clash

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"goclashz/core/downloader"
	"goclashz/core/sys"
	"goclashz/core/utils"
)

const wintunURL = "https://www.wintun.net/builds/wintun-0.14.1.zip"

var wintunBinaryMu sync.Mutex

func PrepareWintunRuntime(ctx context.Context, proxyURL string) (map[string]string, error) {
	wintunBinaryMu.Lock()
	defer wintunBinaryMu.Unlock()

	destPath := filepath.Join(utils.GetCoreBinDir(), "wintun.dll")
	zipPath := destPath + ".zip"
	stagedDLL := destPath + ".new"

	_ = os.Remove(zipPath)
	_ = os.Remove(stagedDLL)

	if err := downloader.DownloadLargeAssetAtomic(ctx, downloader.Options{
		URLs:                []string{wintunURL},
		DestPath:            zipPath,
		ProxyURL:            proxyURL,
		PreferProxy:         proxyURL != "",
		MaxBytes:            50 << 20,
		UserAgent:           "GoclashZ-WintunUpdater",
		AttemptsPerEndpoint: 3,
		Validator: func(tmpPath string) error {
			return validateWintunZip(tmpPath)
		},
	}); err != nil {
		return nil, err
	}

	if err := extractWintunDLL(zipPath, stagedDLL); err != nil {
		_ = os.Remove(stagedDLL)
		return nil, err
	}

	if err := ValidateWindowsPE(stagedDLL, 32*1024); err != nil {
		_ = os.Remove(stagedDLL)
		return nil, err
	}

	return map[string]string{
		"stagedDLL": stagedDLL,
		"destPath":  destPath,
	}, nil
}

func CommitWintunRuntime(ctx context.Context, prepared map[string]string) (string, error) {
	wintunBinaryMu.Lock()
	defer wintunBinaryMu.Unlock()

	stagedDLL := prepared["stagedDLL"]
	destPath := prepared["destPath"]

	if stagedDLL == "" || destPath == "" {
		return "", fmt.Errorf("Wintun staging 信息缺失")
	}

	if err := WaitFileReleased(destPath, 5*time.Second); err != nil {
		return "", err
	}

	if err := ReplaceFileWithBackup(stagedDLL, destPath); err != nil {
		return "", err
	}

	version, err := sys.GetFileVersion(destPath)
	if err != nil || strings.TrimSpace(version) == "" {
		return "已安装，版本未知", nil
	}

	return version, nil
}

func validateWintunZip(path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("Wintun 压缩包无效: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		name := strings.ToLower(strings.ReplaceAll(f.Name, "\\", "/"))

		if strings.HasSuffix(name, "/amd64/wintun.dll") ||
			strings.HasSuffix(name, "/x64/wintun.dll") ||
			name == "wintun.dll" {
			if f.UncompressedSize64 < 32*1024 {
				return fmt.Errorf("wintun.dll 体积异常")
			}
			return nil
		}
	}

	return fmt.Errorf("压缩包中未找到 amd64 wintun.dll")
}

func extractWintunDLL(zipPath, destPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	var target *zip.File

	for _, f := range r.File {
		name := strings.ToLower(strings.ReplaceAll(f.Name, "\\", "/"))

		if strings.HasSuffix(name, "/amd64/wintun.dll") ||
			strings.HasSuffix(name, "/x64/wintun.dll") ||
			name == "wintun.dll" {
			target = f
			break
		}
	}

	if target == nil {
		return fmt.Errorf("zip 中未找到 amd64 wintun.dll")
	}

	rc, err := target.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	f, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, rc); err != nil {
		f.Close()
		return err
	}

	return f.Close()
}
