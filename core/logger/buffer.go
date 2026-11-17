//go:build windows

package logger

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type LogSource string

const (
	LogSourceCore LogSource = "core"
	LogSourceApp  LogSource = "app"
)

type LogEntry struct {
	Type    string    `json:"type"`
	Source  LogSource `json:"source"`
	Payload string    `json:"payload"`
	// Time 保留为兼容字段（HH:MM:SS，前端旧版消费的就是它）。
	Time string `json:"time"`
	// TS 是 RFC3339 毫秒时间戳（含日期与时区），跨天/跨次启动可排序（审计 A3 F-A3-07）。
	TS string `json:"ts,omitempty"`
	// PID 用于区分主进程与其它进程的日志条目（审计 A3 F-A3-07）。
	PID int `json:"pid,omitempty"`
	// Event 是稳定的事件名（如 boot.start / asset.repair.failed），落盘 JSONL 的 event 字段。
	Event string `json:"event,omitempty"`
	// 以下为结构化字段（审计 A3 F-A3-08）：all optional，缺省不影响既有语义。
	Cat     string `json:"cat,omitempty"`     // 子系统：asset/helper/core/log/tun/diag/boot/event…
	Code    string `json:"code,omitempty"`    // 错误码（仅失败时）
	Corr    string `json:"corr,omitempty"`    // 一次用户动作/事务的关联 ID
	DurMS   int64  `json:"durMs,omitempty"`   // 本步骤耗时（毫秒）
	Attempt int    `json:"attempt,omitempty"` // 第几次尝试（≥1 才有意义）
	// Caller 是产生这条日志的调用点（仓库内相对路径:行号）。
	// 只由 Infof/Warnf/Errorf/Debugf/Eventf 自动填写；经 dispatcher 透传的条目为空，
	// 避免把它记成 dispatcher 自己（审计 A3 F-A3-08「无调用点」）。
	Caller string `json:"caller,omitempty"`
}

