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
	head     int
	tail     int
	count    int
}

// 预先分配好 1000 个长度的固定数组，永不扩容，彻底解决内存抖动
var AppLogs = &RingBuffer{
	logs:     make([]LogEntry, 1000),
	capacity: 1000,
}

func (rb *RingBuffer) Add(entry LogEntry) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.logs[rb.tail] = entry
	rb.tail = (rb.tail + 1) % rb.capacity

	if rb.count < rb.capacity {
		rb.count++
	} else {
		// 满了之后，覆盖旧数据，头指针同步向前推进
		rb.head = (rb.head + 1) % rb.capacity
	}
}

func (rb *RingBuffer) GetAll() []LogEntry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	res := make([]LogEntry, 0, rb.count)
	for i := 0; i < rb.count; i++ {
		idx := (rb.head + i) % rb.capacity
		res = append(res, rb.logs[idx])
	}
	return res
}

func (rb *RingBuffer) Search(keyword string) []LogEntry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	var res []LogEntry
	lowerKw := strings.ToLower(keyword)
	for i := 0; i < rb.count; i++ {
		idx := (rb.head + i) % rb.capacity
		if strings.Contains(strings.ToLower(rb.logs[idx].Payload), lowerKw) {
			res = append(res, rb.logs[idx])
		}
	}
	return res
}
