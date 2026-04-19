package clash

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UpdateSubscription 下载 YAML 订阅
// 如果 targetName 为空，则自动根据 URL 生成文件名
func UpdateSubscription(subURL string, targetName string) (string, error) {
	pwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// 1. 决定文件名
	fileName := targetName
	if fileName == "" {
		// 简单从 URL 提取或使用时间戳
		fileName = fmt.Sprintf("sub_%d.yaml", time.Now().Unix())
	}
	if !strings.HasSuffix(fileName, ".yaml") && !strings.HasSuffix(fileName, ".yml") {
		fileName += ".yaml"
	}

	configPath := filepath.Join(pwd, "core", "bin", fileName)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", subURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "ClashforWindows/0.20.39")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("订阅下载失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("订阅服务器异常: HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(configPath)
	if err != nil {
		return "", err
	}

	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		return "", err
	}

	// 如果当前正在运行且更新的是活动配置，则热重载
	// 注意：这里为了简单，暂时不在此处判断是否是活动配置，由 App 层决定是否 Reload
	return fileName, nil
}

// ReloadConfig 调用内核 API 热重载
func ReloadConfig() error {
	req, _ := http.NewRequest("PUT", "http://127.0.0.1:9090/configs?force=true", nil)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("内核配置重载失败")
	}
	return nil
}
