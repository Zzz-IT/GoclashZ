package traffic

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// 关键修复：创建一个忽略系统代理的专属 HTTP 客户端
var localAPIClient = &http.Client{
	Transport: &http.Transport{
		Proxy: nil, // 强制不走代理
	},
	Timeout: 5 * time.Second,
}

// GetTraffic 从内核 API 获取实时的上传下载字节流并格式化
func GetTraffic() (string, string) {
	// 使用 localAPIClient 替换 http.Get
	resp, err := localAPIClient.Get("http://127.0.0.1:9090/traffic")
	if err != nil {
		return "0 B/s", "0 B/s"
	}
	defer resp.Body.Close()

	var data struct {
		Up   int64 `json:"up"`
		Down int64 `json:"down"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "0 B/s", "0 B/s"
	}

	return formatBytes(data.Up), formatBytes(data.Down)
}

// formatBytes 将字节数转换为人类可读的字符串
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B/s", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB/s", float64(b)/float64(div), "KMGTPE"[exp])
}
