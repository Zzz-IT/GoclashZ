package traffic

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// TrafficMessage 定义内核返回的网速结构
type TrafficMessage struct {
	Up   uint64 `json:"up"`
	Down uint64 `json:"down"`
}

var (
	isMonitoring bool
	cancelFunc   context.CancelFunc
)

// StartTrafficMonitor 启动网速监听
func StartTrafficMonitor(ctx context.Context) {
	if isMonitoring {
		return
	}

	// 创建可取消的上下文，用于停止监听
	monitorCtx, cancel := context.WithCancel(ctx)
	cancelFunc = cancel
	isMonitoring = true

	go func() {
		defer func() {
			isMonitoring = false
		}()

		// Clash 默认监控地址
		url := "ws://127.0.0.1:9090/traffic"

		for {
			select {
			case <-monitorCtx.Done():
				return
			default:
				// 尝试建立连接
				conn, _, err := websocket.DefaultDialer.Dial(url, nil)
				if err != nil {
					// 如果连接失败（可能内核还没起），等 2 秒重试
					time.Sleep(2 * time.Second)
					continue
				}

				// 读取循环
				for {
					_, message, err := conn.ReadMessage()
					if err != nil {
						conn.Close()
						break // 断开则重新进入 Dial 逻辑
					}

					var traffic TrafficMessage
					if err := json.Unmarshal(message, &traffic); err == nil {
						// ✨ 核心：通过 Wails 事件推送到前端，事件名为 "clash_traffic"
						runtime.EventsEmit(ctx, "clash_traffic", traffic)
					}
				}
				time.Sleep(1 * time.Second)
			}
		}
	}()
}

// StopTrafficMonitor 停止监听
func StopTrafficMonitor() {
	if cancelFunc != nil {
		cancelFunc()
		isMonitoring = false
	}
}
