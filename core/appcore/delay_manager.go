package appcore

import (
	"context"
	"fmt"
	"goclashz/core/clash"
	"sync"
	"time"
)

var ErrDelayTestBusy = fmt.Errorf("DELAY_TEST_BUSY")

type DelayResult struct {
	Name   string
	Delay  int
	Status string
	Err    error
}

type DelayTestManager struct {
	mu      sync.Mutex
	running bool
	// 🚀 核心改进：记录当前正在跑批次的节点，用于单点请求时的复用
	batchNodes map[string]struct{}
	waiters    map[string][]chan DelayResult

	sem  chan struct{} // 并发控制器
	emit EventSink
	ctrl *Controller // 引用总控，用于启停内核
}

func NewDelayTestManager(emit EventSink, ctrl *Controller) *DelayTestManager {
	return &DelayTestManager{
		emit:       emit,
		ctrl:       ctrl,
		sem:        make(chan struct{}, 8), // 🚀 核心改进：并发从 32 降到 8，防止高频触发内核/网络瓶颈
		batchNodes: make(map[string]struct{}),
		waiters:    make(map[string][]chan DelayResult),
	}
}

// waitForBatchNode 如果节点正在批量测速中，则挂起当前请求等待批量结果
func (m *DelayTestManager) waitForBatchNode(ctx context.Context, name string) (DelayResult, bool) {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return DelayResult{}, false
	}

	if _, ok := m.batchNodes[name]; !ok {
		m.mu.Unlock()
		return DelayResult{}, false
	}

	ch := make(chan DelayResult, 1)
	m.waiters[name] = append(m.waiters[name], ch)
	m.mu.Unlock()

	select {
	case res := <-ch:
		return res, true
	case <-ctx.Done():
		return DelayResult{Name: name, Delay: 0, Status: "timeout", Err: ctx.Err()}, true
	}
}

// notifyNodeResult 通知所有正在等待该节点结果的协程
func (m *DelayTestManager) notifyNodeResult(res DelayResult) {
	m.mu.Lock()
	waiters := m.waiters[res.Name]
	delete(m.waiters, res.Name)
	m.mu.Unlock()

	for _, ch := range waiters {
		select {
		case ch <- res:
		default:
		}
		close(ch)
	}
}

// testOne 统一的底层测速执行函数，包含并发控制和超时管理
func (m *DelayTestManager) testOne(ctx context.Context, name string, testURL string, timeoutMs int) DelayResult {
	select {
	case m.sem <- struct{}{}:
		defer func() { <-m.sem }()
	case <-ctx.Done():
		return DelayResult{Name: name, Delay: 0, Status: "timeout", Err: ctx.Err()}
	}

	// 实际请求的 context 超时应略大于 mihomo timeout 参数
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs+1500)*time.Millisecond)
	defer cancel()

	delay, err := clash.GetProxyDelay(reqCtx, name, testURL, timeoutMs)
	if err != nil || delay <= 0 {
		return DelayResult{Name: name, Delay: 0, Status: "timeout", Err: err}
	}

	return DelayResult{Name: name, Delay: delay, Status: "success"}
}

func (m *DelayTestManager) TestAllProxies(ctx context.Context, nodeNames []string) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()

		for _, name := range nodeNames {
			m.emit.Emit("proxy-delay-update", map[string]interface{}{
				"name":   name,
				"delay":  0,
				"status": "busy",
			})
		}
		m.emit.Emit("proxy-test-finished", "当前已有测速任务正在运行")
		return
	}
	m.running = true
	m.batchNodes = make(map[string]struct{}, len(nodeNames))
	for _, name := range nodeNames {
		m.batchNodes[name] = struct{}{}
	}
	m.mu.Unlock()

	finishMsg := "测速完成"
	silentCore := false

	defer func() {
		m.mu.Lock()
		m.running = false
		m.batchNodes = make(map[string]struct{})
		m.mu.Unlock()

		if silentCore {
			m.ctrl.StopCoreProcess()
		}
		m.emit.Emit("proxy-test-finished", finishMsg)
	}()

	// 1. 如果内核未运行，由 m.ctrl.EnsureCoreRunning 静默拉起
	if !clash.IsRunning() {
		silentCore = true
		if err := m.ctrl.EnsureCoreRunning(ctx); err != nil {
			finishMsg = "测速启动失败：" + err.Error()
			return
		}
	}

	// 2. 如果未传节点，先从内核获取并补全
	if len(nodeNames) == 0 {
		if data, err := clash.GetInitialData(); err == nil {
			if groups, ok := data["groups"].(map[string]interface{}); ok {
				for name, raw := range groups {
					nm, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					typ, _ := nm["type"].(string)

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
	if netCfg, err := clash.GetNetworkConfig(); err == nil && netCfg.TestURL != "" {
		testUrl = netCfg.TestURL
	}

	// 3. 启动 Worker Pool 测速
	jobs := make(chan string, len(nodeNames))
	for _, name := range nodeNames {
		jobs <- name
	}
	close(jobs)

	workerCount := 8
	if len(nodeNames) < workerCount {
		workerCount = len(nodeNames)
	}

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for name := range jobs {
				m.emit.Emit("proxy-test-start", name)

				res := m.testOne(ctx, name, testUrl, 6000)

				m.emit.Emit("proxy-delay-update", map[string]interface{}{
					"name":   res.Name,
					"delay":  res.Delay,
					"status": res.Status,
				})

				m.notifyNodeResult(res)
			}
		}()
	}
	wg.Wait()
}

func (m *DelayTestManager) TestProxy(ctx context.Context, name string) (int, error) {
	if name == "" {
		return 0, fmt.Errorf("empty proxy name")
	}

	// 🛡️ 如果批量测速正在跑，并且这个节点在批量里，挂起等待批量结果
	if res, ok := m.waitForBatchNode(ctx, name); ok {
		if res.Err != nil || res.Delay <= 0 {
			return 0, fmt.Errorf("%s", res.Status)
		}
		return res.Delay, nil
	}

	// 🛡️ 如果批量测速正在跑，但这个节点不在批量里，直接返回 busy，不要额外打内核 API
	m.mu.Lock()
	running := m.running
	m.mu.Unlock()

	if running {
		return 0, ErrDelayTestBusy
	}

	testURL := "http://www.gstatic.com/generate_204"
	if netCfg, err := clash.GetNetworkConfig(); err == nil && netCfg.TestURL != "" {
		testURL = netCfg.TestURL
	}

	// 单独执行测速，使用稍长的超时
	res := m.testOne(ctx, name, testURL, 8000)

	if res.Err != nil || res.Delay <= 0 {
		return 0, fmt.Errorf("timeout")
	}

	return res.Delay, nil
}
