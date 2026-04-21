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
	// 👉 核心修复 1：严格遵循 国内镜像优先 -> GitHub 兜底 的策略
	urlsToTry := []string{
		"https://ghfast.top/" + directURL,
		"https://ghproxy.net/" + directURL,
		directURL,
	}

	os.MkdirAll(filepath.Dir(targetPath), 0755)
	var finalErr error

	for _, url := range urlsToTry {
		// 👉 核心修复 2：精细化超时控制，摒弃全局超时
		client := &http.Client{
			Transport: &http.Transport{
				// 仅限制握手和响应头等待时间。5秒内连不上直接拉闸，换下一个源！
				ResponseHeaderTimeout: 5 * time.Second, 
				IdleConnTimeout:       10 * time.Second,
			},
		}

		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) goclashz")

		resp, err := client.Do(req)
		if err != nil {
			finalErr = err
			continue // 发生错误（如连接超时），光速切换下一个源
		}

		if resp.StatusCode == http.StatusOK {
			out, err := os.Create(targetPath)
			if err != nil {
				resp.Body.Close()
				return err
			}

			// 缓冲流拷贝，控制内存并在断流时抛出 err
			buf := make([]byte, 32*1024)
			_, err = io.CopyBuffer(out, resp.Body, buf)
			
			resp.Body.Close()
			out.Close()

			if err == nil {
				return nil // ✅ 只要完整下载成功一个，直接结束
			}
			finalErr = err // 下载中途断开，记录错误并继续循环
		} else {
			resp.Body.Close()
			finalErr = fmt.Errorf("源返回非200状态码: %d", resp.StatusCode)
		}
	}

	return fmt.Errorf("所有下载源均失败: %v", finalErr)
}
