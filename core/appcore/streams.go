//go:build windows

package appcore

import (
	"context"
	"goclashz/core/logger"
	"goclashz/core/traffic"
	"strings"
	"sync"
	"time"
)

type TrafficSnapshot struct {
	Up               string  `json:"up"`
	Down             string  `json:"down"`
	UpRaw            float64 `json:"upRaw"`
	DownRaw          float64 `json:"downRaw"`
	UploadTotal      string  `json:"uploadTotal"`
	DownloadTotal    string  `json:"downloadTotal"`
	UploadTotalRaw   int64   `json:"uploadTotalRaw"`
	DownloadTotalRaw int64   `json:"downloadTotalRaw"`
}

type TrafficStreamManager struct {
	mu          sync.Mutex
	cancel      context.CancelFunc
	gen         int
	emit        EventSink
	getLogLevel func() string

	lastErrAt  time.Time
	lastErrMsg string

	uploadTotalRaw   int64
	downloadTotalRaw int64
	lastTick         time.Time
}

func NewTrafficStreamManager(emit EventSink, getLogLevel func() string) *TrafficStreamManager {
	return &TrafficStreamManager{
		emit:        emit,
		getLogLevel: getLogLevel,
	}
}

func (m *TrafficStreamManager) Start(parent context.Context, apiURL string) {
	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return
	}

	m.gen++
	currentGen := m.gen

	ctx, cancel := context.WithCancel(parent)
	m.cancel = cancel
	m.lastTick = time.Now()
	m.mu.Unlock()

	go func(currentGen int) {
		defer func() {
			m.mu.Lock()
			if m.gen == currentGen {
				m.cancel = nil
				m.emit.Emit("traffic-data", m.currentTrafficSnapshot(0, 0, "0 B/s", "0 B/s"))
			}
			m.mu.Unlock()
		}()

		traffic.StreamTraffic(ctx, apiURL, func(upRaw, downRaw float64, upStr, downStr string) {
			m.mu.Lock()
			// 如果 generation 已变，或者取消函数为空，说明当前是个僵尸流
			alive := m.gen == currentGen && m.cancel != nil
			if !alive {
				m.mu.Unlock()
				return
			}

			// 计算累计值 (按时间积分更精准)
			now := time.Now()
			elapsed := now.Sub(m.lastTick).Seconds()

			if elapsed > 0 && elapsed < 10 {
				m.uploadTotalRaw += int64(upRaw * elapsed)
				m.downloadTotalRaw += int64(downRaw * elapsed)
			}

			m.lastTick = now

			snapshot := m.currentTrafficSnapshot(upRaw, downRaw, upStr+"/s", downStr+"/s")
			m.mu.Unlock()

			m.emit.Emit("traffic-data", snapshot)
		}, func(err error) {
			m.mu.Lock()
			alive := m.gen == currentGen && m.cancel != nil
			m.mu.Unlock()

			if !alive {
				return
			}

			m.emitErrorLog(err)
		})
	}(currentGen)
}

func (m *TrafficStreamManager) currentTrafficSnapshot(upRaw, downRaw float64, upStr, downStr string) TrafficSnapshot {
	return TrafficSnapshot{
		Up:               upStr,
		Down:             downStr,
		UpRaw:            upRaw,
		DownRaw:          downRaw,
		UploadTotal:      traffic.FormatBytes(m.uploadTotalRaw),
		DownloadTotal:    traffic.FormatBytes(m.downloadTotalRaw),
		UploadTotalRaw:   m.uploadTotalRaw,
		DownloadTotalRaw: m.downloadTotalRaw,
	}
}

func (m *TrafficStreamManager) Stop() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
		m.gen++
	}
	m.mu.Unlock()

	m.emit.Emit("traffic-data", m.currentTrafficSnapshot(0, 0, "0 B/s", "0 B/s"))
}

func (m *TrafficStreamManager) ResetTrafficTotals() {
	m.mu.Lock()
	m.uploadTotalRaw = 0
	m.downloadTotalRaw = 0
	m.lastTick = time.Now()
	m.mu.Unlock()

	m.emit.Emit("traffic-data", m.currentTrafficSnapshot(0, 0, "0 B/s", "0 B/s"))
}

func (m *TrafficStreamManager) Restart(parent context.Context, apiURL string) {
	m.Stop()
	m.Start(parent, apiURL)
}

func (m *TrafficStreamManager) emitErrorLog(err error) {
	if err == nil {
		return
	}

	if m.getLogLevel != nil && !shouldEmitLog(m.getLogLevel(), "error") {
		return
	}

	msg := err.Error()

	m.mu.Lock()
	if msg == m.lastErrMsg && time.Since(m.lastErrAt) < 10*time.Second {
		m.mu.Unlock()
		return
	}
	m.lastErrMsg = msg
	m.lastErrAt = time.Now()
	m.mu.Unlock()

	entry := logger.LogEntry{
		Type:    "error",
		Payload: "Traffic stream error: " + msg,
		Time:    time.Now().Format("15:04:05"),
	}

	logger.AppLogs.Add(entry)

	if m.emit != nil {
		m.emit.Emit("log-message", entry)
	}
}

func shouldEmitLog(configLevel, entryLevel string) bool {
	rank := map[string]int{
		"debug":   0,
		"info":    1,
		"warning": 2,
		"warn":    2,
		"error":   3,
	}

	cfg, ok := rank[strings.ToLower(configLevel)]
	if !ok {
		cfg = rank["info"]
	}

	ent, ok := rank[strings.ToLower(entryLevel)]
	if !ok {
		ent = rank["info"]
	}

	return ent >= cfg
}
