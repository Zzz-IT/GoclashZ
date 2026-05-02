package appcore

import (
	"context"
	"goclashz/core/traffic"
	"sync"
)

type TrafficStreamManager struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	gen    int
	emit   EventSink
}

func NewTrafficStreamManager(emit EventSink) *TrafficStreamManager {
	return &TrafficStreamManager{
		emit: emit,
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
