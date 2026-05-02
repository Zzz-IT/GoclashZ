package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type WailsEventSink struct {
	ctx context.Context
}

func (s WailsEventSink) Emit(name string, args ...any) {
	runtime.EventsEmit(s.ctx, name, args...)
}
