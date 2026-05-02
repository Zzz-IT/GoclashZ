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
	"strconv"
	"strings"
	"time"
)

type Options struct {
	URLs            []string // 🚀 1. 竞速容灾：传入多个下载地址（含镜像），第一个通常为主站
	DestPath        string
	UserAgent       string
	MaxBytes        int64
	Client          *http.Client
	Resume          bool
	VerifyGitHubSHA bool

	ProxyURL           string                     // 🚀 2. 自代理加速：指定本地 Clash 代理地址
	InsecureSkipVerify bool                       // 🚀 3. SSL宽容：无视野鸡机场的过期证书
	OnResponse         func(resp *http.Response)  // 拦截器：供外部提取 subscription-userinfo 头
	Validator          func(tmpPath string) error // 🚀 4. 防损屏障：替换前执行验证逻辑 (杜绝坏文件)
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

// createClients 生成网络环境矩阵（直连 vs 代理）
func createClients(opt Options) []*http.Client {
	if opt.Client != nil {
		return []*http.Client{opt.Client}
	}

	var clients []*http.Client
	tlsConfig := &tls.Config{InsecureSkipVerify: opt.InsecureSkipVerify}

	// 🚀 [引擎 A]：直连客户端 (System Direct)
	directTransport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment, // 走系统默认
		TLSClientConfig:     tlsConfig,
		TLSHandshakeTimeout: 5 * time.Second,
		// 🌟 核心修复：彻底禁用 Keep-Alive，防止在“阅后即焚”的竞速场景下产生大量的幽灵协程泄露
		DisableKeepAlives: true,
	}
	clients = append(clients, &http.Client{
		Timeout:   10 * time.Minute,
		Transport: directTransport,
	})

	// 🚀 [引擎 B]：自代理客户端 (Self-Proxy)
	if opt.ProxyURL != "" {
		if pURL, err := url.Parse(opt.ProxyURL); err == nil {
			proxyTransport := &http.Transport{
				Proxy:               http.ProxyURL(pURL), // 强行走本地 Clash 端口
				TLSClientConfig:     tlsConfig,
				TLSHandshakeTimeout: 5 * time.Second,
				// 🌟 核心修复：同样彻底禁用 Keep-Alive
				DisableKeepAlives: true,
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
		return resumeMeta{}, false, err // ✅ 修复：向上抛出真实网络报错
	}

	// ✅ 修复：很多机场面板屏蔽 HEAD 请求返回 405/403，触发 GET 降级探测
	if resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		req, _ = http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
		req.Header.Set("User-Agent", ua)
		// 使用 Range 限制只请求一点点数据，避免测速时就下载了整个订阅
		req.Header.Set("Range", "bytes=0-0")
		resp, err = client.Do(req)
		if err != nil {
			return resumeMeta{}, false, err
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resumeMeta{}, false, fmt.Errorf("HTTP请求失败，状态码: %d", resp.StatusCode) // ✅ 修复：抛出具体错误
	}

	total := resp.ContentLength
	acceptRanges := strings.Contains(strings.ToLower(resp.Header.Get("Accept-Ranges")), "bytes")

	return resumeMeta{
		URL:          resp.Request.URL.String(), // 记录实际起效的重定向真实地址
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		TotalSize:    total,
	}, acceptRanges && total > 0, nil
}

type envProbeResult struct {
	meta       resumeMeta
	canPartial bool
	client     *http.Client // 🌟 核心：记录是哪个客户端（环境）赢得了竞速
	err        error
}

// probeFastestEnvironment 并发竞速：寻找最快的链接与最通畅的网络环境
func probeFastestEnvironment(ctx context.Context, clients []*http.Client, opt Options) (resumeMeta, bool, *http.Client, error) {
	urls := opt.URLs

	totalRaces := len(clients) * len(urls)
	resultCh := make(chan envProbeResult, totalRaces)

	raceCtx, cancelRace := context.WithCancel(ctx)
	defer cancelRace()

	// 开启 N x M 协程矩阵 (环境 x 镜像)
	for _, client := range clients {
		for _, u := range urls {
			go func(c *http.Client, target string) {
				meta, partial, err := probeSingleRemote(raceCtx, c, target, opt)
				if err == nil { // ✅ 移除 TotalSize > 0 的强制依赖
					resultCh <- envProbeResult{meta, partial, c, nil}
				} else {
					resultCh <- envProbeResult{resumeMeta{}, false, nil, err}
				}
			}(client, u)
		}
	}

	var lastErr error
	for i := 0; i < totalRaces; i++ {
		res := <-resultCh
		if res.err == nil { // ✅ 移除 TotalSize > 0 的强制依赖
			cancelRace() // 🌟 找到最快的通道，立刻切断其他落后协程
			return res.meta, res.canPartial, res.client, nil
		}
		lastErr = res.err
	}

	return resumeMeta{}, false, nil, fmt.Errorf("所有网络环境与镜像均探测失败: %v", lastErr)
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

// FetchSmallFileAtomic 🚀 高级并发下载：针对小文件（如订阅、配置）执行“全速体感竞速”
// 它不仅并发测试连接，还直接并发下载内容。第一个通过 Validator 校验的连接将赢得比赛。
func FetchSmallFileAtomic(ctx context.Context, opt Options) error {
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

				var reader io.Reader = resp.Body
				if opt.MaxBytes > 0 {
					reader = io.LimitReader(resp.Body, opt.MaxBytes+1)
				}

				body, err := io.ReadAll(reader)
				if err != nil {
					resultCh <- result{err: err}
					return
				}

				if opt.MaxBytes > 0 && int64(len(body)) > opt.MaxBytes {
					resultCh <- result{err: fmt.Errorf("文件超过大小限制")}
					return
				}

				resultCh <- result{body: body, header: resp.Header, err: nil}
			}(client, u)
		}
	}

	var lastErr error
	for i := 0; i < totalRaces; i++ {
		res := <-resultCh
		if res.err == nil && len(res.body) > 0 {
			// 1. 写入临时文件，准备“验毒”
			tmpPath := opt.DestPath + ".tmp"
			if err := os.WriteFile(tmpPath, res.body, 0644); err != nil {
				lastErr = err
				continue
			}

			// 🌟 2. 核心修复：先先验毒，后颁奖！
			// 如果校验不通过，说明该通道可能被 WAF 拦截返回了 HTML，直接丢弃并继续等待其他慢速但正确的通道
			if opt.Validator != nil {
				if err := opt.Validator(tmpPath); err != nil {
					_ = os.Remove(tmpPath)
					lastErr = fmt.Errorf("探测到无效数据(可能被拦截): %v", err)
					continue
				}
			}

			// 🌟 3. 校验通过，它是真的！
			cancelRace() // 此时才斩断其他还在跑的协程

			if opt.OnResponse != nil {
				opt.OnResponse(&http.Response{Header: res.header})
			}

			return ReplaceFile(tmpPath, opt.DestPath)
		}
		if res.err != nil {
			lastErr = res.err
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("所有竞速通道均已失效")
	}
	return fmt.Errorf("竞速下载失败: %v", lastErr)
}

func DownloadAtomic(ctx context.Context, opt Options) error {
	// 1. 生成竞速矩阵 (直连 & 代理)
	clients := createClients(opt)

	ua := opt.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GoclashZ/1.0"
	}

	tmpPath := opt.DestPath + ".tmp"
	metaFile := metaPath(tmpPath)

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
	} else {
		_ = os.Remove(tmpPath)
		_ = os.Remove(metaFile)
	}

	// 2. 发起极限竞速 (环境 x 镜像) 🚀
	remoteMeta, serverSupportsRange, winningClient, err := probeFastestEnvironment(ctx, clients, opt)
	if err != nil {
		return err
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

		// 🌟 3. 使用赢得竞速的 client 发起正式的大文件下载
		resp, err := winningClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		// 🚀 在 resp 获取成功后，立刻暴露出 Header 供外部解析
		if opt.OnResponse != nil {
			opt.OnResponse(resp)
		}

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
		// 校验也需要使用获胜的 client，确保网络通畅
		// 注意：ResolveGitHubAssetSHA256 内部会处理镜像 URL 的归一化
		expectedSHA, err := ResolveGitHubAssetSHA256(ctx, winningClient, remoteMeta.URL, ua)
		if err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("获取 GitHub 文件哈希失败: %v", err)
		}

		if err := VerifySHA256(tmpPath, expectedSHA); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("文件 SHA256 校验失败: %v", err)
		}
	}

	// 🚀 在 tmp 文件写入完毕，马上要执行 ReplaceFile 之前：
	if opt.Validator != nil {
		if err := opt.Validator(tmpPath); err != nil {
			_ = os.Remove(tmpPath) // 校验失败毁尸灭迹，保护系统配置安全
			return fmt.Errorf("安全校验拦截, 文件存在损坏或格式错误: %v", err)
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
type AppUpdateInfo struct {
	HasUpdate   bool   `json:"hasUpdate"`
	Version     string `json:"version"`
	Body        string `json:"body"`
	ReleaseURL  string `json:"releaseUrl"`
	DownloadURL string `json:"downloadUrl"`
	AssetName   string `json:"assetName"`
}

func CheckAppUpdate(ctx context.Context, currentVersion string) (*AppUpdateInfo, error) {
	// 请求 GitHub API 获取最新 Release
	apiURL := "https://api.github.com/repos/Zzz-IT/GoclashZ/releases/latest"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	req.Header.Set("User-Agent", "GoclashZ-Updater")

	resp, err := defaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 返回 HTTP %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	hasUpdate := compareVersion(release.TagName, currentVersion) > 0
	assetName, downloadURL := selectWindowsAsset(release.Assets)

	return &AppUpdateInfo{
		HasUpdate:   hasUpdate,
		Version:     release.TagName,
		Body:        release.Body,
		ReleaseURL:  release.HTMLURL,
		DownloadURL: downloadURL,
		AssetName:   assetName,
	}, nil
}

func selectWindowsAsset(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}) (string, string) {
	var fallbackName, fallbackURL string

	for _, asset := range assets {
		name := strings.ToLower(asset.Name)

		if !strings.HasSuffix(name, ".exe") {
			continue
		}

		// 优先匹配包含 windows/win 且带有 setup 或 amd64/x64 的包
		if strings.Contains(name, "windows") ||
			strings.Contains(name, "win") ||
			strings.Contains(name, "setup") ||
			strings.Contains(name, "goclashz") {
			if strings.Contains(name, "amd64") ||
				strings.Contains(name, "x64") ||
				strings.Contains(name, "setup") {
				return asset.Name, asset.BrowserDownloadURL
			}
		}

		if fallbackURL == "" {
			fallbackName = asset.Name
			fallbackURL = asset.BrowserDownloadURL
		}
	}

	return fallbackName, fallbackURL
}

func compareVersion(a, b string) int {
	aa := parseVersionParts(a)
	bb := parseVersionParts(b)

	maxLen := len(aa)
	if len(bb) > maxLen {
		maxLen = len(bb)
	}

	// 补齐长度
	for len(aa) < maxLen {
		aa = append(aa, 0)
	}
	for len(bb) < maxLen {
		bb = append(bb, 0)
	}

	for i := 0; i < maxLen; i++ {
		if aa[i] > bb[i] {
			return 1
		}
		if aa[i] < bb[i] {
			return -1
		}
	}
	return 0
}

func parseVersionParts(v string) []int {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimPrefix(v, "v")

	// 忽略预发布标签 (如 -alpha, -beta)
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		v = v[:idx]
	}

	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))

	for _, p := range parts {
		n, _ := strconv.Atoi(p)
		out = append(out, n)
	}

	return out
}
