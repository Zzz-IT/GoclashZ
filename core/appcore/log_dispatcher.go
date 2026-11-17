//go:build windows

package appcore

import (
	"goclashz/core/logger"
)

type LogDispatcher struct {
	emit           EventSink
	getAppLogLevel func() string
}

func NewLogDispatcher(emit EventSink, getAppLogLevel func() string) *LogDispatcher {
	return &LogDispatcher{
		emit:           emit,
		getAppLogLevel: getAppLogLevel,
	}
}

func (d *LogDispatcher) currentAppLogLevel() string {
	if d == nil || d.getAppLogLevel == nil {
		return "info"
	}
	return logger.NormalizeLevel(d.getAppLogLevel())
}

// normalizeEntry 统一补齐「级别/来源」并按 logger 侧规则补 TS/PID/Time。
//
// 内存环、前端推送与落盘 JSONL 共用同一份字段（时间由 logger.NormalizeEntry
// 统一签发，避免三处各取一次 time.Now 导致毫秒级错位，见审计 A3 F-A3-07）。
func normalizeEntry(entry logger.LogEntry, source logger.LogSource) logger.LogEntry {
	entry.Type = logger.NormalizeLevel(entry.Type)
	entry.Source = source
	return logger.NormalizeEntry(entry)
}

// AddApp 写入一条应用日志。
// 注意：写盘（logger.AppLogs.Add → 文件 sink）在 UI 级别过滤之前执行，
// 所以即使 UI 只看 error，info 级别的应用日志也不会从磁盘上消失。
func (d *LogDispatcher) AddApp(entry logger.LogEntry) {
	entry = d.AddAppStored(entry)
	if d == nil || d.emit == nil {
		return
	}
	if logger.ShouldShow(d.currentAppLogLevel(), entry.Type) {
		d.emit.Emit(EventLogMessage, entry)
	}
}

// AddCore 写入一条内核日志并推前端。
//
// 保持既有语义（不过滤、必推）。需要「先落盘、再按级别/速率决定是否推 UI」的
// 调用方（LogStreamManager）请改用 AddCoreStored + EmitLog（审计 A3 F-A3-10/11）。
func (d *LogDispatcher) AddCore(entry logger.LogEntry) {
	entry = d.AddCoreStored(entry)
	d.EmitLog(entry)
}

// AddAppStored 只写「内存环 + JSONL 文件」，不推前端；返回归一化后的条目。
func (d *LogDispatcher) AddAppStored(entry logger.LogEntry) logger.LogEntry {
	entry = normalizeEntry(entry, logger.LogSourceApp)
	logger.AppLogs.Add(entry)
	return entry
}

// AddCoreStored 只写「内存环 + JSONL 文件」，不推前端；返回归一化后的条目。
// 磁盘侧永远全量：UI 的级别过滤/限速只影响前端，不影响可回放。
func (d *LogDispatcher) AddCoreStored(entry logger.LogEntry) logger.LogEntry {
	entry = normalizeEntry(entry, logger.LogSourceCore)
	logger.AppLogs.Add(entry)
	return entry
}

// EmitLog 只把条目推给前端（不再写环/落盘），供限速后的 UI 通道使用。
func (d *LogDispatcher) EmitLog(entry logger.LogEntry) {
	if d == nil || d.emit == nil {
		return
	}
	d.emit.Emit(EventLogMessage, entry)
}
