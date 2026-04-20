package logger

import (
	"strings"
	"sync"
)

type LogEntry struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Time    string `json:"time"`
}

const MaxLogLines = 1000 // 最大日志存储量

type LogBuffer struct {
	mu      sync.RWMutex
	entries []LogEntry
}

var AppLogs = &LogBuffer{
	entries: make([]LogEntry, 0, MaxLogLines),
}

func (b *LogBuffer) Add(entry LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.entries = append(b.entries, entry)

	// 如果超过限制，移除最旧的条目
	if len(b.entries) > MaxLogLines {
		// 截断 Slice，保留最后 MaxLogLines 条
		b.entries = b.entries[len(b.entries)-MaxLogLines:]
	}
}

func (b *LogBuffer) GetAll() []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// 返回副本以防并发修改
	res := make([]LogEntry, len(b.entries))
	copy(res, b.entries)
	return res
}

func (b *LogBuffer) Search(keyword string) []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var res []LogEntry
	lowerKw := strings.ToLower(keyword)
	for _, entry := range b.entries {
		if strings.Contains(strings.ToLower(entry.Payload), lowerKw) {
			res = append(res, entry)
		}
	}
	return res
}

