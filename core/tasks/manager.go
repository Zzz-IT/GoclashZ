//go:build windows

package tasks

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"goclashz/core/logger"
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

	go m.run(name, myID, autoSuccess, taskCtx, fn)
}

func (m *Manager) RunIfIdle(
	parentCtx context.Context,
	name string,
	autoSuccess bool,
	fn func(context.Context) error,
) bool {
	m.mu.Lock()
	if _, exists := m.tasks[name]; exists {
		m.mu.Unlock()
		return false
	}

	taskCtx, cancel := context.WithCancel(parentCtx)
	myID := time.Now().UnixNano()
	m.tasks[name] = taskHandle{id: myID, cancel: cancel}
	m.mu.Unlock()

	go m.run(name, myID, autoSuccess, taskCtx, fn)

	return true
}

// run 是 Run / RunIfIdle 共用的执行体。
//
// 关键约束：任务体内的任何 panic 都必须在这里被兜住。Go 的 panic 不会跨 goroutine
// 传播，一旦逃出去就是进程级 fatal —— 而本程序是 GUI 子系统，没有 stderr，
// 用户只会看到「点一下更新，软件凭空消失」。
func (m *Manager) run(name string, id int64, autoSuccess bool, taskCtx context.Context, fn func(context.Context) error) {
	startedAt := time.Now()

	defer func() {
		if r := recover(); r != nil {
			cost := time.Since(startedAt).Round(time.Millisecond)
			logger.Errorf("task %s panic after %s: %v\n%s", name, cost, r, debug.Stack())
			m.release(name, id)
			m.emit(name+"-error", fmt.Sprintf("任务内部错误: %v", r))
		}
	}()

	m.emit(name + "-start")
	err := fn(taskCtx)
	m.release(name, id)

	cost := time.Since(startedAt).Round(time.Millisecond)
	if err != nil {
		if taskCtx.Err() != nil {
			logger.Warnf("task %s cancelled after %s: %v", name, cost, err)
			m.emit(name + "-cancelled")
			return
		}
		logger.Errorf("task %s failed after %s: %v", name, cost, err)
		m.emit(name+"-error", err.Error())
		return
	}

	logger.Infof("task %s done in %s (autoSuccess=%v)", name, cost, autoSuccess)
	if autoSuccess {
		m.emit(name + "-success")
	}
}

// release 只在 ID 仍然匹配时删除任务条目，避免把后来者的条目误删。
func (m *Manager) release(name string, id int64) {
	m.mu.Lock()
	// 🚀 核心修复：基于 ID 的精确清理，防止新任务覆盖后旧任务误删
	if handle, ok := m.tasks[name]; ok && handle.id == id {
		delete(m.tasks, name)
	}
	m.mu.Unlock()
}

// emit 包一层 recover：事件发射（含框架侧对未注册 / payload 类型不符的处理）
// 不得反过来杀死任务 goroutine。
func (m *Manager) emit(eventName string, args ...any) {
	if m.events == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("task event %s emit panic: %v", eventName, r)
		}
	}()
	m.events.Emit(eventName, args...)
}

func (m *Manager) Cancel(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if handle, ok := m.tasks[name]; ok {
		handle.cancel()
		delete(m.tasks, name)
	}
}
