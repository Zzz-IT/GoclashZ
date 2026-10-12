//go:build windows

package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AppUpdateInfo struct {
	HasUpdate   bool   `json:"hasUpdate"`
	Version     string `json:"version"`
	Body        string `json:"body"`
	ReleaseURL  string `json:"releaseUrl"`
	DownloadURL string `json:"downloadUrl"`
	AssetName   string `json:"assetName"`
}

var strictVersionRe = regexp.MustCompile(`(?i)(?:^|[^0-9])v?(\d+\.\d+(?:\.\d+)?(?:\.\d+)?)`)

// errStrategyChanged 标记"检查过程中代理/TUN 被切换"，
// 后端将其作为普通结果处理（静默复位前端状态），不弹错误框。
var errStrategyChanged = fmt.Errorf("network strategy changed during check")

// IsStrategyChanged 判断错误是否为"检查过程中网络策略被切换"。
func IsStrategyChanged(err error) bool {
	return err != nil && (err == errStrategyChanged || strings.Contains(err.Error(), errStrategyChanged.Error()))
}

func CheckAppUpdate(ctx context.Context, currentVersion string, strategy func() DownloadStrategy) (*AppUpdateInfo, error) {
	apiURL := "https://api.github.com/repos/Zzz-IT/GoclashZ/releases/latest"

	// 每次调用时读取一次策略作为基线，用于后续轮询检测代理/TUN 是否被切换。
	var baseStrategy DownloadStrategy
	if strategy != nil {
		baseStrategy = strategy()
	}

	clients := BuildOrderedClients(strategy, APIRequestTimeout)
	apiURLs := BuildCandidateAPIURLs(apiURL)

	// 监听代理/TUN 切换：用户在检查过程中切换了网络路径时，
	// 立即中止本次检查（返回带标记的错误），让用户重新点击后
	// 立刻用新路径重试，而不是傻等超时。
	strategyChanged := make(chan struct{}, 1)
	if strategy != nil {
		watchCtx, stopWatch := context.WithCancel(ctx)
		defer stopWatch()
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-watchCtx.Done():
					return
				case <-ticker.C:
					s := strategy()
					if s.PreferProxy != baseStrategy.PreferProxy || s.ProxyURL != baseStrategy.ProxyURL {
						select {
						case strategyChanged <- struct{}{}:
						default:
						}
						return
					}
				}
			}
		}()
	}

	var release struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	// 并发发起所有候选请求（直连/代理 × 原生地址），谁先成功用谁，
	// 其余用 reqCtx 取消。网络不通时最坏也只是一个 client 的 6s 超时，
	// 而不是串行逐个等待（旧逻辑最坏 2 client × 6s = 12s 才能报失败）。
	var lastErr error
	found := false
	reqCtx, cancelReqs := context.WithCancel(ctx)
	defer cancelReqs()

	type checkResult struct {
		rel *struct {
			TagName string `json:"tag_name"`
			Body    string `json:"body"`
			HTMLURL string `json:"html_url"`
			Assets  []struct {
				Name               string `json:"name"`
				BrowserDownloadURL string `json:"browser_download_url"`
			} `json:"assets"`
		}
		err error
	}
	resultCh := make(chan checkResult, len(clients)*len(apiURLs))
	var successOnce sync.Once

	// 标记策略切换：一旦触发，取消所有在途请求
	go func() {
		select {
		case <-strategyChanged:
			cancelReqs()
		case <-reqCtx.Done():
		}
	}()

	for _, client := range clients {
		for _, u := range apiURLs {
			go func(c *http.Client, rawURL string) {
				select {
				case <-strategyChanged:
					resultCh <- checkResult{err: errStrategyChanged}
					return
				default:
				}

				req, reqErr := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
				if reqErr != nil {
					resultCh <- checkResult{err: reqErr}
					return
				}
				req.Header.Set("User-Agent", "GoclashZ-Updater")

				resp, reqErr := c.Do(req)
				if reqErr != nil {
					resultCh <- checkResult{err: reqErr}
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					resultCh <- checkResult{err: fmt.Errorf("GitHub API 返回 HTTP %d", resp.StatusCode)}
					return
				}

				var rel struct {
					TagName string `json:"tag_name"`
					Body    string `json:"body"`
					HTMLURL string `json:"html_url"`
					Assets  []struct {
						Name               string `json:"name"`
						BrowserDownloadURL string `json:"browser_download_url"`
					} `json:"assets"`
				}
				if decErr := json.NewDecoder(resp.Body).Decode(&rel); decErr != nil {
					resultCh <- checkResult{err: decErr}
					return
				}
				if rel.TagName == "" {
					resultCh <- checkResult{err: fmt.Errorf("GitHub API 响应缺少 tag_name")}
					return
				}

				// 用 sync.Once 保证只有第一个成功者写回数据并取消其余请求
				successOnce.Do(func() {
					release = rel
					found = true
					cancelReqs()
				})
				resultCh <- checkResult{rel: &rel, err: nil}
			}(client, u)
		}
	}

	total := len(clients) * len(apiURLs)
	for i := 0; i < total; i++ {
		res := <-resultCh
		if res.err == errStrategyChanged {
			return nil, errStrategyChanged
		}
		if !found {
			lastErr = res.err
		}
	}

	if found {
		lastErr = nil
	}
	if lastErr != nil {
		return nil, lastErr
	}

	cmp, _ := CompareAppVersion(release.TagName, currentVersion)
	assetName, downloadURL := selectWindowsAsset(release.Assets)

	return &AppUpdateInfo{
		HasUpdate:   cmp > 0,
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
}) (string, string) {
	type candidate struct {
		name string
		url  string
		rank int
	}

	var candidates []candidate

	for _, asset := range assets {
		lower := strings.ToLower(asset.Name)

		if !strings.HasSuffix(lower, ".exe") {
			continue
		}

		if !strings.Contains(lower, "goclashz") {
			continue
		}

		if strings.Contains(lower, "sha256") ||
			strings.Contains(lower, "checksum") ||
			strings.Contains(lower, "symbols") ||
			strings.Contains(lower, "debug") {
			continue
		}

		rank := 10
		if strings.Contains(lower, "setup") || strings.Contains(lower, "installer") {
			rank = 0
		} else if strings.Contains(lower, "windows") || strings.Contains(lower, "win") {
			rank = 1
		} else if strings.Contains(lower, "x64") || strings.Contains(lower, "amd64") {
			rank = 2
		}

		candidates = append(candidates, candidate{
			name: asset.Name,
			url:  asset.BrowserDownloadURL,
			rank: rank,
		})
	}

	if len(candidates) == 0 {
		return "", ""
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.rank < best.rank {
			best = c
		}
	}

	return best.name, best.url
}

