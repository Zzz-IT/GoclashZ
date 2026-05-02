package traffic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// ConnectionCallback 连接数据回调类型
type ConnectionCallback func(vos []ConnectionVO)

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

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

// 🚀 核心修复：创建独立的全局流量长连接客户端，并增加 TCP 探活
var trafficStreamClient = &http.Client{
	Transport: &http.Transport{
		Proxy: nil,
		// 👇 核心修复：为流量监听器加上底层心跳检测
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 15 * time.Second,
		}).DialContext,
	},
}

// StreamTraffic 建立一个长连接并持续监听内核推送的流量数据
func StreamTraffic(ctx context.Context, apiURL string, callback func(up, down string), onError func(error)) {
	// 🚀 核心修复：包裹重连状态机
	for {
		// 1. 检查应用是否已退出
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 使用传入的动态 API 地址
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			if !sleepOrDone(ctx, 2*time.Second) {
				return
			}
			continue
		}

		resp, err := trafficStreamClient.Do(req)
		if err != nil {
			if onError != nil && ctx.Err() == nil {
				onError(fmt.Errorf("traffic stream request failed: %w", err))
			}
			if !sleepOrDone(ctx, 2*time.Second) {
				return
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			err := fmt.Errorf("traffic stream unexpected status: %s", resp.Status)
			resp.Body.Close()
			if onError != nil && ctx.Err() == nil {
				onError(err)
			}
			if !sleepOrDone(ctx, 2*time.Second) {
				return
			}
			continue
		}

		const trafficIdleTimeout = 70 * time.Second
		decoder := json.NewDecoder(resp.Body)
		idleTimer := time.NewTimer(trafficIdleTimeout)
		stopIdle := make(chan struct{})

		// 🚀 新增：Idle Watchdog 协程
		go func() {
			select {
			case <-ctx.Done():
				resp.Body.Close()
			case <-idleTimer.C:
				// 超过 70 秒没有收到任何流量推送，主动断开以触发重连
				resp.Body.Close()
			case <-stopIdle:
				// 循环结束，停止监控
			}
		}()

		// 2. 内层循环：正常读取数据流
		for {
			var data struct {
				Up   float64 `json:"up"`
				Down float64 `json:"down"`
			}
			
			if err := decoder.Decode(&data); err != nil {
				// 🚀 核心逻辑：一旦内核重启导致连接 EOF 断开或触发 Watchdog，立刻关闭 Body，跳出内层循环，触发重新连接
				close(stopIdle)
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				resp.Body.Close()

				if ctx.Err() == nil && !errors.Is(err, io.EOF) && onError != nil {
					onError(fmt.Errorf("traffic stream decode failed: %w", err))
				}

				break
			}

			// 收到数据，重置探活计时器
			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C:
				default:
				}
			}
			idleTimer.Reset(trafficIdleTimeout)

			callback(formatBytes(int64(data.Up)), formatBytes(int64(data.Down)))
		}
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

// EmitConnections 处理连接数据（通过回调推送，不再依赖 Wails）
func EmitConnections(ctx context.Context, rawConnections []RawConnection, callback ConnectionCallback) {
	vos := ProcessConnections(rawConnections)
	if callback != nil {
		callback(vos)
	}
}
