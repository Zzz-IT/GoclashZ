package appcore

import (
	"context"
	"goclashz/core/logger"
	"goclashz/core/traffic"
	"strings"
	"sync"
	"time"
)

type TrafficStreamManager struct {
	mu          sync.Mutex
	cancel      context.CancelFunc
	gen         int
	emit        EventSink
	getLogLevel func() string

	lastErrAt  time.Time
	lastErrMsg string
}

func NewTrafficStreamManager(emit EventSink, getLogLevel func() string) *TrafficStreamManager {
	return &TrafficStreamManager{
		emit:        emit,
		getLogLevel: getLogLevel,
	}
}

func (m *TrafficStreamManager) Start(parent context.Context, apiURL string) {
	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return
	}

	m.gen++
	currentGen := m.gen

	ctx, cancel := context.WithCancel(parent)
	m.cancel = cancel
	m.mu.Unlock()

	go func(currentGen int) {
		defer func() {
			m.mu.Lock()
			if m.gen == currentGen {
				m.cancel = nil
				m.emit.Emit("traffic-data", map[string]string{
					"up":   "0 B/s",
					"down": "0 B/s",
				})
			}
			m.mu.Unlock()
		}()

		traffic.StreamTraffic(ctx, apiURL, func(up, down string) {
			m.mu.Lock()
			// 如果 generation 已变，或者取消函数为空，说明当前是个僵尸流
			alive := m.gen == currentGen && m.cancel != nil
			m.mu.Unlock()

			if !alive {
				return // 🚀 核心修复：阻断僵尸数据推送
			}

			m.emit.Emit("traffic-data", map[string]string{
				"up":   up + "/s",
				"down": down + "/s",
			})
		}, func(err error) {
			m.mu.Lock()
			alive := m.gen == currentGen && m.cancel != nil
			m.mu.Unlock()

			if !alive {
				return
			}

			m.emitErrorLog(err)
		})
	}(currentGen)
}

func (m *TrafficStreamManager) Stop() {
	stopped := false
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
		m.gen++
		stopped = true
	}
	m.mu.Unlock()

	if stopped {
		m.emit.Emit("traffic-data", map[string]string{
			"up":   "0 B/s",
			"down": "0 B/s",
		})
	}
}

func (m *TrafficStreamManager) Restart(parent context.Context, apiURL string) {
	m.Stop()
	m.Start(parent, apiURL)
}

func (m *TrafficStreamManager) emitErrorLog(err error) {
	if err == nil {
		return
	}

	if m.getLogLevel != nil && !shouldEmitLog(m.getLogLevel(), "error") {
		return
	}

	msg := err.Error()

	m.mu.Lock()
	if msg == m.lastErrMsg && time.Since(m.lastErrAt) < 10*time.Second {
		m.mu.Unlock()
		return
	}
	m.lastErrMsg = msg
	m.lastErrAt = time.Now()
	m.mu.Unlock()

	entry := logger.LogEntry{
		Type:    "error",
		Payload: "Traffic stream error: " + msg,
		Time:    time.Now().Format("15:04:05"),
	}

	logger.AppLogs.Add(entry)

	if m.emit != nil {
		m.emit.Emit("log-message", entry)
	}
}

func shouldEmitLog(configLevel, entryLevel string) bool {
	rank := map[string]int{
		"debug":   0,
		"info":    1,
		"warning": 2,
		"warn":    2,
		"error":   3,
	}

	cfg, ok := rank[strings.ToLower(configLevel)]
	if !ok {
		cfg = rank["info"]
	}

	ent, ok := rank[strings.ToLower(entryLevel)]
	if !ok {
		ent = rank["info"]
	}

	return ent >= cfg
}
