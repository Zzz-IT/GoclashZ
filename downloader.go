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
func getLatestMihomoAssetURL(platform, arch, fileExt string) (string, string, error) {
	apiURL := "https://api.github.com/repos/MetaCubeX/mihomo/releases/latest"
	
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", "goclashz-updater")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	// ⚠️ 修复：增加对 403 API 速率限制的精准识别
	if resp.StatusCode == http.StatusForbidden {
		remain := resp.Header.Get("X-RateLimit-Remaining")
		if remain == "0" {
			resetTime := resp.Header.Get("X-RateLimit-Reset")
			return "", "", fmt.Errorf("触发 GitHub API 请求频率限制，请尝试切换代理节点或稍后重试（重置时间戳：%s）", resetTime)
		}
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("GitHub API 请求失败，状态码: %d", resp.StatusCode)
	}

	var result struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadUrl string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}

	keyword := fmt.Sprintf("%s-%s", platform, arch)
	for _, asset := range result.Assets {
		if strings.Contains(asset.Name, keyword) && strings.HasSuffix(asset.Name, fileExt) {
			// 👈 修改点：同时返回下载链接和版本号 (result.TagName)
			return asset.BrowserDownloadUrl, result.TagName, nil
		}
	}

	return "", "", fmt.Errorf("未在版本 %s 中找到适配 %s 架构的文件", result.TagName, keyword)
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

			// ⚠️ 修复：优化 HttpClient 配置，避免 60 秒硬超时截断大文件
			client := &http.Client{
				// Timeout: 60 * time.Second, // 删掉全局超时
				Transport: &http.Transport{
					ResponseHeaderTimeout: 15 * time.Second, // 仅对等待服务器响应头设置超时
					IdleConnTimeout:       30 * time.Second,
				},
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

				// ⚠️ 修复：使用 io.CopyBuffer 控制内存并提供更稳定的流式写入
				buf := make([]byte, 32*1024) // 32KB 缓冲
				_, err = io.CopyBuffer(out, resp.Body, buf)
				
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
