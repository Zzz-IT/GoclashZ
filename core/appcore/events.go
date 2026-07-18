//go:build windows

package appcore

type EventSink interface {
	Emit(name string, args ...any)
}

const (
	EventClashExited     = "clash-exited"
	EventUpdateProgress  = "update-progress"
	EventTrafficMetrics  = "traffic-metrics"
	EventBehaviorChanged = "behavior-changed"
	EventCoreRestarted   = "core-restarted"
	EventStateSync       = "app-state-sync"
	EventNotifyError     = "notify-error"
	EventLogMessage      = "log-message"
	EventNotification    = "app-notification"
)

// Notification is the stable user-facing event payload. Legacy notify-error
// remains available during migration for older frontends.
type Notification struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Source  string `json:"source"`
}
