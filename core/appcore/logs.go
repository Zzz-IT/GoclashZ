//go:build windows

package appcore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"goclashz/core/clash"
	"goclashz/core/logger"
)

// 推 UI 的限速参数（审计 A3 F-A3-10 / F-A3-11）。
//
// 取舍：磁盘（JSONL）与内存环保留**全量**，只有「推前端」这一条通道做级别过滤 + 限速，
// 因为前端事件走的是 v3 的同一条串行 mailbox，core info/debug 洪水会把流量/连接等
// 实时事件挤在队列里（见 A3 审计 F-A3-10）。
const (
	// coreUIMaxPerSecond 是每秒最多推给前端的 core 日志条数，超出部分只落盘。
	coreUIMaxPerSecond = 120
	// coreUIWindow 是限速窗口长度。
	coreUIWindow = time.Second
	// coreUIQuietPeriod 是两条「抑制汇总」之间的最小间隔。
	coreUIQuietPeriod = 2 * time.Second
	// logStreamIdleAfter 是「多久没收到任何内核日志」判定为断流（F-A3-17）。
	logStreamIdleAfter = 60 * time.Second
	// logStreamHealthTick 是健康检查周期（同时用作 debug 级心跳）。
	logStreamHealthTick = 30 * time.Second
	// logStreamIdleWarnInterval 是两条断流告警之间的最小间隔。
	logStreamIdleWarnInterval = time.Minute
)

type LogStreamManager struct {
	mu         sync.Mutex
	cancel     context.CancelFunc
	gen        int
	running    bool
	dispatcher *LogDispatcher

	// minLevel 是 Behavior.LogLevel 的归一化值：低于它的 core 日志不推 UI（仍落盘）。
	minLevel string

	// 推 UI 的令牌窗口
	uiWindow    time.Time
	uiCount     int
	uiFiltered  int
	uiThrottled int

	// 累计统计（Stop 时汇总一次）
	totalFiltered  uint64
	totalThrottled uint64
	lastSummaryAt  time.Time

	// F-A3-17：断流检测（FetchLogs 在内部静默重连，外部只能靠「有没有新日志」判断）
	healthStop   chan struct{}
	lastEntryAt  time.Time
	lastIdleWarn time.Time
}

func NewLogStreamManager(dispatcher *LogDispatcher) *LogStreamManager {
	return &LogStreamManager{
		dispatcher: dispatcher,
	}
}

func (m *LogStreamManager) Start(ctx context.Context, logLevel string) {
	level := logger.NormalizeLevel(logLevel)
	if level == "" {
		level = "info"
	}

	m.mu.Lock()
	if m.cancel != nil {
		m.cancel() // 清理旧流
	}
	if m.healthStop != nil {
		close(m.healthStop) // 旧流的健康检查协程退出
		m.healthStop = nil
	}

	streamCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.gen++
	currentGen := m.gen
	started := time.Now()
	m.running = true
	m.minLevel = level
	m.uiWindow = time.Time{}
	m.uiCount = 0
	m.lastEntryAt = started
	m.lastIdleWarn = time.Time{}
	healthStop := make(chan struct{})
	m.healthStop = healthStop
	corr := fmt.Sprintf("core-logstream-%d", currentGen)
	m.mu.Unlock()

	go m.healthLoop(currentGen, healthStop, started)

	logger.Record(logger.NewEvent("core", "core.logstream.start").
		WithSource(logger.LogSourceCore).
		WithCorr(corr).
		WithPayload("内核日志流启动 level=%s gen=%d", level, currentGen))

	go func(gen int, corr string, started time.Time) {
		defer func() {
			m.mu.Lock()
			if m.gen == gen {
				m.cancel = nil
				m.running = false
			}
			m.mu.Unlock()

			logger.Record(logger.NewEvent("core", "core.logstream.stop").
				WithSource(logger.LogSourceCore).
				WithCorr(corr).
				WithDur(time.Since(started)).
				WithPayload("内核日志流结束 gen=%d", gen))
		}()

		clash.FetchLogs(streamCtx, level, func(data interface{}) {
			m.mu.Lock()
			// Generation Guard 帧级防并发污染
			alive := m.gen == gen && m.cancel != nil
			m.mu.Unlock()

			if !alive {
				return
			}

			mapData, ok := data.(map[string]interface{})
			if !ok {
				return
			}
			typ, _ := mapData["type"].(string)
			payload, _ := mapData["payload"].(string)

			m.mu.Lock()
			m.lastEntryAt = time.Now()
			m.mu.Unlock()

			if m.dispatcher == nil {
				return
			}

			// 先全量入环 + 落盘（可回放优先），再决定是否推 UI。
			entry := m.dispatcher.AddCoreStored(logger.LogEntry{
				Type:    typ,
				Payload: payload,
			})
			if m.allowCoreUI(entry.Type) {
				m.dispatcher.EmitLog(entry)
			}
		})
	}(currentGen, corr, started)
}

