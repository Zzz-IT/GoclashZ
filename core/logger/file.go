//go:build windows

package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 文件 sink：把内存环里的日志同步落成 JSONL 一行一条。
//
// 设计约定（对应审计 A3 的 F-A3-01 / F-A3-09 / F-A3-21）：
//   - 异步：写盘走 channel（容量 4096），调用方不因磁盘 IO 阻塞；
//     队列满时 debug 直接丢弃并计数，error/warn/info 退化为同步补写，保证不丢。
//   - 按天命名：app-YYYYMMDD.jsonl；单文件超过 8MB 自动切分片 app-YYYYMMDD.N.jsonl。
//   - 保留 7 天：初始化与跨天时清理过期文件。
//   - ts 使用 RFC3339（毫秒），不受 LogEntry.Time（仅 HH:MM:SS）限制。
const (
	logFilePrefix    = "app"
	logFileMaxBytes  = 8 * 1024 * 1024
	logRetentionDays = 7
	logQueueSize     = 4096
	logTimeFormat    = "2006-01-02T15:04:05.000Z07:00"
	logFlushTimeout  = 2 * time.Second
	// stdWriterLineLimit 限制标准库 log 未换行的残片长度，避免无界增长。
	stdWriterLineLimit = 64 * 1024
	// logDropReportInterval 是「写盘丢弃」告警的上报周期。
	logDropReportInterval = 60 * time.Second
)

// LogSourceStd 标记经由标准库 log 收编进来的日志（如 core/runtimeassets）。
const LogSourceStd LogSource = "std"

// record 是落盘记录（LogEntry 的规范化投影）。
type record struct {
	t       time.Time
	pid     int
	level   string
	source  string
	payload string
	event   string
	// 结构化字段（审计 A3 F-A3-08）：与 LogEntry 一一对应，空值不落盘。
	cat     string
	code    string
	corr    string
	durMS   int64
	attempt int
	caller  string
}

// fileRecord 是 JSONL 一行的结构。
type fileRecord struct {
	TS      string `json:"ts"`
	PID     int    `json:"pid,omitempty"`
	Level   string `json:"level"`
	Source  string `json:"source"`
	Payload string `json:"payload"`
	Event   string `json:"event,omitempty"`
	Cat     string `json:"cat,omitempty"`
	Code    string `json:"code,omitempty"`
	Corr    string `json:"corr,omitempty"`
	DurMS   int64  `json:"durMs,omitempty"`
	Attempt int    `json:"attempt,omitempty"`
	Caller  string `json:"caller,omitempty"`
}

func (r record) marshal() ([]byte, error) {
	b, err := json.Marshal(fileRecord{
		TS:      r.t.Format(logTimeFormat),
		PID:     r.pid,
		Level:   r.level,
		Source:  r.source,
		Payload: r.payload,
		Event:   r.event,
		Cat:     r.cat,
		Code:    r.code,
		Corr:    r.corr,
		DurMS:   r.durMS,
		Attempt: r.attempt,
		Caller:  r.caller,
	})
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

type writeItem struct {
	rec   record
	flush chan struct{} // 非 nil 表示 flush 屏障
}

type sink struct {
	dir string
	ch  chan writeItem

	mu         sync.Mutex // 保护 file/day/part/size/cleanupDay
	file       *os.File
	day        string
	part       int
	size       int64
	cleanupDay string
	wg         sync.WaitGroup

	dropped atomic.Uint64
	written atomic.Uint64
}

var sinkPtr atomic.Pointer[sink]

// InitFileSink 初始化文件 sink。dir 一般是 <dataDir>/logs。
// 幂等：重复调用直接返回 nil。sink 未初始化时所有写盘都是 no-op。
func InitFileSink(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return fmt.Errorf("logger: empty log dir")
	}
	if sinkPtr.Load() != nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	s := &sink{dir: dir, ch: make(chan writeItem, logQueueSize)}
	cleanupOldFiles(dir, time.Now())

	if !sinkPtr.CompareAndSwap(nil, s) {
		// 并发初始化，保留先到者
		return nil
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.run()
	}()

	// 丢弃计数定期上报（F-A3-20）：日志子系统自己出问题时，日志里必须看得到。
	startDropReporter(s, logDropReportInterval)
	return nil
}

func (s *sink) run() {
	for it := range s.ch {
		if it.flush != nil {
			// channel 是 FIFO 且只有一个消费者：收到屏障时之前的记录都已写完
			s.syncFile()
			close(it.flush)
			continue
		}
		if !s.writeRecord(it.rec) {
			s.dropped.Add(1)
		}
	}
}

