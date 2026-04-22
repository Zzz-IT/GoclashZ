package traffic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// RawConnection 对应 Clash API 返回的原始连接项
type RawConnection struct {
	ID       string `json:"id"`
	Metadata struct {
		Network         string `json:"network"`
		Type            string `json:"type"`
		SourceIP        string `json:"sourceIP"`
		DestinationIP   string `json:"destinationIP"`
		SourcePort      string `json:"sourcePort"`
		DestinationPort string `json:"destinationPort"`
		Host            string `json:"host"`
	} `json:"metadata"`
	Upload      int64     `json:"upload"`
	Download    int64     `json:"download"`
	Start       time.Time `json:"start"`
	Chains      []string  `json:"chains"`
	Rule        string    `json:"rule"`
	RulePayload string    `json:"rulePayload"`
}

// 视图对象：无损继承 RawConnection 的所有内容
type ConnectionVO struct {
	RawConnection         // 匿名组合，直接继承
	UploadStr   string `json:"uploadStr"`
	DownloadStr string `json:"downloadStr"`
	DurationStr string `json:"durationStr"`
}

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
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// formatDuration 时间差转换
func formatDuration(start time.Time) string {
	d := time.Since(start)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

// ProcessConnections 将原始连接数据转换为带有格式化字符串的视图对象
func ProcessConnections(rawConnections []RawConnection) []ConnectionVO {
	var vos []ConnectionVO
	for _, conn := range rawConnections {
		vos = append(vos, ConnectionVO{
			RawConnection: conn,
			UploadStr:     formatBytes(conn.Upload),
			DownloadStr:   formatBytes(conn.Download),
			DurationStr:   formatDuration(conn.Start),
		})
	}
	return vos
}

// EmitConnections 处理并向前端推送格式化后的连接数据
func EmitConnections(ctx context.Context, rawConnections []RawConnection) {
	vos := ProcessConnections(rawConnections)
	// 发送组装好的 VO 数组给前端
	runtime.EventsEmit(ctx, "connections-update", map[string]interface{}{
		"connections": vos,
	})
}