// healthLoop 是日志流的健康检查协程（F-A3-17）。
//
// FetchLogs 在内核未运行 / external-controller 不可达时只是每 2s 静默重连，
// 外部完全看不出「日志面板是空的」还是「日志流根本没连上」。这里用「多久没有新日志」
// 做判定，超过阈值就落一条 warn（环 + 文件 + 前端），并限频。
// 同时用 Debugf 打一条心跳，让 app 侧 debug 档不再是空轨道（F-A3-18）。
func (m *LogStreamManager) healthLoop(gen int, stop chan struct{}, started time.Time) {
	ticker := time.NewTicker(logStreamHealthTick)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case now := <-ticker.C:
			var idleEvent *logger.LogEntry
			var lastEntry time.Time
			var alive bool

			m.mu.Lock()
			alive = m.gen == gen
			if alive {
				lastEntry = m.lastEntryAt
				idle := lastEntry.IsZero() || now.Sub(lastEntry) >= logStreamIdleAfter
				canWarn := m.lastIdleWarn.IsZero() || now.Sub(m.lastIdleWarn) >= logStreamIdleWarnInterval
				if idle && canWarn {
					m.lastIdleWarn = now
					entry := logger.NewEvent("core", "core.logstream.idle").
						WithSource(logger.LogSourceCore).
						WithLevel("warn").
						WithCode("LOG_STREAM_IDLE").
						WithDur(now.Sub(started)).
						WithPayload("内核日志流已 %.0f 秒没有收到任何日志（内核未运行或 external-controller 不可达），正在静默重连",
							now.Sub(started).Seconds())
					idleEvent = &entry
				}
			}
			m.mu.Unlock()

			if !alive {
				return
			}
			if idleEvent != nil {
				// 落盘 + 推前端（EmitLog 只推不写，避免重复记录）。
				logger.Record(*idleEvent)
				if m.dispatcher != nil {
					m.dispatcher.EmitLog(*idleEvent)
				}
				continue
			}
			if lastEntry.IsZero() {
				logger.Debugf("日志流健康检查：gen=%d 尚未收到任何日志（已运行 %.0fs）", gen, now.Sub(started).Seconds())
			} else {
				logger.Debugf("日志流健康检查：gen=%d 最近日志 %.0fs 前", gen, now.Sub(lastEntry).Seconds())
			}
		}
	}
}

// allowCoreUI 返回这条 core 日志是否推给前端。
// 级别低于 Behavior.LogLevel 或超出每秒限额时返回 false，并计入抑制统计。
func (m *LogStreamManager) allowCoreUI(level string) bool {
	level = logger.NormalizeLevel(level)

	var summary *logger.LogEntry

	m.mu.Lock()
	allow := true
	if logger.LevelRank(level) < logger.LevelRank(m.minLevel) {
		m.uiFiltered++
		m.totalFiltered++
		allow = false
	} else if logger.LevelRank(level) >= logger.LevelRank("error") {
		// error 级永不被速率限制吞掉（宁可多推几条，也不让报错在 UI 上消失）。
	} else {
		now := time.Now()
		if m.uiWindow.IsZero() || now.Sub(m.uiWindow) >= coreUIWindow {
			m.uiWindow = now
			m.uiCount = 0
		}
		if m.uiCount >= coreUIMaxPerSecond {
			m.uiThrottled++
			m.totalThrottled++
			allow = false
		} else {
			m.uiCount++
		}
	}
	summary = m.takeSummaryLocked(time.Now(), coreUIQuietPeriod)
	m.mu.Unlock()

	// 汇总条目的写环/落盘/推送都在锁外做，避免持锁调用事件通道。
	if summary != nil && m.dispatcher != nil {
		m.dispatcher.AddCore(*summary)
	}
	return allow
}

// takeSummaryLocked 取出「抑制汇总」条目（如果有），并重置窗口计数。
func (m *LogStreamManager) takeSummaryLocked(now time.Time, quiet time.Duration) *logger.LogEntry {
	if m.uiFiltered == 0 && m.uiThrottled == 0 {
		return nil
	}
	if quiet > 0 && !m.lastSummaryAt.IsZero() && now.Sub(m.lastSummaryAt) < quiet {
		return nil
	}

	filtered, throttled := m.uiFiltered, m.uiThrottled
	m.uiFiltered, m.uiThrottled = 0, 0
	m.lastSummaryAt = now

	entry := logger.NewEvent("log", "log.ui.suppressed").
		WithSource(logger.LogSourceCore).
		WithLevel("warn").
		WithCode("LOG_UI_SUPPRESSED").
		WithPayload("core 日志推 UI 已抑制 %d 条（级别过滤 %d / 速率限制 %d），完整内容见 JSONL 日志",
			filtered+throttled, filtered, throttled)
	return &entry
}

func (m *LogStreamManager) Stop() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
		m.gen++
		m.running = false
	}
	if m.healthStop != nil {
		close(m.healthStop)
		m.healthStop = nil
	}
	summary := m.takeSummaryLocked(time.Now(), 0)
	total := m.totalFiltered + m.totalThrottled
	m.mu.Unlock()

	if summary != nil && m.dispatcher != nil {
		m.dispatcher.AddCore(*summary)
	}
	if total > 0 {
		logger.Record(logger.NewEvent("log", "log.ui.suppressed.total").
			WithSource(logger.LogSourceCore).
			WithCode("LOG_UI_SUPPRESSED").
			WithAttempt(int(total)).
			WithPayload("本次日志流累计抑制 %d 条 core UI 日志", total))
	}
}

func (m *LogStreamManager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}
