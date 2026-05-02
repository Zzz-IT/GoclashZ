package appcore

import (
	"context"
	"goclashz/core/clash"
	"goclashz/core/logger"
	"sync"
	"time"
)

type LogStreamManager struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	gen     int
	running bool
	emit    EventSink
}

func NewLogStreamManager(emit EventSink) *LogStreamManager {
	return &LogStreamManager{
		emit: emit,
	}
}

func (m *LogStreamManager) Start(ctx context.Context, logLevel string) {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel() // 清理旧流
	}

	streamCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.gen++
	currentGen := m.gen
	m.running = true
	m.mu.Unlock()

	go func(gen int) {
		defer func() {
			m.mu.Lock()
			if m.gen == gen {
				m.cancel = nil
				m.running = false
			}
			m.mu.Unlock()
		}()

		clash.FetchLogs(streamCtx, logLevel, func(data interface{}) {
			m.mu.Lock()
			// Generation Guard 帧级防并发污染
			alive := m.gen == gen && m.cancel != nil
			m.mu.Unlock()

			if !alive {
				return
			}

			if mapData, ok := data.(map[string]interface{}); ok {
				typ, _ := mapData["type"].(string)
				payload, _ := mapData["payload"].(string)

				entry := logger.LogEntry{
					Type:    typ,
					Payload: payload,
					Time:    time.Now().Format("15:04:05"),
				}

				// 写入缓冲区并通知前端
				logger.AppLogs.Add(entry)
				if m.emit != nil {
					m.emit.Emit(EventLogMessage, entry)
				}
			}
		})
	}(currentGen)
}

func (m *LogStreamManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
		m.gen++
		m.running = false
	}
}

func (m *LogStreamManager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}
