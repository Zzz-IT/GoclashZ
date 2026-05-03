//go:build windows

package downloader

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Options struct {
	URLs             []string // 🚀 竞速容灾：传入多个下载地址
	DestPath         string
	UserAgent        string
	MaxBytes         int64
	Client           *http.Client
	Resume           bool
	VerifyGitHubSHA  bool
	RequireGitHubSHA bool                       // SHA 为可选，Validator 为必须
	ProxyURL         string                     // 自代理加速：指定本地 Clash 代理地址
	InsecureSkipVerify bool                     // SSL 宽容
	OnResponse       func(resp *http.Response)  // 拦截器
	Validator        func(tmpPath string) error // 防损屏障：替换前执行验证逻辑
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

// createClients 生成网络环境矩阵（直连 vs 代理）
func createClients(opt Options) []*http.Client {
	if opt.Client != nil {
		return []*http.Client{opt.Client}
	}

	var clients []*http.Client
	tlsConfig := &tls.Config{InsecureSkipVerify: opt.InsecureSkipVerify}

	// [引擎 A]：直连客户端
	directTransport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSClientConfig:     tlsConfig,
		TLSHandshakeTimeout: 5 * time.Second,
		DisableKeepAlives:   true,
	}
	clients = append(clients, &http.Client{
		Timeout:   10 * time.Minute,
		Transport: directTransport,
	})

	// [引擎 B]：自代理客户端
	if opt.ProxyURL != "" {
		if pURL, err := url.Parse(opt.ProxyURL); err == nil {
			proxyTransport := &http.Transport{
				Proxy:               http.ProxyURL(pURL),
				TLSClientConfig:     tlsConfig,
				TLSHandshakeTimeout: 5 * time.Second,
				DisableKeepAlives:   true,
			}
			clients = append(clients, &http.Client{
				Timeout:   10 * time.Minute,
				Transport: proxyTransport,
			})
		}
	}

	return clients
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

var destLocks sync.Map // map[string]*sync.Mutex

func lockDest(path string) func() {
	clean := filepath.Clean(path)
	abs, err := filepath.Abs(clean)
	if err != nil {
		abs = clean
	}
	abs = strings.ToLower(abs)

	v, _ := destLocks.LoadOrStore(abs, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()

	return func() {
		mu.Unlock()
	}
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

func isTransientDownloadError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "eof") ||
		strings.Contains(s, "timeout") ||
		strings.Contains(s, "tls handshake timeout") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "connection refused") ||
		strings.Contains(s, "unexpected eof") ||
		strings.Contains(s, "use of closed network connection")
}

func sanitizeDownloadError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	// 删除超长 GitHub signed asset URL
	re := regexp.MustCompile(`https://release-assets\.githubusercontent\.com/[^\s"]+`)
	msg = re.ReplaceAllString(msg, "GitHub Release 资产下载地址")
	// 删除普通长 URL 的 query
	re2 := regexp.MustCompile(`https?://[^\s"]+`)
	msg = re2.ReplaceAllStringFunc(msg, func(u string) string {
		if parsed, e := url.Parse(u); e == nil {
			parsed.RawQuery = ""
			return parsed.String()
		}
		return u
	})
	return fmt.Errorf("%s", msg)
}

func DownloadAtomic(ctx context.Context, opt Options) error {
	if strings.TrimSpace(opt.DestPath) == "" {
		return fmt.Errorf("DestPath 为空")
	}

	unlock := lockDest(opt.DestPath)
	defer unlock()

	clients := createClients(opt)
	ua := opt.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GoclashZ/1.0"
	}

	candidates, err := probeDownloadCandidates(ctx, clients, opt)
	if err != nil {
		return sanitizeDownloadError(err)
	}

	var lastErr error
	tmpPath := opt.DestPath + ".tmp"
	metaFile := metaPath(tmpPath)

	for _, candidate := range candidates {
		// 每次更换候选环境前，根据情况清理或尝试续传
		for attempt := 1; attempt <= 2; attempt++ {
			err := downloadWithCandidate(ctx, opt, candidate, ua, tmpPath, metaFile)
			if err == nil {
				return nil
			}

			lastErr = err
			if !isTransientDownloadError(err) {
				break // 非网络瞬断错误，不在此候选环境下重试
			}
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}

		// 如果此候选环境失败且是瞬态错误，清理掉可能产生的坏文件，尝试下一个候选
		if isTransientDownloadError(lastErr) {
			_ = os.Remove(tmpPath)
			_ = os.Remove(metaFile)
		}
	}

	return sanitizeDownloadError(lastErr)
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

func ReplaceFile(tmpPath, destPath string) error {
	backupPath := destPath + ".bak"
	_ = os.Remove(backupPath)

	if _, err := os.Stat(destPath); err == nil {
		if err := os.Rename(destPath, backupPath); err != nil {
			return fmt.Errorf("备份失败: %w", err)
		}
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Rename(backupPath, destPath)
		return fmt.Errorf("替换失败: %w", err)
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
		return fmt.Errorf("SHA256 不匹配")
	}
	return nil
}

func FetchSmallFileAtomic(ctx context.Context, opt Options) error {
	// 保持原样或按需重构，这里为了简洁先保留基本逻辑
	if strings.TrimSpace(opt.DestPath) == "" {
		return fmt.Errorf("DestPath 为空")
	}
	unlock := lockDest(opt.DestPath)
	defer unlock()

	clients := createClients(opt)
	urls := opt.URLs
	totalRaces := len(clients) * len(urls)

	type result struct {
		body   []byte
		header http.Header
		err    error
	}
	resultCh := make(chan result, totalRaces)
	raceCtx, cancelRace := context.WithCancel(ctx)
	defer cancelRace()

	ua := opt.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GoclashZ/1.0"
	}

	for _, client := range clients {
		for _, u := range urls {
			go func(c *http.Client, target string) {
				req, err := http.NewRequestWithContext(raceCtx, http.MethodGet, target, nil)
				if err != nil {
					resultCh <- result{err: err}
					return
				}
				req.Header.Set("User-Agent", ua)
				resp, err := c.Do(req)
				if err != nil {
					resultCh <- result{err: err}
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					resultCh <- result{err: fmt.Errorf("HTTP %d", resp.StatusCode)}
					return
				}
				var r io.Reader = resp.Body
				if opt.MaxBytes > 0 {
					r = io.LimitReader(resp.Body, opt.MaxBytes+1)
				}
				body, _ := io.ReadAll(r)
				resultCh <- result{body: body, header: resp.Header, err: nil}
			}(client, u)
		}
	}

	var lastErr error
	for i := 0; i < totalRaces; i++ {
		res := <-resultCh
		if res.err == nil && len(res.body) > 0 {
			tmpPath := opt.DestPath + ".tmp"
			_ = os.WriteFile(tmpPath, res.body, 0644)
			if opt.Validator != nil {
				if err := opt.Validator(tmpPath); err != nil {
					_ = os.Remove(tmpPath)
					lastErr = err
					continue
				}
			}
			cancelRace()
			if opt.OnResponse != nil {
				opt.OnResponse(&http.Response{Header: res.header})
			}
			return ReplaceFile(tmpPath, opt.DestPath)
		}
		lastErr = res.err
	}
	return sanitizeDownloadError(lastErr)
}
