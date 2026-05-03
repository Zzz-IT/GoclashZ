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
)

type AppUpdateInfo struct {
	HasUpdate   bool     `json:"hasUpdate"`
	Version     string   `json:"version"`
	Body        string   `json:"body"`
	ReleaseURL  string   `json:"releaseUrl"`
	DownloadURL string   `json:"downloadUrl"`
	AssetName   string   `json:"assetName"`
}

var strictVersionRe = regexp.MustCompile(`(?i)(?:^|[^0-9])v?(\d+\.\d+(?:\.\d+)?(?:\.\d+)?)`)

func CheckAppUpdate(ctx context.Context, currentVersion string) (*AppUpdateInfo, error) {
	apiURL := "https://api.github.com/repos/Zzz-IT/GoclashZ/releases/latest"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	req.Header.Set("User-Agent", "GoclashZ-Updater")

	resp, err := defaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var release struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&release)

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
	for _, asset := range assets {
		lower := strings.ToLower(asset.Name)
		if strings.HasSuffix(lower, ".exe") && strings.Contains(lower, "goclashz") {
			if strings.Contains(lower, "setup") || strings.Contains(lower, "installer") {
				return asset.Name, asset.BrowserDownloadURL
			}
		}
	}
	return "", ""
}

func CompareAppVersion(remote, current string) (int, error) {
	aa := parseVersionParts(remote)
	bb := parseVersionParts(current)
	if len(aa) == 0 || len(bb) == 0 { return 0, nil }
	for i := 0; i < 3; i++ {
		var a, b int
		if i < len(aa) { a = aa[i] }
		if i < len(bb) { b = bb[i] }
		if a > b { return 1, nil }
		if a < b { return -1, nil }
	}
	return 0, nil
}

func parseVersionParts(v string) []int {
	m := strictVersionRe.FindStringSubmatch(v)
	if len(m) < 2 { return nil }
	parts := strings.Split(m[1], ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, _ := strconv.Atoi(p)
		out = append(out, n)
	}
	return out
}

// DownloadAppUpdate 使用通用下载机下载应用更新包
func DownloadAppUpdate(ctx context.Context, info *AppUpdateInfo, destDir string) (string, error) {
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

	// 🚀 复用原子下载机
	err := DownloadAtomic(ctx, Options{
		URLs:      []string{info.DownloadURL},
		DestPath:  destPath,
		UserAgent: "GoclashZ-Updater",
		MaxBytes:  300 << 20, // 限制 300MB
		Resume:    true,
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
