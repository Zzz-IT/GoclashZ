package traffic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// 定义发给前端的视图对象 (VO)
type ConnectionVO struct {
	ID          string   `json:"id"`
	Network     string   `json:"network"`
	Host        string   `json:"host"`
	SourceIP    string   `json:"sourceIP"`
	Rule        string   `json:"rule"`
	Chains      []string `json:"chains"`
	UploadStr   string   `json:"uploadStr"`   // Go计算好的上传
	DownloadStr string   `json:"downloadStr"` // Go计算好的下载
	DurationStr string   `json:"durationStr"` // Go计算好的时长
}

// RawConnection 对应 Clash API 返回的原始连接项
type RawConnection struct {
	ID       string `json:"id"`
	Metadata struct {
		Network  string `json:"network"`
		Host     string `json:"host"`
		SourceIP string `json:"sourceIP"`
	} `json:"metadata"`
	Upload   int64     `json:"upload"`
	Download int64     `json:"download"`
	Start    time.Time `json:"start"`
	Chains   []string  `json:"chains"`
	Rule     string    `json:"rule"`
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

// EmitConnections 处理并向前端推送格式化后的连接数据
func EmitConnections(ctx context.Context, rawConnections []RawConnection) {
	var vos []ConnectionVO
	for _, conn := range rawConnections {
		vos = append(vos, ConnectionVO{
			ID:          conn.ID,
			Network:     conn.Metadata.Network,
			Host:        conn.Metadata.Host,
			SourceIP:    conn.Metadata.SourceIP,
			Rule:        conn.Rule,
			Chains:      conn.Chains,
			UploadStr:   formatBytes(conn.Upload),
			DownloadStr: formatBytes(conn.Download),
			DurationStr: formatDuration(conn.Start),
		})
	}
	// 发送组装好的 VO 数组给前端
	runtime.EventsEmit(ctx, "connections-update", map[string]interface{}{
		"connections": vos,
	})
}
