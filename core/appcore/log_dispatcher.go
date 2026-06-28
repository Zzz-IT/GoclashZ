package appcore

import (
	"time"

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

func (d *LogDispatcher) AddApp(entry logger.LogEntry) {
	if d == nil {
		logger.AppLogs.Add(entry)
		return
	}

	entry.Type = logger.NormalizeLevel(entry.Type)
	entry.Source = logger.LogSourceApp
	if entry.Time == "" {
		entry.Time = time.Now().Format("15:04:05")
	}

	logger.AppLogs.Add(entry)

	if d.emit != nil && logger.ShouldShow(d.currentAppLogLevel(), entry.Type) {
		d.emit.Emit(EventLogMessage, entry)
	}
}

func (d *LogDispatcher) AddCore(entry logger.LogEntry) {
	if d == nil {
		logger.AppLogs.Add(entry)
		return
	}

	entry.Type = logger.NormalizeLevel(entry.Type)
	entry.Source = logger.LogSourceCore
	if entry.Time == "" {
		entry.Time = time.Now().Format("15:04:05")
	}

	logger.AppLogs.Add(entry)

	if d.emit != nil {
		d.emit.Emit(EventLogMessage, entry)
	}
}
