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

// RingBuffer 线程安全的环形日志缓冲区
type RingBuffer struct {
	mu   sync.RWMutex // 🚀 并发安全锁
	data []LogEntry
	max  int
}

func NewRingBuffer(max int) *RingBuffer {
	return &RingBuffer{
		data: make([]LogEntry, 0, max),
		max:  max,
	}
}

// Add 追加日志（加写锁）
func (r *RingBuffer) Add(entry LogEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.data) >= r.max {
		// 丢弃最老的一条，追加新数据
		r.data = append(r.data[1:], entry)
	} else {
		r.data = append(r.data, entry)
	}
}

// GetAll 获取全部日志（加读锁）
func (r *RingBuffer) GetAll() []LogEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 🚀 核心修复：必须深拷贝返回。
	// 如果直接 return r.data，Wails 在序列化为 JSON 时，底层仍在 append，会直接触发 fatal error。
	result := make([]LogEntry, len(r.data))
	copy(result, r.data)
	return result
}

// Search 供前端搜索日志 (加读锁)
func (r *RingBuffer) Search(keyword string) []LogEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	var result []LogEntry
	lowerKey := strings.ToLower(keyword)
	for _, entry := range r.data {
		if strings.Contains(strings.ToLower(entry.Payload), lowerKey) || 
		   strings.Contains(strings.ToLower(entry.Type), lowerKey) {
			result = append(result, entry)
		}
	}
	return result
}

// 全局单例，限制 500 条防止内存溢出
var AppLogs = NewRingBuffer(500)

// Clear 清空日志
func (r *RingBuffer) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = make([]LogEntry, 0, r.max)
}
