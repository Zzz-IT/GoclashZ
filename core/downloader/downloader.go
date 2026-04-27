package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Options struct {
	URL             string
	DestPath        string
	UserAgent       string
	MaxBytes        int64
	Client          *http.Client
	Resume          bool
	VerifyGitHubSHA bool
}

type resumeMeta struct {
	URL          string `json:"url"`
	ETag         string `json:"etag"`
	LastModified string `json:"lastModified"`
	TotalSize    int64  `json:"totalSize"`
}

var defaultClient = &http.Client{
	Timeout: 60 * time.Second,
}

var largeClient = &http.Client{
	Timeout: 10 * time.Minute,
}

func metaPath(tmpPath string) string {
	return tmpPath + ".meta.json"
}

func readResumeMeta(path string) (resumeMeta, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return resumeMeta{}, false
	}

	var meta resumeMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return resumeMeta{}, false
	}
	return meta, true
}

func writeResumeMeta(path string, meta resumeMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func probeRemote(ctx context.Context, client *http.Client, opt Options) (resumeMeta, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, opt.URL, nil)
	if err != nil {
		return resumeMeta{}, false, err
	}

	ua := opt.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GoclashZ/1.0"
	}
	req.Header.Set("User-Agent", ua)

	resp, err := client.Do(req)
	if err != nil {
		return resumeMeta{}, false, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resumeMeta{}, false, nil
	}

	total := resp.ContentLength
	acceptRanges := strings.Contains(strings.ToLower(resp.Header.Get("Accept-Ranges")), "bytes")

	return resumeMeta{
		URL:          opt.URL,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		TotalSize:    total,
	}, acceptRanges && total > 0, nil
}

func parseContentRangeTotal(v string) (int64, bool) {
	idx := strings.LastIndex(v, "/")
	if idx < 0 || idx+1 >= len(v) {
		return 0, false
	}

	totalStr := strings.TrimSpace(v[idx+1:])
	if totalStr == "*" {
		return 0, false
	}

	total, err := strconv.ParseInt(totalStr, 10, 64)
	if err != nil || total <= 0 {
		return 0, false
	}
	return total, true
}