// callerAt 返回调用点（形如 "core/appcore/logs.go:120"）。skip 是相对 callerAt 的帧数。
func callerAt(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok || file == "" {
		return ""
	}
	file = strings.ReplaceAll(file, "\\", "/")
	if i := strings.LastIndex(file, "/goclashz/"); i >= 0 {
		file = file[i+len("/goclashz/"):]
	} else if parts := strings.Split(file, "/"); len(parts) >= 2 {
		file = parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	return fmt.Sprintf("%s:%d", file, line)
}

// logPID 是进程启动时的 PID，避免每条日志都取一次。
var logPID = os.Getpid()

// NowTS 返回 RFC3339 毫秒时间戳（本地时区），与落盘 JSONL 的 ts 同一格式。
func NowTS() string {
	return time.Now().Format(logTimeFormat)
}

// NormalizeEntry 补齐日志条目的通用字段：TS / PID / Time。
// 级别与来源由调用方决定（dispatcher 需要区分 app/core），不在这里改写。
// 所有入环路径（RingBuffer.Add）与 UI 推送路径（dispatcher）都走它，
// 保证「内存环、前端面板、JSONL 文件」三处时间字段一致。
func NormalizeEntry(entry LogEntry) LogEntry {
	if entry.TS == "" {
		entry.TS = NowTS()
	}
	if entry.PID == 0 {
		entry.PID = logPID
	}
	if entry.Time == "" {
		if t, err := time.Parse(logTimeFormat, entry.TS); err == nil {
			entry.Time = t.Format("15:04:05")
		} else {
			entry.Time = time.Now().Format("15:04:05")
		}
	}
	return entry
}

// ---- 结构化事件构造（审计 A3 F-A3-08） ----
//
// 用法（示意）：
//
//	start := time.Now()
//	logger.Record(logger.NewEvent("core", "core.logstream.start").
//		WithSource(logger.LogSourceCore).
//		WithCorr(fmt.Sprintf("gen-%d", gen)).
//		WithPayload("level=%s", level))
//	...
//	logger.Record(logger.NewEvent("core", "core.logstream.stop").
//		WithSource(logger.LogSourceCore).WithDur(time.Since(start)))
//
// 这些字段会同时进入内存环、前端事件与 JSONL 文件（file.go 的 fileRecord）。

// NewEvent 构造一条结构化日志（默认 level=info, source=app）。
func NewEvent(cat, event string) LogEntry {
	return LogEntry{Type: "info", Source: LogSourceApp, Cat: cat, Event: event}
}

func (e LogEntry) WithLevel(level string) LogEntry   { e.Type = NormalizeLevel(level); return e }
func (e LogEntry) WithSource(src LogSource) LogEntry { e.Source = src; return e }
func (e LogEntry) WithCode(code string) LogEntry     { e.Code = code; return e }
func (e LogEntry) WithCorr(corr string) LogEntry     { e.Corr = corr; return e }
func (e LogEntry) WithAttempt(n int) LogEntry        { e.Attempt = n; return e }

// WithDur 记录本步骤耗时（毫秒）。
func (e LogEntry) WithDur(d time.Duration) LogEntry {
	e.DurMS = d.Milliseconds()
	return e
}

// WithPayload 设置自由文本（保持与 Infof 一致的格式化语义）。
func (e LogEntry) WithPayload(format string, args ...any) LogEntry {
	e.Payload = fmt.Sprintf(format, args...)
	return e
}

// Record 写入一条日志（内存环 + JSONL 文件），是 Eventf/自定义事件的统一出口。
func Record(entry LogEntry) {
	AppLogs.Add(entry)
}

// Eventf 写一条 info 级结构化事件（cat/event/code/corr 固定字段 + 文本）。
func Eventf(cat, event, code, corr, format string, args ...any) {
	entry := NewEvent(cat, event).
		WithCode(code).
		WithCorr(corr).
		WithPayload(format, args...)
	entry.Caller = callerAt(2)
	Record(entry)
}

// ---- 环形缓冲 ----

// ringSlot 把条目与自增序号绑在一起：序号用于「主环 + 保留区」两路归并与去重。
type ringSlot struct {
	entry LogEntry
	seq   uint64
}

// RingBuffer 线程安全的环形日志缓冲区。
//
// F-A3-09：真实环形索引（固定长度数组 + head/size），命中满环时零分配、零 memmove，
// 取代原先 append(data[1:], entry) 的 O(n) 复制（每次约 32KB memmove + 周期性 ~50KB 分配）。
//
// F-A3-11：可选的「保留区」（pin）单独保存 warn/error，保证错误不会被 core info 洪水冲掉；
// GetAll 会把主环与保留区按写入顺序合并去重，因此调用方（controller.GetRecentLogs）
// 无需改动即可拿到「全量 + 错误必留」的视图。
type RingBuffer struct {
	mu    sync.RWMutex
	slots []ringSlot
	head  int
	size  int
	max   int

	pinSlots []ringSlot
	pinHead  int
	pinSize  int
	pinMax   int

	seq atomic.Uint64
}

func NewRingBuffer(max int) *RingBuffer {
	if max <= 0 {
		max = 1
	}
	return &RingBuffer{
		slots: make([]ringSlot, max),
		max:   max,
	}
}

// NewTieredRingBuffer 创建带「warn/error 保留区」的环形缓冲。
// pinMax<=0 时等价于 NewRingBuffer。
func NewTieredRingBuffer(max, pinMax int) *RingBuffer {
	r := NewRingBuffer(max)
	if pinMax > 0 {
		r.pinSlots = make([]ringSlot, pinMax)
		r.pinMax = pinMax
	}
	return r
}

// isPinnedLevel 判断某级别是否进入保留区（warn / error）。
func isPinnedLevel(level string) bool {
	return LevelRank(level) >= LevelRank("warn")
}

// Add 追加日志（加写锁）
//
// 落盘挂钩放在锁外：所有日志（App/Core/stdlib 收编）最终都经这里进入内存环，
// 因此这也是「全量写盘」的唯一入口，避免在 dispatcher 里重复记录两份。
func (r *RingBuffer) Add(entry LogEntry) {
	// 统一补 TS/PID/Time（见 NormalizeEntry）：内存环与落盘都拿同一份字段。
	entry = NormalizeEntry(entry)
	slot := ringSlot{entry: entry, seq: r.seq.Add(1)}

	r.mu.Lock()
	r.slots[r.head] = slot
	r.head = (r.head + 1) % r.max
	if r.size < r.max {
		r.size++
	}
	if r.pinMax > 0 && isPinnedLevel(entry.Type) {
		r.pinSlots[r.pinHead] = slot
		r.pinHead = (r.pinHead + 1) % r.pinMax
		if r.pinSize < r.pinMax {
			r.pinSize++
		}
	}
	r.mu.Unlock()

	enqueueToSink(entry)
}

// Len 返回当前条目数（主环，不含保留区）。
func (r *RingBuffer) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.size
}

