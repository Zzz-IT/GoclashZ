package appcore

type EventSink interface {
	Emit(name string, args ...any)
}

const (
	EventClashExited      = "clash-exited"
	EventUpdateProgress   = "update-progress"
	EventTrafficMetrics   = "traffic-metrics"
	EventBehaviorChanged  = "behavior-changed"
	EventCoreRestarted    = "core-restarted"
	EventStateSync        = "app-state-sync"
	EventNotifyError      = "notify-error"
	EventLogMessage       = "log-message"
)
