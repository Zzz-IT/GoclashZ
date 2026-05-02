package tasks

import (
	"context"
	"sync"
)

// EventSink abstracts event emission (structurally compatible with appcore.EventSink)
type EventSink interface {
	Emit(name string, args ...any)
}

type Manager struct {
	mu     sync.Mutex
	tasks  map[string]context.CancelFunc
	events EventSink
}

func NewManager(events EventSink) *Manager {
	return &Manager{
		tasks:  make(map[string]context.CancelFunc),
		events: events,
	}
}

func (m *Manager) Run(parentCtx context.Context, name string, autoSuccess bool, fn func(context.Context) error) {
	m.mu.Lock()
	if oldCancel, exists := m.tasks[name]; exists {
		oldCancel()
	}

	taskCtx, cancel := context.WithCancel(parentCtx)
	m.tasks[name] = cancel
	m.mu.Unlock()

	go func() {
		m.events.Emit(name + "-start")
		err := fn(taskCtx)

		m.mu.Lock()
		if current, ok := m.tasks[name]; ok {
			current()
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
	}()
}

func (m *Manager) Cancel(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cancel, ok := m.tasks[name]; ok {
		cancel()
		delete(m.tasks, name)
	}
}

func (m *Manager) CancelAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, cancel := range m.tasks {
		cancel()
		delete(m.tasks, name)
	}
}
