//go:build windows

package appcore

import (
	"context"
	"encoding/json"
	"goclashz/core/clash"
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

type TrafficMode string

const (
	TrafficModeCore  TrafficMode = "core"
	TrafficModeProxy TrafficMode = "proxy"
)

type connTrafficMark struct {
	Upload   int64
	Download int64
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

	// 👇 新增：流量统计模式与连接历史
	mode            TrafficMode
	prevConnTraffic map[string]connTrafficMark
}

func NewTrafficStreamManager(emit EventSink, getLogLevel func() string) *TrafficStreamManager {
	return &TrafficStreamManager{
		emit:            emit,
		getLogLevel:     getLogLevel,
		prevConnTraffic: make(map[string]connTrafficMark),
	}
}

func (m *TrafficStreamManager) Start(parent context.Context, apiURL string, proxyOnly bool) {
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

	if proxyOnly {
		m.mode = TrafficModeProxy
	} else {
		m.mode = TrafficModeCore
	}
	m.lastTick = time.Now()
	m.prevConnTraffic = make(map[string]connTrafficMark)
	m.mu.Unlock()

	if proxyOnly {
		go m.proxyTrafficLoop(ctx, currentGen)
	} else {
		go m.coreTrafficLoop(ctx, currentGen, apiURL)
	}
}

func (m *TrafficStreamManager) coreTrafficLoop(ctx context.Context, currentGen int, apiURL string) {
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
		alive := m.gen == currentGen && m.cancel != nil
		if !alive {
			m.mu.Unlock()
			return
		}

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
}

func (m *TrafficStreamManager) proxyTrafficLoop(ctx context.Context, currentGen int) {
	defer func() {
		m.mu.Lock()
		if m.gen == currentGen {
			m.cancel = nil
			m.emit.Emit("traffic-data", m.currentTrafficSnapshot(0, 0, "0 B/s", "0 B/s"))
		}
		m.mu.Unlock()
	}()

	// /connections 模式下，我们每秒采样一次
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.sampleProxyConnections(currentGen)
		}
	}
}

func (m *TrafficStreamManager) sampleProxyConnections(currentGen int) {
	raw, err := clash.GetConnectionsRaw()
	if err != nil {
		m.emitErrorLog(err)
		return
	}

	var payload struct {
		Connections []traffic.RawConnection `json:"connections"`
	}

	if err := json.Unmarshal(raw, &payload); err != nil {
		m.emitErrorLog(err)
		return
	}

	now := time.Now()

	m.mu.Lock()
	alive := m.gen == currentGen && m.cancel != nil
	if !alive {
		m.mu.Unlock()
		return
	}

	elapsed := now.Sub(m.lastTick).Seconds()
	if elapsed <= 0 {
		elapsed = 1
	}

	var upDelta int64
	var downDelta int64

	nextMarks := make(map[string]connTrafficMark, len(payload.Connections))

	for _, conn := range payload.Connections {
		if !isProxyConnection(conn) {
			continue
		}

		cur := connTrafficMark{
			Upload:   conn.Upload,
			Download: conn.Download,
		}

		nextMarks[conn.ID] = cur

		prev, ok := m.prevConnTraffic[conn.ID]
		if !ok {
			// 新连接，第一次采样只建立基线
			continue
		}

		if du := cur.Upload - prev.Upload; du > 0 {
			upDelta += du
		}

		if dd := cur.Download - prev.Download; dd > 0 {
			downDelta += dd
		}
	}

	m.prevConnTraffic = nextMarks

	if elapsed > 0 && elapsed < 10 {
		m.uploadTotalRaw += upDelta
		m.downloadTotalRaw += downDelta
	}

	m.lastTick = now

	upRaw := float64(upDelta) / elapsed
	downRaw := float64(downDelta) / elapsed

	snapshot := m.currentTrafficSnapshot(
		upRaw,
		downRaw,
		traffic.FormatBytes(int64(upRaw))+"/s",
		traffic.FormatBytes(int64(downRaw))+"/s",
	)

	m.mu.Unlock()

	m.emit.Emit("traffic-data", snapshot)
}

func isProxyConnection(conn traffic.RawConnection) bool {
	if len(conn.Chains) == 0 {
		return false
	}

	for _, chain := range conn.Chains {
		c := strings.ToUpper(strings.TrimSpace(chain))
		if c == "DIRECT" || c == "REJECT" || c == "REJECT-DROP" {
			return false
		}
	}

	rule := strings.ToUpper(strings.TrimSpace(conn.Rule))
	if rule == "DIRECT" || rule == "REJECT" || rule == "REJECT-DROP" {
		return false
	}

	return true
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

func (m *TrafficStreamManager) ResetRuntimeState() {
	m.mu.Lock()
	m.uploadTotalRaw = 0
	m.downloadTotalRaw = 0
	m.lastTick = time.Now()
	m.prevConnTraffic = make(map[string]connTrafficMark)
	m.mu.Unlock()

	m.emit.Emit("traffic-data", m.currentTrafficSnapshot(0, 0, "0 B/s", "0 B/s"))
}

func (m *TrafficStreamManager) Restart(parent context.Context, apiURL string, proxyOnly bool) {
	m.Stop()
	m.Start(parent, apiURL, proxyOnly)
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
