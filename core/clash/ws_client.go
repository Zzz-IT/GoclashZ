package clash

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	wsConn   *websocket.Conn
	wsMutex  sync.Mutex
	wsCancel context.CancelFunc
)

// StartConnectionMonitor 启动 WebSocket 监听 Clash 实时连接
func StartConnectionMonitor(ctx context.Context) error {
	wsMutex.Lock()
	defer wsMutex.Unlock()

	// 如果已经有正在运行的连接，不要重复开启
	if wsConn != nil {
		return nil
	}

	var wsCtx context.Context
	wsCtx, wsCancel = context.WithCancel(context.Background())

	// 关键配置：强制禁用代理，彻底防止被本地系统代理劫持
	dialer := websocket.Dialer{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return nil, nil
		},
		HandshakeTimeout: 5 * time.Second,
	}

	// 连接到内核的 WebSocket 接口
	wsURL := "ws://127.0.0.1:9090/connections"
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("WebSocket 连接内核失败: %w", err)
	}

	wsConn = conn

	// 开启后台协程，死循环死守推送消息
	go func() {
		defer StopConnectionMonitor() // 发生错误退出时自动清理

		for {
			select {
			case <-wsCtx.Done():
				return // 收到停止信号，退出协程
			default:
				_, message, err := conn.ReadMessage()
				if err != nil {
					// 内核重启或连接异常断开
					fmt.Println("[WebSocket] 连接断开:", err)
					return
				}

				// 解析 JSON 并推送到前端的 "connections-update" 事件
				var data map[string]interface{}
				if err := json.Unmarshal(message, &data); err == nil {
					runtime.EventsEmit(ctx, "connections-update", data)
				}
			}
		}
	}()

	return nil
}

// StopConnectionMonitor 停止监听并断开长连接
func StopConnectionMonitor() {
	wsMutex.Lock()
	defer wsMutex.Unlock()

	if wsCancel != nil {
		wsCancel()
		wsCancel = nil
	}
	if wsConn != nil {
		wsConn.Close()
		wsConn = nil
	}
}
