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

var githubReleaseDownloadRe = regexp.MustCompile(
	`^https://github\.com/([^/]+)/([^/]+)/releases/download/([^/]+)/([^?#]+)`,
)

func ResolveGitHubAssetSHA256(ctx context.Context, client *http.Client, rawURL string, userAgent string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	// 兼容 ghproxy.net/https://github.com/...
	if parsed.Host != "github.com" {
		idx := strings.Index(rawURL, "https://github.com/")
		if idx >= 0 {
			rawURL = rawURL[idx:]
		}
	}

	m := githubReleaseDownloadRe.FindStringSubmatch(rawURL)
	if len(m) != 5 {
		return "", fmt.Errorf("不是 GitHub release asset URL")
	}

	owner := m[1]
	repo := m[2]
	tag := m[3]
	assetName, _ := url.PathUnescape(path.Base(m[4]))

	var apiURL string
	if tag == "latest" {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	} else {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", owner, repo, url.PathEscape(tag))
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
		return "", fmt.Errorf("GitHub release API 返回 HTTP %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	for _, asset := range release.Assets {
		if asset.Name != assetName {
			continue
		}

		digest := strings.TrimSpace(asset.Digest)
		digest = strings.TrimPrefix(digest, "sha256:")
		digest = strings.ToLower(digest)

		if len(digest) == 64 {
			return digest, nil
		}
		return "", fmt.Errorf("asset %s 未提供 sha256 digest", assetName)
	}

	return "", fmt.Errorf("release 中未找到 asset: %s", assetName)
}
