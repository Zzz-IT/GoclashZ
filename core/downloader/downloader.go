package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Options struct {
	URL          string
	DestPath     string
	UserAgent    string
	ExpectedSHA  string
	MaxBytes     int64
	Client       *http.Client
	Resume       bool
}

var defaultClient = &http.Client{
	Timeout: 60 * time.Second,
}

var largeClient = &http.Client{
	Timeout: 10 * time.Minute,
}

func DownloadAtomic(ctx context.Context, opt Options) error {
	if opt.URL == "" || opt.DestPath == "" {
		return fmt.Errorf("下载参数缺失")
	}

	client := opt.Client
	if client == nil {
		client = defaultClient
	}

	tmpPath := opt.DestPath + ".tmp"
	_ = os.Remove(tmpPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opt.URL, nil)
	if err != nil {
		return err
	}

	if opt.UserAgent == "" {
		opt.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GoclashZ/1.0"
	}
	req.Header.Set("User-Agent", opt.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(tmpPath)
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

	if opt.ExpectedSHA != "" {
		if err := VerifySHA256(tmpPath, opt.ExpectedSHA); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
	}

	if err := os.Rename(tmpPath, opt.DestPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

func DownloadLargeAtomic(ctx context.Context, url, dest, expectedSHA string, maxBytes int64) error {
	return DownloadAtomic(ctx, Options{
		URL:         url,
		DestPath:    dest,
		ExpectedSHA: expectedSHA,
		MaxBytes:    maxBytes,
		Client:      largeClient,
	})
}

func VerifySHA256(path string, expected string) error {
	expected = strings.TrimSpace(strings.ToLower(expected))
	if expected == "" {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, expected)
	}
	return nil
}