func CompareAppVersion(remote, current string) (int, error) {
	aa := parseVersionParts(remote)
	bb := parseVersionParts(current)
	if len(aa) == 0 || len(bb) == 0 {
		return 0, nil
	}
	for i := 0; i < 3; i++ {
		var a, b int
		if i < len(aa) {
			a = aa[i]
		}
		if i < len(bb) {
			b = bb[i]
		}
		if a > b {
			return 1, nil
		}
		if a < b {
			return -1, nil
		}
	}
	return 0, nil
}

func parseVersionParts(v string) []int {
	m := strictVersionRe.FindStringSubmatch(v)
	if len(m) < 2 {
		return nil
	}
	parts := strings.Split(m[1], ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, _ := strconv.Atoi(p)
		out = append(out, n)
	}
	return out
}

// githubMirrors 是国内可用的 GitHub 反代镜像（公益服务，可用性波动，仅作加速/兜底）
var githubMirrors = []string{
	"https://ghfast.top/",
	"https://mirror.ghproxy.com/",
}

// APIRequestTimeout 是检查更新时单次 API 请求的超时。
// 取值 6s：候选遍历最坏要跑 2 个 client × 3 个地址 = 6 次，
// 6 × 6s = 36s，仍落在外层 40s 的整体预算内。
const APIRequestTimeout = 6 * time.Second

