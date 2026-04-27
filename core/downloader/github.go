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
		return "", err
	}

	if userAgent == "" {
		userAgent = "GoclashZ/1.0"
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API 返回 HTTP %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	for _, asset := range release.Assets {
		if asset.Name != assetName {
			continue
		}

		digest := strings.TrimSpace(strings.ToLower(asset.Digest))
		digest = strings.TrimPrefix(digest, "sha256:")

		if len(digest) != 64 {
			return "", fmt.Errorf("GitHub asset 未提供有效 sha256 digest: %s", assetName)
		}

		return digest, nil
	}

	return "", fmt.Errorf("GitHub release 中未找到文件: %s", assetName)
}
