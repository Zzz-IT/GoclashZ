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

func CheckAppUpdate(ctx context.Context, currentVersion string, strategy func() DownloadStrategy) (*AppUpdateInfo, error) {
	apiURL := "https://api.github.com/repos/Zzz-IT/GoclashZ/releases/latest"

	clients := BuildOrderedClients(strategy, APIRequestTimeout)

	// 单次拨号超时 6s，保证 2 client × 3 URL = 6 次拨号（最坏 36s）
	// 能在外层 40s 的预算内跑完，不会在候选还没试完时就被整体超时掐断。
	apiURLs := BuildCandidateAPIURLs(apiURL)

	var release struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	var lastErr error
found:
	for _, client := range clients {
		for _, u := range apiURLs {
			req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if reqErr != nil {
				lastErr = reqErr
				continue
			}
			req.Header.Set("User-Agent", "GoclashZ-Updater")

			resp, reqErr := client.Do(req)
			if reqErr != nil {
				lastErr = reqErr
				continue
			}

			func() {
				defer resp.Body.Close()
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					lastErr = fmt.Errorf("GitHub API 返回 HTTP %d", resp.StatusCode)
					return
				}
				lastErr = json.NewDecoder(resp.Body).Decode(&release)
			}()

			if lastErr == nil && release.TagName != "" {
				break found
			}
		}
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
// 实测 ghfast / ghproxy 这类公益镜像基本只反代 github.com 的下载路径，
// 不反代 api.github.com（直接 403 或断连），因此原生地址恒优先、镜像仅作最后兜底。
func BuildCandidateAPIURLs(apiURL string) []string {
	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		return nil
	}
	if !strings.Contains(apiURL, "api.github.com/") {
		return []string{apiURL}
	}

	urls := make([]string, 0, len(githubMirrors)+1)
	urls = append(urls, apiURL)
	for _, m := range githubMirrors {
		urls = append(urls, m+apiURL)
	}
	return urls
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
