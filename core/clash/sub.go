package clash

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv" // 👈 新增导入
	"strings"
	"time"

	"goclashz/core/utils" // 引入全局路径包
)

// SubInfo 👈 新增结构体
type SubInfo struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	Total    int64 `json:"total"`
	Expire   int64 `json:"expire"`
}

// ParseSubInfo 👈 新增解析函数
func ParseSubInfo(header string) *SubInfo {
	if header == "" {
		return nil
	}
	info := &SubInfo{}
	parts := strings.Split(header, ";")
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 {
			val, _ := strconv.ParseInt(kv[1], 10, 64)
			switch strings.ToLower(kv[0]) {
			case "upload":
				info.Upload = val
			case "download":
				info.Download = val
			case "total":
				info.Total = val
			case "expire":
				info.Expire = val
			}
		}
	}
	return info
}

// UpdateSubscription 下载 YAML 订阅
// 如果 targetName 为空，则自动根据 URL 生成文件名
func UpdateSubscription(subURL string, targetName string, userAgent string) (string, *SubInfo, error) {
	// 1. 决定文件名
	fileName := targetName
	if fileName == "" {
		// 简单从 URL 提取或使用时间戳
		fileName = fmt.Sprintf("sub_%d.yaml", time.Now().Unix())
	}
	if !strings.HasSuffix(fileName, ".yaml") && !strings.HasSuffix(fileName, ".yml") {
		fileName += ".yaml"
	}

	configPath := filepath.Join(utils.GetProfilesDir(), fileName)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", subURL, nil)
	if err != nil {
		return "", nil, err
	}

	// 使用传入的 UA，如果没有则使用默认值
	if userAgent == "" {
		userAgent = "ClashforWindows/0.20.39"
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("订阅下载失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("订阅服务器异常: HTTP %d", resp.StatusCode)
	}

	// 👈 核心拦截：在这里抓取流量 Header
	infoStr := resp.Header.Get("Subscription-Userinfo")
	info := ParseSubInfo(infoStr)

	out, err := os.Create(configPath)
	if err != nil {
		return "", nil, fmt.Errorf("无法创建配置文件(权限不足或目录不存在): %v", err)
	}

	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		os.Remove(configPath) // 写入失败时清理残缺文件
		return "", nil, err
	}

	// 如果当前正在运行且更新的是活动配置，则热重载
	// 注意：这里为了简单，暂时不在此处判断是否是活动配置，由 App 层决定是否 Reload
	return fileName, info, nil
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
