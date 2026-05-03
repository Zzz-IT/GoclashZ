//go:build windows

package clash

import (
	"context"
	"encoding/json"
	"goclashz/core/traffic"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// ConnectionDataCallback 连接数据回调类型
type ConnectionDataCallback func(vos []traffic.ConnectionVO)

var (
	connMutex    sync.Mutex
	connCancel   context.CancelFunc
	connActive   atomic.Bool
	connCallback ConnectionDataCallback
)

// SetConnectionCallback 设置连接数据的回调（由 app 层在启动时注册）
func SetConnectionCallback(cb ConnectionDataCallback) {
	connMutex.Lock()
	defer connMutex.Unlock()
	connCallback = cb
}

// StartConnectionMonitor 启动连接监控（REST 轮询 + 回调推送）
func StartConnectionMonitor(ctx context.Context) error {
	if !connActive.CompareAndSwap(false, true) {
		return nil
	}

	connMutex.Lock()
	var pollCtx context.Context
	pollCtx, connCancel = context.WithCancel(ctx)
	cb := connCallback
	connMutex.Unlock()

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-pollCtx.Done():
				return
			case <-ticker.C:
				req, err := http.NewRequestWithContext(pollCtx, http.MethodGet, APIURL("/connections"), nil)
				if err != nil {
					continue
				}

				resp, err := localAPIClient.Do(req)
				if err != nil {
					continue
				}

				if resp.StatusCode != http.StatusOK {
					resp.Body.Close()
					continue
				}

				var data struct {
					Connections []traffic.RawConnection `json:"connections"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
					resp.Body.Close()
					continue
				}
				resp.Body.Close()

				if cb != nil {
					vos := traffic.ProcessConnections(data.Connections)
					cb(vos)
				}
			}
		}
	}()

	return nil
}

// StopConnectionMonitor 停止连接监控
func StopConnectionMonitor() {
	if connActive.CompareAndSwap(true, false) {
		connMutex.Lock()
		defer connMutex.Unlock()
		if connCancel != nil {
			connCancel()
			connCancel = nil
		}
	}
}
