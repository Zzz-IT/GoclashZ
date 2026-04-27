package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
)

type githubReleaseAsset struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

type githubRelease struct {
	Assets []githubReleaseAsset `json:"assets"`
}

var githubReleaseAssetRe = regexp.MustCompile(
	`^https://github\.com/([^/]+)/([^/]+)/releases/download/([^/]+)/([^?#]+)`,
)

// githubDigestCache 按 owner/repo/tag 缓存整个 release 的所有 asset digest，
// 一键更新多个同 release 文件时只请求一次 GitHub API。
var githubDigestCache = struct {
	sync.Mutex
	data map[string]map[string]string
}{
	data: make(map[string]map[string]string),
}

func normalizeGitHubAssetURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)

	// 兼容 ghproxy:
	// https://ghproxy.net/https://github.com/owner/repo/releases/download/tag/file.zip
	if idx := strings.Index(rawURL, "https://github.com/"); idx >= 0 {
		return rawURL[idx:]
	}

	return rawURL
}

func ShouldVerifyGitHubSHA(rawURL string) bool {
	rawURL = normalizeGitHubAssetURL(rawURL)
	return githubReleaseAssetRe.MatchString(rawURL)
}

func ResolveGitHubAssetSHA256(ctx context.Context, client *http.Client, rawURL string, userAgent string) (string, error) {
	rawURL = normalizeGitHubAssetURL(rawURL)

	m := githubReleaseAssetRe.FindStringSubmatch(rawURL)
	if len(m) != 5 {
		return "", fmt.Errorf("不是 GitHub release asset URL: %s", rawURL)
	}

	owner := m[1]
	repo := m[2]
	tag := m[3]

	assetName, err := url.PathUnescape(path.Base(m[4]))
	if err != nil {
		return "", err
	}

	cacheKey := owner + "/" + repo + "/" + tag

	// 先查缓存
	githubDigestCache.Lock()
	if assets, ok := githubDigestCache.data[cacheKey]; ok {
		if digest := assets[assetName]; digest != "" {
			githubDigestCache.Unlock()
			return digest, nil
		}
	}
	githubDigestCache.Unlock()

	// 缓存未命中，请求 GitHub API 获取整个 release 的 digest map
	assets, err := fetchGitHubReleaseDigests(ctx, client, owner, repo, tag, userAgent)
	if err != nil {
		return "", err
	}

	githubDigestCache.Lock()
	githubDigestCache.data[cacheKey] = assets
	digest := assets[assetName]
	githubDigestCache.Unlock()

	if digest == "" {
		return "", fmt.Errorf("GitHub release asset %s 未提供 digest，无法执行 SHA256 校验", assetName)
	}

	return digest, nil
}

func fetchGitHubReleaseDigests(ctx context.Context, client *http.Client, owner, repo, tag, userAgent string) (map[string]string, error) {
	var apiURL string
	if tag == "latest" {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	} else {
		apiURL = fmt.Sprintf(
			"https://api.github.com/repos/%s/%s/releases/tags/%s",
			owner,
			repo,
			url.PathEscape(tag),
		)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	if userAgent == "" {
		userAgent = "GoclashZ/1.0"
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 返回 HTTP %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, asset := range release.Assets {
		digest := strings.TrimSpace(strings.ToLower(asset.Digest))
		digest = strings.TrimPrefix(digest, "sha256:")

		if len(digest) == 64 {
			result[asset.Name] = digest
		}
	}

	return result, nil
}
