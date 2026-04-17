package clash

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// UpdateSubscription 下载 YAML 订阅并覆盖本地 config.yaml
func UpdateSubscription(subURL string) error {
	pwd, err := os.Getwd()
	if err != nil {
		return err
	}
	configPath := filepath.Join(pwd, "core", "bin", "config.yaml")

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", subURL, nil)
	if err != nil {
		return err
	}
	// 伪装 User-Agent，防止部分机场拦截
	req.Header.Set("User-Agent", "ClashforWindows/0.20.39")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("订阅下载失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("订阅服务器异常: HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(configPath)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		return err
	}

	// 订阅下载完成后，调用内核的 API 无缝热重载配置
	return ReloadConfig()
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