// Capacity 返回主环容量。
func (r *RingBuffer) Capacity() int {
	return r.max
}

// PinLen 返回 warn/error 保留区当前条目数（无保留区时为 0）。
func (r *RingBuffer) PinLen() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pinSize
}

// PinCapacity 返回 warn/error 保留区容量（无保留区时为 0）。
func (r *RingBuffer) PinCapacity() int {
	return r.pinMax
}

// snapshotLocked 按写入顺序返回主环 + 保留区（去重后）。调用方必须持有读锁。
func (r *RingBuffer) snapshotLocked() []ringSlot {
	if r.size == 0 {
		return nil
	}

	main := make([]ringSlot, r.size)
	start := r.head - r.size
	for start < 0 {
		start += r.max
	}
	for i := 0; i < r.size; i++ {
		main[i] = r.slots[(start+i)%r.max]
	}
	if r.pinSize == 0 {
		return main
	}

	pin := make([]ringSlot, r.pinSize)
	pstart := r.pinHead - r.pinSize
	for pstart < 0 {
		pstart += r.pinMax
	}
	for i := 0; i < r.pinSize; i++ {
		pin[i] = r.pinSlots[(pstart+i)%r.pinMax]
	}

	// 两路都按 seq 单调递增，做一次线性归并（同 seq 说明是同一条，只保留一份）。
	merged := make([]ringSlot, 0, len(main)+len(pin))
	i, j := 0, 0
	for i < len(main) && j < len(pin) {
		switch {
		case main[i].seq == pin[j].seq:
			merged = append(merged, main[i])
			i++
			j++
		case main[i].seq < pin[j].seq:
			merged = append(merged, main[i])
			i++
		default:
			merged = append(merged, pin[j])
			j++
		}
	}
	merged = append(merged, main[i:]...)
	merged = append(merged, pin[j:]...)
	return merged
}

// GetAll 获取全部日志（主环 + 保留区，按时间顺序）。
func (r *RingBuffer) GetAll() []LogEntry {
	r.mu.RLock()
	slots := r.snapshotLocked()
	r.mu.RUnlock()

	// 深拷贝返回：Wails 序列化期间底层可能在写入，直接返回会触发 fatal error。
	result := make([]LogEntry, len(slots))
	for i := range slots {
		result[i] = slots[i].entry
	}
	return result
}

// Search 供前端搜索日志（payload/type/cat/code/event 任一命中）。
func (r *RingBuffer) Search(keyword string) []LogEntry {
	key := strings.ToLower(keyword)

	r.mu.RLock()
	slots := r.snapshotLocked()
	r.mu.RUnlock()

	var result []LogEntry
	for _, slot := range slots {
		e := slot.entry
		if strings.Contains(strings.ToLower(e.Payload), key) ||
			strings.Contains(strings.ToLower(e.Type), key) ||
			strings.Contains(strings.ToLower(e.Cat), key) ||
			strings.Contains(strings.ToLower(e.Code), key) ||
			strings.Contains(strings.ToLower(e.Event), key) {
			result = append(result, e)
		}
	}
	return result
}

// AppLogs 全局日志环：主环 2000 条，其中 warn/error 另有 500 条保留区（F-A3-11）。
var AppLogs = NewTieredRingBuffer(2000, 500)

// Clear 清空日志（主环 + 保留区）
func (r *RingBuffer) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.head = 0
	r.size = 0
	r.pinHead = 0
	r.pinSize = 0
}

func Infof(format string, args ...any) {
	AppLogs.Add(LogEntry{
		Type:    "info",
		Source:  LogSourceApp,
		Payload: fmt.Sprintf(format, args...),
		Caller:  callerAt(2),
	})
}

func Errorf(format string, args ...any) {
	AppLogs.Add(LogEntry{
		Type:    "error",
		Source:  LogSourceApp,
		Payload: fmt.Sprintf(format, args...),
		Caller:  callerAt(2),
	})
}

func Warnf(format string, args ...any) {
	AppLogs.Add(LogEntry{
		Type:    "warn",
		Source:  LogSourceApp,
		Payload: fmt.Sprintf(format, args...),
		Caller:  callerAt(2),
	})
}

func Debugf(format string, args ...any) {
	AppLogs.Add(LogEntry{
		Type:    "debug",
		Source:  LogSourceApp,
		Payload: fmt.Sprintf(format, args...),
		Caller:  callerAt(2),
	})
}