// BuildCandidateDownloadURLs 为 GitHub Release 资产生成候选下载地址。
// preferProxy 为 true（用户开着代理）时原生地址优先；
// 为 false（国内裸连）时镜像优先、原生地址兜底，避免先撞一次 GitHub 的墙。
func BuildCandidateDownloadURLs(rawURL string, preferProxy bool) []string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil
	}

	isGitHubRelease := strings.Contains(rawURL, "github.com/") && strings.Contains(rawURL, "/releases/download/")
	if !isGitHubRelease {
		return []string{rawURL}
	}

	urls := make([]string, 0, len(githubMirrors)+1)
	if preferProxy {
		urls = append(urls, rawURL)
		for _, m := range githubMirrors {
			urls = append(urls, m+rawURL)
		}
	} else {
		for _, m := range githubMirrors {
			urls = append(urls, m+rawURL)
		}
		urls = append(urls, rawURL)
	}
	return urls
}

// BuildCandidateAPIURLs 为 GitHub API 请求生成候选地址。
// 实测 ghfast / ghproxy 这类公益镜像只反代 github.com 的下载路径，
// 不反代 api.github.com（直接 403 或断连），镜像候选每次都会白等一个超时，
// 因此 API 请求固定只用原生地址，不再拼镜像兜底。
func BuildCandidateAPIURLs(apiURL string) []string {
	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		return nil
	}
	return []string{apiURL}
}

// DownloadAppUpdate 使用通用下载机下载应用更新包
func DownloadAppUpdate(
	ctx context.Context,
	info *AppUpdateInfo,
	destDir string,
	onProgress func(bytesDone, totalBytes, speedBps int64, etaSec int64),
	strategy func() DownloadStrategy,
) (string, error) {
	if info == nil {
		return "", fmt.Errorf("更新信息为空")
	}
	if strings.TrimSpace(info.DownloadURL) == "" {
		return "", fmt.Errorf("没有可用的应用更新下载地址")
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}

	fileName := sanitizeUpdateAssetName(info.AssetName)
	if fileName == "" {
		// 兜底方案
		fileName = fmt.Sprintf("GoclashZ_%s_Setup.exe", strings.TrimPrefix(info.Version, "v"))
	}

	destPath := filepath.Join(destDir, fileName)

	preferProxy := false
	if strategy != nil {
		preferProxy = strategy().PreferProxy
	}

	// 🚀 复用 grab/v3 下载机，支持多通道竞速、镜像自动回退、断点续传及 SHA256 校验
	err := DownloadLargeAssetAtomic(ctx, Options{
		URLs:                BuildCandidateDownloadURLs(info.DownloadURL, preferProxy),
		DestPath:            destPath,
		UserAgent:           "GoclashZ-Updater",
		MaxBytes:            300 << 20, // 限制 300MB
		Strategy:            strategy,
		OnProgress:          onProgress,
		VerifyGitHubSHA:     true,
		RequireGitHubSHA:    false,
		AttemptsPerEndpoint: 3,
		Validator: func(tmpPath string) error {
			return ValidateWindowsExecutable(tmpPath)
		},
	})
	if err != nil {
		return "", err
	}

	return destPath, nil
}

// ValidateWindowsExecutable 验证是否为有效的 Windows 可执行文件
func ValidateWindowsExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	// 最小体积校验 (1MB)
	if info.Size() < 1024*1024 {
		return fmt.Errorf("更新包体积异常 (小于 1MB)")
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// 读取前两个字节检查 "MZ" 头
	header := make([]byte, 2)
	if _, err := io.ReadFull(f, header); err != nil {
		return err
	}

	if string(header) != "MZ" {
		return fmt.Errorf("更新包不是有效的 Windows 可执行文件 (缺少 MZ 标识)")
	}

	return nil
}

func sanitizeUpdateAssetName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == "" {
		return ""
	}
	// 只允许 .exe
	if !strings.HasSuffix(strings.ToLower(name), ".exe") {
		return ""
	}
	return name
}
