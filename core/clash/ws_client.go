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
	wsMutex  sync.Mutex
	wsCancel context.CancelFunc
	isActive bool // 记录前端是否处于“连接”页面
)

// StartConnectionMonitor 启动 WebSocket 监听（带无限自动重连机制）
func StartConnectionMonitor(ctx context.Context) error {
	wsMutex.Lock()
	if isActive {
		wsMutex.Unlock()
		return nil // 已经在运行，防止重复开启
	}
	isActive = true
	var wsCtx context.Context
	wsCtx, wsCancel = context.WithCancel(context.Background())
	wsMutex.Unlock()

	// 开启后台守护协程
	go func() {
		dialer := websocket.Dialer{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return nil, nil // 核心：强制绕过本地系统代理劫持
			},
			HandshakeTimeout: 5 * time.Second,
		}
		wsURL := "ws://127.0.0.1:9090/connections"

		for {
			// 1. 检查前端是否已经离开了页面
			select {
			case <-wsCtx.Done():
				fmt.Println("[WebSocket] 前端页面关闭，停止监听连接")
				return
			default:
			}

			// 2. 尝试连接内核（如果没开代理，这里会报错，然后等2秒无限重试）
			conn, _, err := dialer.Dial(wsURL, nil)
			if err != nil {
				fmt.Println("[WebSocket] 内核未启动或拒绝连接，2秒后自动重试...")
				time.Sleep(2 * time.Second)
				continue
			}

			fmt.Println("[WebSocket] ✅ 成功连接到内核！正在接收实时流量流...")

			// 3. 成功连接后，死循环接收数据
			for {
				_, message, err := conn.ReadMessage()
				if err != nil {
					fmt.Println("[WebSocket] ❌ 连接意外断开(内核可能重启)，准备重连:", err)
					conn.Close()
					break // 跳出内部循环，进入外部的重连循环
				}

				// 解析 JSON 并推送到前端
				var data map[string]interface{}
				if err := json.Unmarshal(message, &data); err == nil {
					runtime.EventsEmit(ctx, "connections-update", data)
				}

				// 每次处理完消息，也检查一下前端是否退出了页面
				select {
				case <-wsCtx.Done():
					conn.Close()
					fmt.Println("[WebSocket] 前端页面关闭，断开内核 WebSocket")
					return
				default:
				}
			}

			// 避免断开后疯狂重试吃满 CPU
			time.Sleep(1 * time.Second)
		}
	}()

	return nil
}

// StopConnectionMonitor 停止监听
func StopConnectionMonitor() {
	wsMutex.Lock()
	defer wsMutex.Unlock()
	if wsCancel != nil {
		wsCancel()
		wsCancel = nil
	}
	isActive = false
}
