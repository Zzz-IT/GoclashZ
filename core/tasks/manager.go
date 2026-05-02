package tasks

import (
	"context"
	"fmt"
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

	go func(myCancel context.CancelFunc) {
		m.events.Emit(name + "-start")
		err := fn(taskCtx)

		m.mu.Lock()
		// 🚀 核心修复：只有当 map 中的 cancel 函数等于当前的 myCancel 时，才进行清理
		// 这样可以防止：新任务 A 启动后替换了旧任务 B 的 cancel，但旧任务 B 执行完后把新任务 A 的 cancel 给删了
		if currentCancel, ok := m.tasks[name]; ok && fmt.Sprintf("%p", currentCancel) == fmt.Sprintf("%p", myCancel) {
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
	}(cancel)
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
