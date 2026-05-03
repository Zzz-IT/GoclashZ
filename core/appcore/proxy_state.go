//go:build windows

package appcore

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"goclashz/core/clash"
)

// ProxyGroupState 代理组精简状态
type ProxyGroupState struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Now  string `json:"now"`
}

// ProxyStateMonitor 负责监控策略组的选中状态变化
type ProxyStateMonitor struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	gen    int
	emit   EventSink
}

func NewProxyStateMonitor(emit EventSink) *ProxyStateMonitor {
	return &ProxyStateMonitor{
		emit: emit,
	}
}

func getProxyGroupStates() ([]ProxyGroupState, error) {
	data, err := clash.GetInitialData()
	if err != nil {
		return nil, err
	}

	groups, ok := data["groups"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid proxy groups")
	}

	out := make([]ProxyGroupState, 0)

	for name, raw := range groups {
		g, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		typ, _ := g["type"].(string)
		now, _ := g["now"].(string)

		switch typ {
		case "Selector", "URLTest", "Fallback", "LoadBalance":
			// 排除系统保留组
			if name == "GLOBAL" || name == "DIRECT" || name == "REJECT" {
				continue
			}

			out = append(out, ProxyGroupState{
				Name: name,
				Type: typ,
				Now:  now,
			})
		}
	}

	return out, nil
}

func (m *ProxyStateMonitor) Start(ctx context.Context) {
	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return
	}

	m.gen++
	currentGen := m.gen

	runCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.mu.Unlock()

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		defer func() {
			m.mu.Lock()
			if m.gen == currentGen {
				m.cancel = nil
			}
			m.mu.Unlock()
		}()

		var lastHash string

		for {
			select {
			case <-runCtx.Done():
				return

			case <-ticker.C:
				if !clash.IsRunning() {
					continue
				}

				states, err := getProxyGroupStates()
				if err != nil {
					continue
				}

				// 使用 JSON 哈希简单的对比数据是否变化
				payload, _ := json.Marshal(states)
				hash := string(payload)
				if hash == lastHash {
					continue
				}
				lastHash = hash

				m.mu.Lock()
				alive := m.gen == currentGen && m.cancel != nil
				m.mu.Unlock()
				if !alive {
					return
				}

				// 发送增量更新事件
				m.emit.Emit("proxy-state-sync", states)
			}
		}
	}()
}

func (m *ProxyStateMonitor) Stop() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
		m.gen++
	}
	m.mu.Unlock()
}
