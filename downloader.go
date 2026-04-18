package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// getLatestMihomoAssetURL 动态获取最新的 GitHub Release Asset URL
func getLatestMihomoAssetURL(platform, arch, fileExt string) (string, error) {
	apiURL := "https://api.github.com/repos/MetaCubeX/mihomo/releases/latest"
	
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", "goclashz-updater")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API 请求失败，状态码: %d", resp.StatusCode)
	}

	var result struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadUrl string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	keyword := fmt.Sprintf("%s-%s", platform, arch)
	for _, asset := range result.Assets {
		if strings.Contains(asset.Name, keyword) && strings.HasSuffix(asset.Name, fileExt) {
			return asset.BrowserDownloadUrl, nil
		}
	}

	return "", fmt.Errorf("未在版本 %s 中找到适配 %s 架构的文件", result.TagName, keyword)
}

// downloadFileWithRetry 带重试机制和多源 fallback 的下载器
func downloadFileWithRetry(targetPath string, directURL string) error {
	urlsToTry := []string{
		directURL,
		"https://ghproxy.net/" + directURL,
		"https://ghfast.top/" + directURL,
	}

	maxRetries := 3 
	os.MkdirAll(filepath.Dir(targetPath), 0755)

	for _, url := range urlsToTry {
		for attempt := 1; attempt <= maxRetries; attempt++ {
			if attempt > 1 {
				time.Sleep(time.Duration(attempt*2) * time.Second) 
			}

			client := &http.Client{
				Timeout: 60 * time.Second,
			}

			req, _ := http.NewRequest("GET", url, nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) goclashz")

			resp, err := client.Do(req)
			if err != nil {
				continue 
			}

			if resp.StatusCode == http.StatusNotFound {
				resp.Body.Close()
				break 
			}

			if resp.StatusCode == http.StatusOK {
				out, err := os.Create(targetPath)
				if err != nil {
					resp.Body.Close()
					return err
				}

				_, err = io.Copy(out, resp.Body)
				resp.Body.Close()
				out.Close()

				if err == nil {
					return nil 
				}
			} else {
				resp.Body.Close()
			}
		}
	}

	return fmt.Errorf("所有下载源与重试均告失败")
}
