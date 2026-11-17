//go:build windows

package logger

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// 框架层（Wails v3）日志适配。
//
// 审计 A3 F-A3-06：application.Options.Logger 不设置时，v3 生产构建默认使用
// slog.NewTextHandler(io.Discard, nil)，框架日志与错误被整盘丢弃；事件类型
// 校验失败也只走 handleError，外界无感知。这里提供 slog.Handler 与
// ErrorHandler 的落地实现，把框架日志并入 AppLogs（→ 内存环 + JSONL 文件）。
const wailsPrefix = "[wails] "

// slogHandler 把 slog.Record 转成 LogEntry。
type slogHandler struct {
	level  slog.Level
	attrs  []slog.Attr
	groups []string
}

// SlogHandler 返回可交给 application.Options.Logger 的 handler（默认 Info 级）。
func SlogHandler() slog.Handler {
	return &slogHandler{level: slog.LevelInfo}
}

func (h *slogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *slogHandler) Handle(_ context.Context, r slog.Record) error {
	var sb strings.Builder
	sb.WriteString(wailsPrefix)
	sb.WriteString(r.Message)
	for _, a := range h.attrs {
		h.appendAttr(&sb, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		h.appendAttr(&sb, a)
		return true
	})

	AppLogs.Add(LogEntry{
		Type:    slogLevelToType(r.Level),
		Source:  LogSourceApp,
		Payload: sb.String(),
		Event:   "wails.log",
	})
	return nil
}

func (h *slogHandler) appendAttr(sb *strings.Builder, a slog.Attr) {
	if a.Key == "" {
		return
	}
	key := a.Key
	if len(h.groups) > 0 {
		key = strings.Join(append(append([]string{}, h.groups...), a.Key), ".")
	}
	fmt.Fprintf(sb, " %s=%v", key, a.Value)
}

func (h *slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	nh := *h
	nh.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &nh
}

func (h *slogHandler) WithGroup(name string) slog.Handler {
	nh := *h
	nh.groups = append(append([]string{}, h.groups...), name)
	return &nh
}

func slogLevelToType(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "error"
	case l >= slog.LevelWarn:
		return "warn"
	case l >= slog.LevelInfo:
		return "info"
	default:
		return "debug"
	}
}

// LogFrameworkError 是 application.Options.ErrorHandler 的落地实现。
//
// v3 在事件类型校验失败时（validateCustomEvent）会把 error 交给 ErrorHandler，
// 语义是「该事件被静默丢弃」——必须至少留一条 warn，否则前端只会看到事件
// 「莫名不来」（审计 A3 F-A3-06）。
func LogFrameworkError(err error) {
	if err == nil {
		return
	}

	msg := err.Error()
	if strings.Contains(msg, "does not match registered data type") {
		AppLogs.Add(LogEntry{
			Type:    "warn",
			Source:  LogSourceApp,
			Payload: wailsPrefix + "事件类型不匹配，v3 已丢弃该事件: " + msg,
			Event:   "wails.event.typeMismatch",
		})
		return
	}

	AppLogs.Add(LogEntry{
		Type:    "error",
		Source:  LogSourceApp,
		Payload: wailsPrefix + msg,
		Event:   "wails.error",
	})
}
