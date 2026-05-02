package appcore

import (
	"context"
	"goclashz/core/clash"
	"sync"
	"time"
)

type DelayTestManager struct {
	mu          sync.Mutex
	activeTests int
	semaphore   chan struct{}
	silentCore  bool // 标记是否为了测速而临时静默拉起的内核
	emit        EventSink
	ctrl        *Controller // 引用总控，用于启停内核
}

func NewDelayTestManager(emit EventSink, ctrl *Controller) *DelayTestManager {
	return &DelayTestManager{
		semaphore: make(chan struct{}, 64),
		emit:      emit,
		ctrl:      ctrl,
	}
}

func (m *DelayTestManager) TestAllProxies(ctx context.Context, nodeNames []string) {
	m.mu.Lock()
	if m.activeTests > 0 {
		m.mu.Unlock()
		return
	}
	m.activeTests = 1
	m.mu.Unlock()

	finishMsg := "测速完成"
	defer func() {
		m.mu.Lock()
		m.activeTests = 0
		m.mu.Unlock()
		m.emit.Emit("proxy-test-finished", finishMsg)
	}()

	// 1. 如果内核未运行，由 m.ctrl.EnsureCoreRunning 静默拉起并标记 m.silentCore = true
	if !clash.IsRunning() {
		m.mu.Lock()
		m.silentCore = true
		m.mu.Unlock()

		if err := m.ctrl.EnsureCoreRunning(ctx); err != nil {
			finishMsg = "测速启动失败：" + err.Error()
			return
		}
	} else {
		m.mu.Lock()
		m.silentCore = false
		m.mu.Unlock()
	}

	// 2. 如果未传节点，先从内核获取并补全
	if len(nodeNames) == 0 {
		if data, err := clash.GetInitialData(); err == nil {
			// 🛡️ 核心修复：获取节点的 key 必须是 "groups"，且类型是 map[string]interface{}
			if groups, ok := data["groups"].(map[string]interface{}); ok {
				for name, raw := range groups {
					nm, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					typ, _ := nm["type"].(string)

					// 排除策略组和内置节点，只测真实节点
					switch typ {
					case "Selector", "URLTest", "Fallback", "LoadBalance":
						continue
					}
					if name == "GLOBAL" || name == "DIRECT" || name == "REJECT" {
						continue
					}
					nodeNames = append(nodeNames, name)
				}
			}
		}
	}

	if len(nodeNames) == 0 {
		finishMsg = "没有可测速节点"
		return
	}

	testUrl := "http://www.gstatic.com/generate_204"
	// 获取用户自定义测速链接
	if netCfg, err := clash.GetNetworkConfig(); err == nil && netCfg.TestURL != "" {
		testUrl = netCfg.TestURL
	}

	// 3. 启动 Worker 并发测速
	var wg sync.WaitGroup
	for _, name := range nodeNames {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			m.semaphore <- struct{}{}
			defer func() { <-m.semaphore }()

			m.emit.Emit("proxy-test-start", n)

			reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			delay, err := clash.GetProxyDelay(reqCtx, n, testUrl)
			cancel()

			status := "success"
			if err != nil || delay <= 0 {
				status = "timeout"
				delay = 0
			}

			m.emit.Emit("proxy-delay-update", map[string]interface{}{
				"name":   n,
				"delay":  delay,
				"status": status,
			})
		}(name)
	}
	wg.Wait()

	// 4. 测速完毕后，如果 m.silentCore == true，由 m.ctrl.StopCoreProcess() 关闭
	m.mu.Lock()
	isSilent := m.silentCore
	m.mu.Unlock()

	if isSilent {
		m.ctrl.StopCoreProcess()
	}
}

func (m *DelayTestManager) TestProxy(name string) (int, error) {
	return clash.TestProxy(name)
}
