//go:build windows

package main

import (
	"context"
	"sync"
	"time"

	"goclashz/core/logger"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// pendingEventLimit 是「应用启动完成前」事件队列的上限。
// 超过上限的事件丢弃并计数（绝不阻塞调用方），SetContext 后按序补发。
const pendingEventLimit = 256

type queuedEvent struct {
	name string
	args []any
}

type WailsEventSink struct {
	mu  sync.RWMutex
	ctx context.Context

	// pending 只在 ctx == nil（应用尚未就绪）时累积；SetContext 时整体取走补发。
	pending []queuedEvent
	// pendingDropped：队列满被丢弃的条数；replayFailed：补发失败的条数。
	// 两者都会写进日志（→ 内存环 + JSONL 文件），便于复盘启动期丢了多少事件。
	pendingDropped uint64
	replayFailed   uint64
}

// SetContext sets the context for the event sink
func (s *WailsEventSink) SetContext(ctx context.Context) {
	s.mu.Lock()
	s.ctx = ctx
	pending := s.pending
	s.pending = nil
	s.mu.Unlock()

	if len(pending) == 0 {
		return
	}

	start := time.Now()
	failed := 0
	for i := range pending {
		if s.emitNow(pending[i].name, pending[i].args...) {
			continue
		}
		failed++
		logger.Warnf("启动早期事件补发失败 (%d/%d): %s", i+1, len(pending), pending[i].name)
	}

	s.mu.Lock()
	s.replayFailed += uint64(failed)
	droppedAtEnqueue := s.pendingDropped
	s.mu.Unlock()

	// 结构化事件（F-A3-08）：补发量/失败量/耗时/入队期丢弃数一次说清，
	// 避免「启动期少了几个事件」只能靠数日志。
	entry := logger.NewEvent("event", "wails.event.replay").
		WithCorr("event-replay").
		WithDur(time.Since(start)).
		WithAttempt(len(pending)).
		WithCode("").
		WithPayload("启动早期事件补发：共 %d 条，失败 %d 条，入队期丢弃 %d 条，耗时 %dms",
			len(pending), failed, droppedAtEnqueue, time.Since(start).Milliseconds())
	if failed > 0 {
		entry = entry.WithLevel("warn").WithCode("EVENT_REPLAY_FAILED")
	}
	logger.Record(entry)
}

// Emit 实现 EventSink 接口，带指针接收者和 nil 安全检查
func (s *WailsEventSink) Emit(name string, args ...any) {
	if s == nil {
		return
	}

	s.mu.RLock()
	ctx := s.ctx
	s.mu.RUnlock()

	// ctx 仅作为「应用已启动」的判据保留（v2 的 runtime.EventsEmit 需要 ctx，
	// v3 的 Event.Emit 不需要）。早期事件不再直接丢弃：先进 pending 队列，
	// SetContext 之后按序补发，避免启动期时间线断裂（审计 A3 F-A3-12）。
	if ctx == nil {
		s.enqueuePending(name, args...)
		return
	}

	s.emitNow(name, args...)
}

// enqueuePending 把早期事件放入有界队列；满则丢弃并计数。
func (s *WailsEventSink) enqueuePending(name string, args ...any) {
	s.mu.Lock()
	if len(s.pending) >= pendingEventLimit {
		s.pendingDropped++
		dropped := s.pendingDropped
		s.mu.Unlock()
		logger.Warnf("启动早期事件队列已满(%d)，丢弃事件 %s（累计丢弃 %d）", pendingEventLimit, name, dropped)
		return
	}
	s.pending = append(s.pending, queuedEvent{name: name, args: args})
	depth := len(s.pending)
	s.mu.Unlock()

	if depth == pendingEventLimit {
		logger.Warnf("启动早期事件队列达到上限(%d)，后续事件将被丢弃", pendingEventLimit)
	}
}

// emitNow 投递一次事件；返回 false 表示当前无法投递（应用未就绪）。
func (s *WailsEventSink) emitNow(name string, args ...any) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Wails event emit panic: %s: %v", name, r)
			ok = false
		}
	}()

	app := application.Get()
	if app == nil {
		return false
	}
	app.Event.Emit(name, args...)
	return true
}
