//go:build windows

package clash

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// LogMessage 内核日志结构
type LogMessage struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

// LogCallback 日志回调类型
type LogCallback func(log LogMessage)

// StartLogStream 开启日志监听流（通过回调推送，不再依赖 Wails）
func StartLogStream(ctx context.Context, onLog LogCallback) {
	const (
		pongWait   = 60 * time.Second
		pingPeriod = 30 * time.Second
		writeWait  = 5 * time.Second
	)

	// 使用动态生成的 WebSocket 地址
	wsURL := APIWSURLWithRawQuery("/logs", "level=info")

	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		fmt.Println("日志连接失败:", err)
		return
	}
	defer conn.Close()

	// 🚀 核心：设置读超时并绑定 Pong 处理，用于探测连接活性
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	done := make(chan struct{})

	// 读取循环
	go func() {
		defer close(done)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var log LogMessage
			if err := json.Unmarshal(message, &log); err != nil {
				continue
			}

			// ✨ 通过回调推送日志
			if onLog != nil {
				onLog(log)
			}
		}
	}()

	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	// 控制循环
	for {
		select {
		case <-ctx.Done():
			// 优雅关闭 WebSocket 连接
			_ = conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
				time.Now().Add(writeWait),
			)
			return

		case <-done:
			return

		case <-ticker.C:
			// 🚀 核心：主动发送 Ping 以维持连接并探测对端状态
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
				return
			}
		}
	}
}

// PatchConfig 修改内核运行特性 (TUN, LAN, IPv6, LogLevel等)
func PatchConfig(settings map[string]interface{}) error {
	payload, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("配置序列化失败: %v", err)
	}

	// 使用动态 API 地址
	req, err := http.NewRequest("PATCH", APIURL("/configs"), strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("构建补丁请求失败: %v", err)
	}

	resp, err := localAPIClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("内核返回错误码: %d", resp.StatusCode)
	}
	return nil
}
