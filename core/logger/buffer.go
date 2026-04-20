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

// 优化：采用定长环形缓冲区 (Ring Buffer) 避免切片扩容和截断拷贝带来的 GC 压力
type LogBuffer struct {
	mu      sync.RWMutex
	entries [MaxLogLines]LogEntry // 固定长度数组
	head    int                   // 下一个插入位置的索引
	count   int                   // 当前已存储的有效日志总数
}

var AppLogs = &LogBuffer{
	// 数组已按定长自动初始化，无需 make
}

func (b *LogBuffer) Add(entry LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 插入数据到 head 位置
	b.entries[b.head] = entry
	
	// head 指针循环递增
	b.head = (b.head + 1) % MaxLogLines
	
	// 维护最大条目数
	if b.count < MaxLogLines {
		b.count++
	}
}

func (b *LogBuffer) GetAll() []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	res := make([]LogEntry, 0, b.count)
	
	// 按照时间先后顺序（最旧到最新）提取日志
	for i := 0; i < b.count; i++ {
		// 计算实际索引：从最老的数据位置开始
		idx := (b.head - b.count + i + MaxLogLines) % MaxLogLines
		res = append(res, b.entries[idx])
	}
	
	return res
}

func (b *LogBuffer) Search(keyword string) []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var res []LogEntry
	lowerKw := strings.ToLower(keyword)
	
	// 按时间顺序搜索
	for i := 0; i < b.count; i++ {
		idx := (b.head - b.count + i + MaxLogLines) % MaxLogLines
		entry := b.entries[idx]
		if strings.Contains(strings.ToLower(entry.Payload), lowerKw) {
			res = append(res, entry)
		}
	}
	return res
}
