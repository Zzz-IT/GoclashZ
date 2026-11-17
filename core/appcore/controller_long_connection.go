//go:build windows

package appcore

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

var defaultSensitiveHosts = []string{
	"overleaf.com",
	"google.com",
	"gemini.google.com",
	"aistudio.google.com",
	"openai.com",
	"chatgpt.com",
	"claude.ai",
	"notion.so",
	"figma.com",
}

type LongConnectionSnapshot struct {
	HasActiveLongConnection bool     `json:"hasActiveLongConnection"`
	Count                   int      `json:"count"`
	Hosts                   []string `json:"hosts"`
	MaxDurationSeconds      int64    `json:"maxDurationSeconds"`
}

type ErrLongConnectionActive struct {
	Reason string
	Count  int
	Hosts  []string
}

func (e ErrLongConnectionActive) Error() string {
	return fmt.Sprintf("由于长连接活动而延迟 %s (检测到 %d 个连接: %s)", e.Reason, e.Count, strings.Join(e.Hosts, ", "))
}

type PendingDisruptiveAction struct {
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"createdAt"`
	Deadline  time.Time `json:"deadline"`
}

func (c *Controller) GetLongConnectionSnapshot() LongConnectionSnapshot {
	snap := LongConnectionSnapshot{}

	conns, err := c.GetConnections()
	if err != nil || len(conns.Connections) == 0 {
		return snap
	}

	behavior := c.Behavior.Get()
	minSeconds := int64(behavior.LongConnectionMinSeconds)
	if minSeconds <= 0 {
		minSeconds = 60
	}

	now := time.Now()
	for _, conn := range conns.Connections {
		startTime, err := time.Parse(time.RFC3339, conn.Start)
		if err != nil {
			continue
		}
		duration := int64(now.Sub(startTime).Seconds())
		if duration >= minSeconds && strings.ToLower(conn.Metadata.Network) == "tcp" {
			// Check sensitive hosts
			isSensitive := false
			for _, host := range defaultSensitiveHosts {
				if strings.Contains(strings.ToLower(conn.Metadata.Host), host) || strings.Contains(strings.ToLower(conn.Metadata.DestinationIP), host) {
					isSensitive = true
					break
				}
			}

			// Or active traffic (Download > 0 or Upload > 0 recently)
			// Wait, we don't have recent traffic, just total. We'll rely on duration + sensitive hosts + non-DIRECT chains
			hasProxyChain := false
			for _, chain := range conn.Chains {
				if chain != "DIRECT" && chain != "REJECT" && chain != "GLOBAL" {
					hasProxyChain = true
					break
				}
			}

			if hasProxyChain && (isSensitive || conn.Upload > 1024 || conn.Download > 1024) {
				snap.HasActiveLongConnection = true
				snap.Count++
				snap.Hosts = append(snap.Hosts, conn.Metadata.Host)
				if duration > snap.MaxDurationSeconds {
					snap.MaxDurationSeconds = duration
				}
			}
		}
	}

	// deduplicate hosts
	uniqueHosts := make(map[string]bool)
	var finalHosts []string
	for _, h := range snap.Hosts {
		if !uniqueHosts[h] && h != "" {
			uniqueHosts[h] = true
			finalHosts = append(finalHosts, h)
		}
	}
	snap.Hosts = finalHosts

	return snap
}

// userInitiatedReasons 用户直接触发的操作，不应被长连接保护阻断
var userInitiatedReasons = map[string]bool{
	"manual":              true,
	"tun-toggle":          true,
	"system-proxy-toggle": true,
	"config-switch":       true,
	"config-save":         true,
}

func (c *Controller) ShouldDeferDisruptiveAction(reason string) (bool, LongConnectionSnapshot) {
	// 用户直接触发的操作立即执行，不延迟
	if userInitiatedReasons[reason] {
		return false, LongConnectionSnapshot{}
	}

	snap := c.GetLongConnectionSnapshot()
	return snap.HasActiveLongConnection, snap
}

func (c *Controller) guardDisruptiveAction(reason string) error {
	behavior := c.Behavior.Get()
	if !behavior.LongConnectionProtection || !behavior.DeferRestartWhenActive {
		return nil
	}

	deferIt, snap := c.ShouldDeferDisruptiveAction(reason)
	if !deferIt {
		return nil
	}

	return ErrLongConnectionActive{
		Reason: reason,
		Count:  snap.Count,
		Hosts:  snap.Hosts,
	}
}

func (c *Controller) HasActiveLongConnection() bool {
	snap := c.GetLongConnectionSnapshot()
	return snap.HasActiveLongConnection
}

// deferredActionQueued 保证同一时刻只有一个"待执行的延迟动作"（F-A2-18）。
// 历史实现每次失败都起一个 10s ticker goroutine（watchdog 每 5s 触发 → 30 分钟内可累计数百个），
// 且长连接结束时它们会各自调用一次 RestartCore，形成重复重启。
var deferredActionQueued atomic.Bool

func (c *Controller) queuePendingDisruptiveAction(reason string) {
	if !deferredActionQueued.CompareAndSwap(false, true) {
		c.logInfo("已有待执行的延迟任务，本次不再重复排队 (%s)", reason)
		return
	}

	// Simple background poller for pending disruptive action
	action := PendingDisruptiveAction{
		Reason:    reason,
		CreatedAt: time.Now(),
		Deadline:  time.Now().Add(30 * time.Minute),
	}

	// F-A4-14：排队时也给出稳定 code（此前只有"放弃/恢复"两条通知，排队阶段是静默的）。
	c.notify("info", "DEFERRED_RESTART_QUEUED",
		fmt.Sprintf("检测到长连接活动，已排队延迟执行 (%s)，长连接结束后将自动继续", action.Reason),
		"deferred-action")

	baseCtx := c.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}

	go func() {
		defer deferredActionQueued.Store(false)

		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-baseCtx.Done():
				return
			case now := <-ticker.C:
				if now.After(action.Deadline) {
					c.notify("error", "DEFERRED_RESTART_ABANDONED", fmt.Sprintf("由于长连接一直存在，已放弃延迟执行的重启任务 (%s)", action.Reason), "deferred-action")
					return
				}

				if !c.HasActiveLongConnection() {
					c.notify("success", "DEFERRED_RESTART_RESUMED", fmt.Sprintf("长连接已结束，正在执行延迟的任务 (%s)", action.Reason), "deferred-action")
					_ = c.RestartCoreWithReason(c.ctx, action.Reason)
					return
				}
			}
		}
	}()
}
