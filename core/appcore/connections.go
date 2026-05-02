package appcore

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"goclashz/core/clash"
	"goclashz/core/traffic"
)

type ConnectionsSnapshot struct {
	Connections []traffic.ConnectionVO `json:"connections"`
}

type ConnectionMonitorManager struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	gen    int
	emit   EventSink
}

func NewConnectionMonitorManager(emit EventSink) *ConnectionMonitorManager {
	return &ConnectionMonitorManager{
		emit: emit,
	}
}

func (m *ConnectionMonitorManager) GetSnapshot() (ConnectionsSnapshot, error) {
	raw, err := clash.GetConnectionsRaw()
	if err != nil {
		return ConnectionsSnapshot{}, err
	}

	var payload struct {
		Connections []traffic.RawConnection `json:"connections"`
	}

	if err := json.Unmarshal(raw, &payload); err != nil {
		return ConnectionsSnapshot{}, err
	}

	return ConnectionsSnapshot{
		Connections: traffic.ProcessConnections(payload.Connections),
	}, nil
}

func (m *ConnectionMonitorManager) Start(ctx context.Context) {
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
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		defer func() {
			m.mu.Lock()
			if m.gen == currentGen {
				m.cancel = nil
			}
			m.mu.Unlock()
		}()

		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				snap, err := m.GetSnapshot()
				if err != nil {
					continue
				}

				m.mu.Lock()
				alive := m.gen == currentGen && m.cancel != nil
				m.mu.Unlock()

				if !alive {
					return
				}

				if m.emit != nil {
					m.emit.Emit("connections-update", snap)
				}
			}
		}
	}()
}

func (m *ConnectionMonitorManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
		m.gen++
	}
}
