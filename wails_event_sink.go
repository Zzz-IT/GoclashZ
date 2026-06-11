//go:build windows

package main

import (
	"goclashz/core/logger"
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type WailsEventSink struct {
	ctx context.Context
}

// Emit 实现 EventSink 接口，带指针接收者和 nil 安全检查
func (s *WailsEventSink) Emit(name string, args ...any) {
	if s == nil || s.ctx == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Wails event emit panic: %s: %v", name, r)
		}
	}()

	runtime.EventsEmit(s.ctx, name, args...)
}