func DownloadAtomic(ctx context.Context, opt Options) error {
	if opt.URL == "" || opt.DestPath == "" {
		return fmt.Errorf("下载参数缺失")
	}

	client := opt.Client
	if client == nil {
		client = defaultClient
	}

	ua := opt.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GoclashZ/1.0"
	}

	tmpPath := opt.DestPath + ".tmp"
	metaFile := metaPath(tmpPath)

	var startOffset int64
	var oldMeta resumeMeta
	var canResume bool

	if opt.Resume {
		if info, err := os.Stat(tmpPath); err == nil && !info.IsDir() && info.Size() > 0 {
			if meta, ok := readResumeMeta(metaFile); ok && meta.URL == opt.URL {
				oldMeta = meta
				startOffset = info.Size()
				canResume = true
			}
		}
	} else {
		_ = os.Remove(tmpPath)
		_ = os.Remove(metaFile)
	}

	remoteMeta, serverSupportsRange, _ := probeRemote(ctx, client, opt)
	if remoteMeta.URL == "" {
		remoteMeta.URL = opt.URL
	}

	if canResume {
		if oldMeta.ETag != "" && remoteMeta.ETag != "" && oldMeta.ETag != remoteMeta.ETag {
			canResume = false
		}
		if oldMeta.LastModified != "" && remoteMeta.LastModified != "" && oldMeta.LastModified != remoteMeta.LastModified {
			canResume = false
		}
		if oldMeta.TotalSize > 0 && remoteMeta.TotalSize > 0 && oldMeta.TotalSize != remoteMeta.TotalSize {
			canResume = false
		}
	}

	if !serverSupportsRange || !canResume || startOffset <= 0 {
		startOffset = 0
		_ = os.Remove(tmpPath)
		_ = os.Remove(metaFile)
	}

	// 尝试下载或续传
	if startOffset > 0 && remoteMeta.TotalSize > 0 && startOffset >= remoteMeta.TotalSize {
		// 已下载完，跳过下载阶段
	} else {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, opt.URL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", ua)

		if startOffset > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))
			if remoteMeta.ETag != "" {
				req.Header.Set("If-Range", remoteMeta.ETag)
			} else if remoteMeta.LastModified != "" {
				req.Header.Set("If-Range", remoteMeta.LastModified)
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		appendMode := false
		switch resp.StatusCode {
		case http.StatusOK:
			startOffset = 0
			_ = os.Remove(tmpPath)
			_ = os.Remove(metaFile)
		case http.StatusPartialContent:
			appendMode = true
		default:
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
			}
		}

		if remoteMeta.ETag == "" {
			remoteMeta.ETag = resp.Header.Get("ETag")
		}
		if remoteMeta.LastModified == "" {
			remoteMeta.LastModified = resp.Header.Get("Last-Modified")
		}
		if remoteMeta.TotalSize <= 0 {
			if resp.StatusCode == http.StatusPartialContent {
				if total, ok := parseContentRangeTotal(resp.Header.Get("Content-Range")); ok {
					remoteMeta.TotalSize = total
				}
			} else if resp.ContentLength > 0 {
				remoteMeta.TotalSize = resp.ContentLength
			}
		}

		_ = writeResumeMeta(metaFile, remoteMeta)

		flag := os.O_CREATE | os.O_WRONLY
		if appendMode {
			flag |= os.O_APPEND
		} else {
			flag |= os.O_TRUNC
		}

		out, err := os.OpenFile(tmpPath, flag, 0644)
		if err != nil {
			return err
		}

		var reader io.Reader = resp.Body
		if opt.MaxBytes > 0 {
			remain := opt.MaxBytes - startOffset
			if remain <= 0 {
				out.Close()
				return fmt.Errorf("下载文件超过大小限制")
			}
			reader = io.LimitReader(resp.Body, remain+1)
		}

		n, copyErr := io.Copy(out, reader)
		closeErr := out.Close()

		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}

		if opt.MaxBytes > 0 && startOffset+n > opt.MaxBytes {
			_ = os.Remove(tmpPath)
			_ = os.Remove(metaFile)
			return fmt.Errorf("下载文件超过大小限制")
		}
	}

	if remoteMeta.TotalSize > 0 {
		info, err := os.Stat(tmpPath)
		if err != nil {
			return err
		}
		if info.Size() < remoteMeta.TotalSize {
			return fmt.Errorf("下载未完成: %d/%d", info.Size(), remoteMeta.TotalSize)
		}
		if info.Size() > remoteMeta.TotalSize {
			_ = os.Remove(tmpPath)
			_ = os.Remove(metaFile)
			return fmt.Errorf("下载文件大小异常: %d/%d", info.Size(), remoteMeta.TotalSize)
		}
	}

	// 校验逻辑
	if opt.VerifyGitHubSHA {
		expectedSHA, err := ResolveGitHubAssetSHA256(ctx, client, opt.URL, ua)
		if err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("获取 GitHub 文件哈希失败: %v", err)
		}

		if err := VerifySHA256(tmpPath, expectedSHA); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("文件 SHA256 校验失败: %v", err)
		}
	}

	if err := ReplaceFile(tmpPath, opt.DestPath); err != nil {
		_ = os.Remove(tmpPath)
		_ = os.Remove(metaFile)
		return err
	}

	_ = os.Remove(metaFile)
	return nil
}

func ReplaceFile(tmpPath, destPath string) error {
	backupPath := destPath + ".bak"
	_ = os.Remove(backupPath)

	if _, err := os.Stat(destPath); err == nil {
		if err := os.Rename(destPath, backupPath); err != nil {
			return fmt.Errorf("备份旧文件失败: %w", err)
		}
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Rename(backupPath, destPath)
		return fmt.Errorf("替换目标文件失败: %w", err)
	}

	_ = os.Remove(backupPath)
	return nil
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
