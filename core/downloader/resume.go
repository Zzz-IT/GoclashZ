//go:build windows

package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func DownloadResumeAtomic(ctx context.Context, opt Options) error {
	if strings.TrimSpace(opt.DestPath) == "" {
		return fmt.Errorf("DestPath 为空")
	}

	unlock := lockDest(opt.DestPath)
	defer unlock()

	clients := createOrderedClients(opt)
	ua := opt.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GoclashZ/1.0"
	}

	candidates, err := probeDownloadCandidates(ctx, clients, opt)
	if err != nil {
		return SanitizeDownloadError(err)
	}

	var lastErr error
	tmpPath := opt.DestPath + ".tmp"
	metaFile := metaPath(tmpPath)

	for _, candidate := range candidates {
		for attempt := 1; attempt <= 2; attempt++ {
			err := downloadWithCandidate(ctx, opt, candidate, ua, tmpPath, metaFile)
			if err == nil {
				return nil
			}

			lastErr = err
			if !isTransientDownloadError(err) {
				break
			}
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}

		if isTransientDownloadError(lastErr) {
			_ = os.Remove(tmpPath)
			_ = os.Remove(metaFile)
		}
	}

	return SanitizeDownloadError(lastErr)
}

func probeDownloadCandidates(ctx context.Context, clients []*http.Client, opt Options) ([]downloadCandidate, error) {
	urls := opt.URLs
	totalRaces := len(clients) * len(urls)
	resultCh := make(chan envProbeResult, totalRaces)

	raceCtx, cancelRace := context.WithCancel(ctx)
	defer cancelRace()

	for _, client := range clients {
		for _, u := range urls {
			go func(c *http.Client, target string) {
				meta, partial, err := probeSingleRemote(raceCtx, c, target, opt)
				if err == nil {
					resultCh <- envProbeResult{meta: meta, canPartial: partial, client: c, err: nil}
				} else {
					resultCh <- envProbeResult{err: err}
				}
			}(client, u)
		}
	}

	var candidates []downloadCandidate
	var lastErr error

	for i := 0; i < totalRaces; i++ {
		res := <-resultCh
		if res.err != nil {
			lastErr = res.err
			continue
		}
		candidates = append(candidates, downloadCandidate{
			meta:      res.meta,
			canResume: res.canPartial,
			client:    res.client,
		})
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("所有网络环境与镜像均探测失败: %v", lastErr)
	}

	return candidates, nil
}

func probeSingleRemote(ctx context.Context, client *http.Client, testURL string, opt Options) (resumeMeta, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, testURL, nil)
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
		return resumeMeta{}, false, err
	}

	if resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		req, _ = http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Range", "bytes=0-0")
		resp, err = client.Do(req)
		if err != nil {
			return resumeMeta{}, false, err
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resumeMeta{}, false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	total := resp.ContentLength
	acceptRanges := strings.Contains(strings.ToLower(resp.Header.Get("Accept-Ranges")), "bytes")

	return resumeMeta{
		URL:          resp.Request.URL.String(),
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		TotalSize:    total,
	}, acceptRanges && total > 0, nil
}

type envProbeResult struct {
	meta       resumeMeta
	canPartial bool
	client     *http.Client
	err        error
}

type downloadCandidate struct {
	meta       resumeMeta
	canResume  bool
	client     *http.Client
}

func downloadWithCandidate(ctx context.Context, opt Options, candidate downloadCandidate, ua, tmpPath, metaFile string) error {
	var oldMeta resumeMeta
	var canResume bool
	var startOffset int64

	if opt.Resume {
		if info, err := os.Stat(tmpPath); err == nil && !info.IsDir() && info.Size() > 0 {
			if meta, ok := readResumeMeta(metaFile); ok {
				oldMeta = meta
				startOffset = info.Size()
				canResume = true
			}
		}
	}

	remoteMeta := candidate.meta
	if canResume {
		if (oldMeta.ETag != "" && remoteMeta.ETag != "" && oldMeta.ETag != remoteMeta.ETag) ||
			(oldMeta.LastModified != "" && remoteMeta.LastModified != "" && oldMeta.LastModified != remoteMeta.LastModified) ||
			(oldMeta.TotalSize > 0 && remoteMeta.TotalSize > 0 && oldMeta.TotalSize != remoteMeta.TotalSize) {
			canResume = false
		}
	}

	if !candidate.canResume || !canResume || startOffset <= 0 {
		startOffset = 0
	}

	if startOffset > 0 && remoteMeta.TotalSize > 0 && startOffset >= remoteMeta.TotalSize {
		// 已下载完
	} else {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteMeta.URL, nil)
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

		resp, err := candidate.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if opt.OnResponse != nil {
			opt.OnResponse(resp)
		}

		appendMode := false
		switch resp.StatusCode {
		case http.StatusOK:
			startOffset = 0
		case http.StatusPartialContent:
			appendMode = true
		default:
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return fmt.Errorf("HTTP %d", resp.StatusCode)
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
				return fmt.Errorf("文件超过大小限制")
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
			return fmt.Errorf("文件超过大小限制")
		}
	}

	// 最终检查与校验
	if remoteMeta.TotalSize > 0 {
		info, err := os.Stat(tmpPath)
		if err != nil {
			return err
		}
		if info.Size() != remoteMeta.TotalSize {
			return fmt.Errorf("下载未完成: %d/%d", info.Size(), remoteMeta.TotalSize)
		}
	}

	if opt.VerifyGitHubSHA {
		expectedSHA, err := ResolveGitHubAssetSHA256(ctx, candidate.client, remoteMeta.URL, ua)
		if err != nil {
			if opt.RequireGitHubSHA {
				return fmt.Errorf("获取 SHA256 失败: %v", err)
			}
		} else if expectedSHA != "" {
			if err := VerifySHA256(tmpPath, expectedSHA); err != nil {
				return err
			}
		}
	}

	if opt.Validator != nil {
		if err := opt.Validator(tmpPath); err != nil {
			return err
		}
	}

	if err := ReplaceFile(tmpPath, opt.DestPath); err != nil {
		return err
	}

	_ = os.Remove(metaFile)
	return nil
}
