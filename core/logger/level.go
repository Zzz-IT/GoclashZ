package logger

import "strings"

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func NormalizeLevel(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug", "trace":
		return "debug"
	case "info":
		return "info"
	case "warn", "warning":
		return "warn"
	case "error", "fatal", "panic":
		return "error"
	default:
		return "info" // unknown -> info
	}
}

func LevelRank(s string) int {
	switch NormalizeLevel(s) {
	case "debug":
		return 0
	case "info":
		return 1
	case "warn":
		return 2
	case "error":
		return 3
	default:
		return 1
	}
}

func ShouldShow(configLevel, entryLevel string) bool {
	return LevelRank(entryLevel) >= LevelRank(configLevel)
}
