//go:build windows

package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func DownloadLargeAssetAtomic(ctx context.Context, opt Options) error {
	if strings.TrimSpace(opt.DestPath) == "" {
		return fmt.Errorf("DestPath 为空")
	}
	if len(opt.URLs) == 0 {
		return fmt.Errorf("下载地址为空")
	}

	unlock := lockDest(opt.DestPath)
	defer unlock()

	attempts := opt.AttemptsPerEndpoint
	if attempts <= 0 {
		attempts = 3
	}

	clients := createOrderedClients(opt)
	ua := opt.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GoclashZ/1.0"
	}

	var lastErr error

	for _, client := range clients {
		for _, rawURL := range opt.URLs {
			for attempt := 1; attempt <= attempts; attempt++ {
				err := downloadLargeAssetOnce(ctx, client, rawURL, opt, ua)
				if err == nil {
					return nil
				}

				lastErr = err
				if !isTransientDownloadError(err) {
					break
				}

				select {
				case <-time.After(time.Duration(attempt) * 800 * time.Millisecond):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("下载失败")
	}

	return SanitizeDownloadError(lastErr)
}

func downloadLargeAssetOnce(
	ctx context.Context,
	client *http.Client,
	rawURL string,
	opt Options,
	ua string,
) error {
	release := acquireHostLimiter(rawURL)
	defer release()

	tmpPath := opt.DestPath + ".tmp"
	metaFile := metaPath(tmpPath)

	_ = os.Remove(tmpPath)
	_ = os.Remove(metaFile)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/octet-stream,*/*")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if opt.OnResponse != nil {
		opt.OnResponse(resp)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(opt.DestPath), 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	var reader io.Reader = resp.Body
	if opt.MaxBytes > 0 {
		reader = io.LimitReader(resp.Body, opt.MaxBytes+1)
	}

	n, copyErr := io.Copy(out, reader)
	closeErr := out.Close()

	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}

	if opt.MaxBytes > 0 && n > opt.MaxBytes {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("下载文件超过大小限制")
	}

	if resp.ContentLength > 0 && n < resp.ContentLength {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("下载未完成: %d/%d", n, resp.ContentLength)
	}

	if opt.VerifyGitHubSHA {
		expectedSHA, err := ResolveGitHubAssetSHA256(ctx, client, resp.Request.URL.String(), ua)
		if err != nil {
			if opt.RequireGitHubSHA {
				_ = os.Remove(tmpPath)
				return fmt.Errorf("获取 SHA256 失败: %v", err)
			}
		} else if expectedSHA != "" {
			if err := VerifySHA256(tmpPath, expectedSHA); err != nil {
				_ = os.Remove(tmpPath)
				return err
			}
		}
	}

	if opt.Validator != nil {
		if err := opt.Validator(tmpPath); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("安全校验失败: %v", err)
		}
	}

	if err := ReplaceFile(tmpPath, opt.DestPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}