// syncFile 把已写入的数据刷到磁盘。
func (s *sink) syncFile() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		_ = s.file.Sync()
	}
}

// writeRecord 写入一条记录；返回 false 表示这条记录没能落盘。
func (s *sink) writeRecord(rec record) bool {
	line, err := rec.marshal()
	if err != nil {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := rec.t
	if s.file == nil {
		if err := s.ensureFileLocked(now, false); err != nil {
			return false
		}
	}
	// 单文件上限：写不下就切分片
	if s.size > 0 && s.size+int64(len(line)) > logFileMaxBytes {
		if err := s.ensureFileLocked(now, true); err != nil {
			return false
		}
	}
	n, err := s.file.Write(line)
	if err != nil {
		return false
	}
	s.size += int64(n)
	s.written.Add(1)
	return true
}

// ensureFileLocked 保证当前打开的是「当天且未满」的文件。
// forceNew=true 时强制切到下一个分片。
func (s *sink) ensureFileLocked(now time.Time, forceNew bool) error {
	day := now.Format("20060102")

	if s.file != nil {
		if !forceNew && s.day == day && s.size < logFileMaxBytes {
			return nil
		}
		_ = s.file.Sync()
		_ = s.file.Close()
		s.file = nil
		if s.day != day {
			s.part = 0
		}
	}
	if s.day != day {
		s.day = day
		s.part = 0
	}
	if forceNew {
		s.part++
	}

	for {
		path := filepath.Join(s.dir, logFileName(day, s.part))
		st, statErr := os.Stat(path)
		if statErr != nil && !os.IsNotExist(statErr) {
			return statErr
		}
		if statErr == nil && st.Size() >= logFileMaxBytes {
			// 分片已满，继续下一个
			s.part++
			continue
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		s.file = f
		if statErr == nil {
			s.size = st.Size()
		} else {
			s.size = 0
		}
		break
	}

	if s.cleanupDay != day {
		s.cleanupDay = day
		cleanupOldFiles(s.dir, now)
	}
	return nil
}

func logFileName(day string, part int) string {
	if part <= 0 {
		return logFilePrefix + "-" + day + ".jsonl"
	}
	return fmt.Sprintf("%s-%s.%d.jsonl", logFilePrefix, day, part)
}

// parseDayFromName 从 app-YYYYMMDD.jsonl / app-YYYYMMDD.N.jsonl 解析日期。
func parseDayFromName(name string) (time.Time, bool) {
	rest, ok := strings.CutPrefix(name, logFilePrefix+"-")
	if !ok {
		return time.Time{}, false
	}
	rest, ok = strings.CutSuffix(rest, ".jsonl")
	if !ok {
		return time.Time{}, false
	}
	if i := strings.IndexByte(rest, '.'); i >= 0 {
		rest = rest[:i]
	}
	if len(rest) != 8 {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("20060102", rest, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// cleanupOldFiles 删除超过保留期的日志分片。
func cleanupOldFiles(dir string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := now.AddDate(0, 0, -logRetentionDays)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, logFilePrefix+"-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		path := filepath.Join(dir, name)
		if day, ok := parseDayFromName(name); ok {
			if day.Before(cutoff) {
				_ = os.Remove(path)
			}
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(path)
		}
	}
}

// enqueue 把一条记录投递给 sink（非阻塞）。
func (s *sink) enqueue(rec record) {
	select {
	case s.ch <- writeItem{rec: rec}:
	default:
		if rec.level == "debug" {
			// 队列满：debug 优先丢弃
			s.dropped.Add(1)
			return
		}
		// error/warn/info 退化为同步补写，宁可慢一点也不丢
		if !s.writeRecord(rec) {
			s.dropped.Add(1)
		}
	}
}

// enqueueToSink 是 RingBuffer.Add 的落盘挂钩；未初始化时 no-op。
func enqueueToSink(entry LogEntry) {
	s := sinkPtr.Load()
	if s == nil {
		return
	}
	// 优先沿用条目自带的 TS（NormalizeEntry 已补齐），保证内存环/前端/文件
	// 三处时间戳完全一致；解析失败才回落到当前时间。
	t := time.Now()
	if entry.TS != "" {
		if parsed, err := time.Parse(logTimeFormat, entry.TS); err == nil {
			t = parsed
		}
	}

	s.enqueue(record{
		t:       t,
		pid:     entry.PID,
		level:   NormalizeLevel(entry.Type),
		source:  string(entry.Source),
		payload: entry.Payload,
		event:   entry.Event,
		cat:     entry.Cat,
		code:    entry.Code,
		corr:    entry.Corr,
		durMS:   entry.DurMS,
		attempt: entry.Attempt,
		caller:  entry.Caller,
	})
}

// Flush 等待已入队的记录落盘并 Sync。进程正常退出前必须调用。
func Flush() {
	stdOut.flushTail()
	s := sinkPtr.Load()
	if s == nil {
		return
	}
	done := make(chan struct{})
	select {
	case s.ch <- writeItem{flush: done}:
	case <-time.After(logFlushTimeout):
		s.syncFile()
		return
	}
	select {
	case <-done:
	case <-time.After(logFlushTimeout):
	}
	s.syncFile()
}

// Stats 是日志子系统自身的健康计数（审计 A3 F-A3-20）。
//
// 说明：内存环的「覆盖旧条目」是设计行为，不算丢弃；这里只统计真正没落盘/没推出去的。
type Stats struct {
	Dropped      uint64 `json:"dropped"`      // 未落盘条数（队列满且 debug / 写失败）
	Written      uint64 `json:"written"`      // 成功落盘条数
	RingSize     int    `json:"ringSize"`     // 主环当前条数
	RingCapacity int    `json:"ringCapacity"` // 主环容量
	PinSize      int    `json:"pinSize"`      // warn/error 保留区当前条数
	PinCapacity  int    `json:"pinCapacity"`  // warn/error 保留区容量
}

// GetStats 返回当前日志子系统健康快照（廉价，无 IO）。
func GetStats() Stats {
	stats := Stats{
		RingSize:     AppLogs.Len(),
		RingCapacity: AppLogs.Capacity(),
		PinSize:      AppLogs.PinLen(),
		PinCapacity:  AppLogs.PinCapacity(),
	}
	if s := sinkPtr.Load(); s != nil {
		stats.Dropped = s.dropped.Load()
		stats.Written = s.written.Load()
	}
	return stats
}

// startDropReporter 周期性检查丢弃计数：有新增就写一条 warn 进日志（→ 环 + 文件），
// 让「日志系统自己丢失日志」不再是静默的（F-A3-20）。
func startDropReporter(s *sink, interval time.Duration) {
	if s == nil || interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		var last uint64
		for range ticker.C {
			n := s.dropped.Load()
			if n <= last {
				continue
			}
			delta := n - last
			last = n
			AppLogs.Add(LogEntry{
				Type:    "warn",
				Source:  LogSourceApp,
				Cat:     "log",
				Event:   "log.sink.dropped",
				Code:    "LOG_SINK_DROPPED",
				Attempt: int(delta),
				Payload: fmt.Sprintf("日志写盘丢弃 %d 条（累计 %d 条），请检查磁盘空间与写入速度", delta, n),
			})
		}
	}()
}

// Dropped 返回因队列满/写盘失败而未能落盘的记录数。
func Dropped() uint64 {
	if s := sinkPtr.Load(); s != nil {
		return s.dropped.Load()
	}
	return 0
}

// stdWriter 把标准库 log 的字节流按行收编成 JSONL 记录。
type stdWriter struct {
	mu  sync.Mutex
	buf []byte
}

var stdOut = &stdWriter{}

func (w *stdWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := make([]byte, i)
		copy(line, w.buf[:i])
		w.buf = w.buf[i+1:]
		w.emit(line)
	}
	if len(w.buf) > stdWriterLineLimit {
		line := make([]byte, len(w.buf))
		copy(line, w.buf)
		w.buf = w.buf[:0]
		w.emit(line)
	}
	return len(p), nil
}

func (w *stdWriter) emit(line []byte) {
	text := strings.TrimRight(string(line), "\r")
	if text == "" {
		return
	}
	s := sinkPtr.Load()
	if s == nil {
		return
	}
	s.enqueue(record{
		t:       time.Now(),
		pid:     logPID,
		level:   "info",
		source:  string(LogSourceStd),
		payload: text,
	})
}

func (w *stdWriter) flushTail() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) == 0 {
		return
	}
	line := make([]byte, len(w.buf))
	copy(line, w.buf)
	w.buf = w.buf[:0]
	w.emit(line)
}

// StdWriter 返回可交给 log.SetOutput 的 io.Writer：
// 配合 io.MultiWriter(os.Stderr, logger.StdWriter()) 可把标准库 log 一并落盘。
func StdWriter() io.Writer {
	return stdOut
}
