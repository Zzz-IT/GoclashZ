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
		sem:        make(chan struct{}, 6), // 🚀 核心改进：并发进一步降到 6，保障弱网环境成功率
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
func (m *DelayTestManager) testOne(ctx context.Context, name string, testURL string, timeoutMs int, contextExtraMs int) DelayResult {
	select {
	case m.sem <- struct{}{}:
		defer func() { <-m.sem }()
	case <-ctx.Done():
		return DelayResult{Name: name, Delay: 0, Status: "timeout", Err: ctx.Err()}
	}

	// 实际请求的 context 超时应略大于 mihomo timeout 参数
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs+contextExtraMs)*time.Millisecond)
	defer cancel()

	delay, err := clash.GetProxyDelay(reqCtx, name, testURL, timeoutMs)
	if err != nil || delay <= 0 {
		return DelayResult{Name: name, Delay: 0, Status: "timeout", Err: err}
	}

	return DelayResult{Name: name, Delay: delay, Status: "success"}
}

// testOneWithFallback 带备用地址的测速逻辑，极大提升复杂网络环境下的成功率
func (m *DelayTestManager) testOneWithFallback(ctx context.Context, name string, testURLs []string, timeoutMs int, contextExtraMs int) DelayResult {
	var last DelayResult
	for _, testURL := range testURLs {
		res := m.testOne(ctx, name, testURL, timeoutMs, contextExtraMs)
		if res.Status == "success" {
			return res
		}
		last = res
	}

	if last.Name == "" {
		last = DelayResult{Name: name, Delay: 0, Status: "timeout", Err: fmt.Errorf("all test urls failed")}
	}
	return last
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
	m.mu.Unlock()

	finishMsg := "测速完成"
	silentCore := false

	defer func() {
		m.mu.Lock()
		m.running = false
		m.batchNodes = make(map[string]struct{})
		m.waiters = make(map[string][]chan DelayResult) // 🛡️ 核心修复：清理等待者，防止内存泄漏
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
		nodeNames = m.extractDelayTargets()
	}

	if len(nodeNames) == 0 {
		finishMsg = "没有可测速节点"
		return
	}

	// 🚀 核心修复：节点提取/补全后，再写入 batchNodes，确保单点测速能正确进入等待逻辑
	m.mu.Lock()
	m.batchNodes = make(map[string]struct{}, len(nodeNames))
	for _, name := range nodeNames {
		m.batchNodes[name] = struct{}{}
	}
	m.mu.Unlock()

	m.runBatch(ctx, nodeNames)
}

func (m *DelayTestManager) extractDelayTargets() []string {
	var nodeNames []string
	data, err := clash.GetInitialData()
	if err != nil {
		return nodeNames
	}

	groups, ok := data["groups"].(map[string]interface{})
	if !ok {
		return nodeNames
	}

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
	return nodeNames
}

func (m *DelayTestManager) getTestURLs() []string {
	if netCfg, err := clash.GetNetworkConfig(); err == nil && netCfg.TestURL != "" {
		return []string{netCfg.TestURL}
	}

	// 🛡️ 增加 Fallback 列表，解决部分测速地址被墙导致的批量超时
	return []string{
		"https://cp.cloudflare.com/generate_204",
		"https://www.gstatic.com/generate_204",
		"http://www.msftconnecttest.com/connecttest.txt",
	}
}

func (m *DelayTestManager) runBatch(ctx context.Context, nodeNames []string) {
	testURLs := m.getTestURLs()

	jobs := make(chan string, len(nodeNames))
	for _, name := range nodeNames {
		jobs <- name
	}
	close(jobs)

	workerCount := 6
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

				// 批量测速：7s 超时，3s Context 宽限
				res := m.testOneWithFallback(ctx, name, testURLs, 7000, 3000)

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

	testURLs := m.getTestURLs()
	// 单点测试：10s 超时，2s Context 宽限，提升单点稳定性
	res := m.testOneWithFallback(ctx, name, testURLs, 10000, 2000)

	if res.Err != nil || res.Delay <= 0 {
		return 0, fmt.Errorf("timeout")
	}

	return res.Delay, nil
}
