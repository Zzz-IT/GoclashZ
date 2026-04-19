package traffic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// StreamTraffic 建立一个长连接并持续监听内核推送的流量数据
func StreamTraffic(ctx context.Context, callback func(up, down string)) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:9090/traffic", nil)
	if err != nil {
		return
	}

	// 流式接口不能有 Timeout，且需禁用代理
	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
	}

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	for {
		var data struct {
			Up   int64 `json:"up"`
			Down int64 `json:"down"`
		}
		
		// Decode 会阻塞直到接收到下一个 JSON 推送块，如果内核关闭流则返回 err
		if err := decoder.Decode(&data); err != nil {
			break 
		}
		
		callback(formatBytes(data.Up), formatBytes(data.Down))
	}
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
