//go:build windows

package tasks

import (
	"context"
	"sync"
	"time"
)

// EventSink abstracts event emission
type EventSink interface {
	Emit(name string, args ...any)
}

type taskHandle struct {
	id     int64
	cancel context.CancelFunc
}

type Manager struct {
	mu     sync.Mutex
	tasks  map[string]taskHandle
	events EventSink
}

func NewManager(events EventSink) *Manager {
	return &Manager{
		tasks:  make(map[string]taskHandle),
		events: events,
	}
}

func (m *Manager) Run(parentCtx context.Context, name string, autoSuccess bool, fn func(context.Context) error) {
	m.mu.Lock()
	if old, exists := m.tasks[name]; exists {
		old.cancel()
	}

	taskCtx, cancel := context.WithCancel(parentCtx)
	myID := time.Now().UnixNano()
	m.tasks[name] = taskHandle{id: myID, cancel: cancel}
	m.mu.Unlock()

	go func(currentID int64) {
		m.events.Emit(name + "-start")
		err := fn(taskCtx)

		m.mu.Lock()
		// 🚀 核心修复：基于 ID 的精确清理，防止新任务覆盖后旧任务误删
		if handle, ok := m.tasks[name]; ok && handle.id == currentID {
			delete(m.tasks, name)
		}
		m.mu.Unlock()

		if err != nil {
			if taskCtx.Err() != nil {
				m.events.Emit(name + "-cancelled")
				return
			}
			m.events.Emit(name+"-error", err.Error())
			return
		}

		if autoSuccess {
			m.events.Emit(name + "-success")
		}
	}(myID)
}

func (m *Manager) Cancel(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if handle, ok := m.tasks[name]; ok {
		handle.cancel()
		delete(m.tasks, name)
	}
}

func (m *Manager) CancelAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, handle := range m.tasks {
		handle.cancel()
		delete(m.tasks, name)
	}
}
