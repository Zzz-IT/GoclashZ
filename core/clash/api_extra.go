package clash

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// LogMessage 内核日志结构
type LogMessage struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

// StartLogStream 开启日志监听流
func StartLogStream(ctx context.Context) {
	// 监听内核日志接口 (默认级别 info)
	url := "ws://127.0.0.1:9090/logs?level=info"

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		fmt.Println("日志连接失败:", err)
		return
	}
	defer conn.Close()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var log LogMessage
		json.Unmarshal(message, &log)

		// ✨ 模仿 Stelliberty：实时推送事件给前端
		runtime.EventsEmit(ctx, "clash_log", log)
	}
}

// PatchConfig 修改内核运行特性 (TUN, LAN, IPv6, LogLevel等)
func PatchConfig(settings map[string]interface{}) error {
	payload, _ := json.Marshal(settings)
	req, _ := http.NewRequest("PATCH", "http://127.0.0.1:9090/configs", strings.NewReader(string(payload)))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("内核返回错误码: %d", resp.StatusCode)
	}
	return nil
}
