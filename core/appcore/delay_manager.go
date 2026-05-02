package appcore

import (
	"context"
	"goclashz/core/clash"
	"sync"
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

	defer func() {
		m.mu.Lock()
		m.activeTests = 0
		m.mu.Unlock()
		m.emit.Emit("proxy-test-finished", nil)
	}()

	// 1. 如果内核未运行，由 m.ctrl.EnsureCoreRunning 静默拉起并标记 m.silentCore = true
	if !clash.IsRunning() {
		m.mu.Lock()
		m.silentCore = true
		m.mu.Unlock()

		if err := m.ctrl.EnsureCoreRunning(ctx); err != nil {
			m.emit.Emit("proxy-test-finished", err.Error())
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
			if proxies, ok := data["groups"].([]interface{}); ok {
				for _, p := range proxies {
					if pm, ok := p.(map[string]interface{}); ok {
						if name, ok := pm["name"].(string); ok {
							nodeNames = append(nodeNames, name)
						}
					}
				}
			}
		}
	}

	if len(nodeNames) == 0 {
		return
	}

	// 3. 启动 Worker 并发测速
	var wg sync.WaitGroup
	for _, name := range nodeNames {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			m.semaphore <- struct{}{}
			defer func() { <-m.semaphore }()
			_, _ = clash.TestProxy(n)
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
