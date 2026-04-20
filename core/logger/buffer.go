// 文件路径: core/logger/buffer.go
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

type RingBuffer struct {
	mu       sync.RWMutex
	logs     []LogEntry
	capacity int
}

// 初始化一个容量为 1000 的日志缓冲
var AppLogs = &RingBuffer{
	logs:     make([]LogEntry, 0, 1000),
	capacity: 1000,
}

func (rb *RingBuffer) Add(entry LogEntry) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.logs = append(rb.logs, entry)
	if len(rb.logs) > rb.capacity {
		rb.logs = rb.logs[1:] // 抛弃最旧的一条
	}
}

func (rb *RingBuffer) GetAll() []LogEntry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	// 返回副本防止前端并发修改
	res := make([]LogEntry, len(rb.logs))
	copy(res, rb.logs)
	return res
}

func (rb *RingBuffer) Search(keyword string) []LogEntry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	var res []LogEntry
	lowerKw := strings.ToLower(keyword)
	for _, l := range rb.logs {
		if strings.Contains(strings.ToLower(l.Payload), lowerKw) {
			res = append(res, l)
		}
	}
	return res
}
