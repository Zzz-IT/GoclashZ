//go:build windows

package appcore

import (
	"context"
	"fmt"
	"goclashz/core/clash"
	"strings"
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

// ProxyNodeMeta 代理节点元数据，用于拓扑解析
type ProxyNodeMeta struct {
	Name    string
	Type    string
	Now     string
	All     []string
	IsGroup bool
}

// DelayTopology 测速拓扑结构，用于归一化目标与结果分发
type DelayTopology struct {
	Nodes                map[string]ProxyNodeMeta
	SelectedLeafByGroup  map[string]string   // 策略组 -> 当前选中的叶子节点
	GroupsBySelectedLeaf map[string][]string // 叶子节点 -> 选中该叶子的策略组列表
}

func isProxyGroupType(t string) bool {
	t = strings.ToLower(t)
	switch t {
	case "selector", "urltest", "fallback", "loadbalance":
		return true
	default:
		return false
	}
}

func isSystemProxyName(name string) bool {
	name = strings.ToUpper(name)
	switch name {
	case "GLOBAL", "DIRECT", "REJECT":
		return true
	default:
		return false
	}
}

type DelayRunState string

const (
	DelayIdle      DelayRunState = "idle"
	DelayPreparing DelayRunState = "preparing"
	DelayRunning   DelayRunState = "running"
)

type DelayTestManager struct {
	mu    sync.Mutex
	state DelayRunState // 🚀 状态机：idle, preparing, running

	batchNodes map[string]struct{}
	waiters    map[string][]chan DelayResult

	activeSingles map[string]struct{} // 🚀 跟踪当前正在单点测速的节点，允许不同节点并发

	sem  chan struct{}
	emit EventSink
	ctrl *Controller
}

func NewDelayTestManager(emit EventSink, ctrl *Controller) *DelayTestManager {
	return &DelayTestManager{
		emit:          emit,
		ctrl:          ctrl,
		state:         DelayIdle,
		sem:           make(chan struct{}, 6),
		batchNodes:    make(map[string]struct{}),
		waiters:       make(map[string][]chan DelayResult),
		activeSingles: make(map[string]struct{}),
	}
}

// waitForBatchNode 如果节点正在批量测速中，则挂起当前请求等待批量结果
func (m *DelayTestManager) waitForBatchNode(ctx context.Context, name string) (DelayResult, bool) {
	m.mu.Lock()
	if m.state != DelayRunning {
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

// emitDelayResult 核心改进：不仅发叶子节点结果，还分发给受影响的策略组
func (m *DelayTestManager) emitDelayResult(topo *DelayTopology, res DelayResult) {
	// 1. 发送叶子节点自己的更新
	m.emit.Emit("proxy-delay-update", map[string]interface{}{
		"name":   res.Name,
		"delay":  res.Delay,
		"status": res.Status,
		"source": "leaf",
	})

	// 2. 发送派生更新给选中该节点的策略组
	if topo != nil {
		for _, groupName := range topo.GroupsBySelectedLeaf[res.Name] {
			m.emit.Emit("proxy-delay-update", map[string]interface{}{
				"name":   groupName,
				"delay":  res.Delay,
				"status": res.Status,
				"source": "group-derived",
				"from":   res.Name,
			})
		}
	}
}

func (m *DelayTestManager) beginSingleNode(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.activeSingles[name]; ok {
		return false
	}

	m.activeSingles[name] = struct{}{}
	return true
}

func (m *DelayTestManager) endSingleNode(name string) {
	m.mu.Lock()
	delete(m.activeSingles, name)
	m.mu.Unlock()
}


func (m *DelayTestManager) TestAllProxies(ctx context.Context, nodeNames []string) {
	m.mu.Lock()
	// 🛡️ 核心修复：单点测速正在跑时，也禁止批量进入，防止 API 冲突
	if m.state != DelayIdle || len(m.activeSingles) > 0 {
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
	m.state = DelayPreparing
	m.mu.Unlock()

	finishMsg := "测速完成"
	silentCore := false

	defer func() {
		m.mu.Lock()
		m.state = DelayIdle
		m.batchNodes = make(map[string]struct{})
		m.waiters = make(map[string][]chan DelayResult)
		m.mu.Unlock()

		if silentCore {
			m.ctrl.StopCoreProcess()
		}

		m.emit.Emit("proxy-test-finished", finishMsg)
	}()

	// 1. 🛡️ 核心修复：必须先确保内核运行，再构建拓扑（否则无法读取 API）
	if !clash.IsRunning() {
		silentCore = true
		if err := m.ctrl.EnsureCoreRunning(ctx); err != nil {
			finishMsg = "测速启动失败：" + err.Error()
			return
		}
	}

	topo, err := buildDelayTopology()
	if err != nil {
		finishMsg = "读取代理拓扑失败：" + err.Error()
		return
	}

	// 🛡️ 核心改进：拓扑归一化
	var targets []string
	if len(nodeNames) == 0 {
		targets = topo.allLeafNodes()
	} else {
		targets = topo.normalizeTargets(nodeNames)
	}

	if len(targets) == 0 {
		finishMsg = "没有可测速的有效节点"
		return
	}

	m.mu.Lock()
	m.state = DelayRunning
	m.batchNodes = make(map[string]struct{})
	for _, n := range targets {
		m.batchNodes[n] = struct{}{}
	}
	m.mu.Unlock()

	m.runBatch(ctx, topo, targets)
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

func (m *DelayTestManager) getTestURL() string {
	if netCfg, err := clash.GetNetworkConfig(); err == nil && netCfg != nil {
		if u := strings.TrimSpace(netCfg.TestURL); u != "" {
			return u
		}
	}
	return clash.DefaultDelayTestURL
}

func (m *DelayTestManager) runBatch(ctx context.Context, topo *DelayTopology, nodeNames []string) {
	testURL := m.getTestURL()

	jobs := make(chan string, len(nodeNames))
	for _, name := range nodeNames {
		jobs <- name
	}
	close(jobs)

	workerCount := 6
	if len(nodeNames) < workerCount {
		workerCount = len(nodeNames)
	}

	var failedMu sync.Mutex
	var failed []string

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for name := range jobs {
				m.emit.Emit("proxy-test-start", name)

				// 第一轮：并发 6
				res := m.testOne(ctx, name, testURL, 7000, 3000)

				if res.Status != "success" {
					failedMu.Lock()
					failed = append(failed, name)
					failedMu.Unlock()
				} else {
					m.emitDelayResult(topo, res)
					m.notifyNodeResult(res)
				}
			}
		}()
	}
	wg.Wait()

	// 🛡️ 第二轮：针对第一轮超时的节点进行“低并发补测”，提高长尾成功率
	if len(failed) > 0 {
		for _, name := range failed {
			select {
			case <-ctx.Done():
				return
			default:
				// 第二轮：并发 1 (串行) 补测，给予更宽松的 9s+3s 超时
				res := m.testOne(ctx, name, testURL, 9000, 3000)
				m.emitDelayResult(topo, res)
				m.notifyNodeResult(res)
			}
		}
	}
}

func (m *DelayTestManager) TestProxy(ctx context.Context, name string) (int, error) {
	if name == "" {
		return 0, fmt.Errorf("empty proxy name")
	}

	topo, err := buildDelayTopology()
	if err != nil {
		return 0, err
	}

	// 🛡️ 核心修复：单点归一化
	// 如果测的是策略组，实际上去测该组当前选中的真实叶子
	target := name
	if node, ok := topo.Nodes[name]; ok && node.IsGroup {
		leaf := topo.resolveSelectedLeaf(name, map[string]bool{})
		if leaf == "" {
			return 0, fmt.Errorf("group has no selectable leaf")
		}
		target = leaf
	}

	// 🛡️ 归一化后，针对真实叶子检查批量状态
	if res, ok := m.waitForBatchNode(ctx, target); ok {
		if res.Err != nil || res.Delay <= 0 {
			return 0, fmt.Errorf("%s", res.Status)
		}
		m.emitDelayResult(topo, res) // 分发同步
		return res.Delay, nil
	}

	// 🛡️ 针对真实叶子进行锁检查
	m.mu.Lock()
	isBusy := m.state != DelayIdle
	m.mu.Unlock()

	if isBusy {
		return 0, ErrDelayTestBusy
	}

	if !m.beginSingleNode(target) {
		return 0, ErrDelayTestBusy
	}
	defer m.endSingleNode(target)

	testURL := m.getTestURL()
	// 单点测试：10s 超时，2s Context 宽限
	res := m.testOne(ctx, target, testURL, 10000, 2000)

	// 分发结果（包含派生组更新）
	m.emitDelayResult(topo, res)

	if res.Err != nil || res.Delay <= 0 {
		return 0, fmt.Errorf("timeout")
	}

	return res.Delay, nil
}

// --- Topology Helpers ---

func buildDelayTopology() (*DelayTopology, error) {
	data, err := clash.GetInitialData()
	if err != nil {
		return nil, err
	}

	rawGroups, ok := data["groups"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid groups data")
	}

	t := &DelayTopology{
		Nodes:                make(map[string]ProxyNodeMeta),
		SelectedLeafByGroup:  make(map[string]string),
		GroupsBySelectedLeaf: make(map[string][]string),
	}

	for name, raw := range rawGroups {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		typ, _ := m["type"].(string)
		now, _ := m["now"].(string)

		var all []string
		if arr, ok := m["all"].([]interface{}); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok {
					all = append(all, s)
				}
			}
		}

		t.Nodes[name] = ProxyNodeMeta{
			Name:    name,
			Type:    typ,
			Now:     now,
			All:     all,
			IsGroup: isProxyGroupType(typ),
		}
	}

	for name, node := range t.Nodes {
		if !node.IsGroup {
			continue
		}

		leaf := t.resolveSelectedLeaf(name, map[string]bool{})
		if leaf == "" {
			continue
		}

		t.SelectedLeafByGroup[name] = leaf
		t.GroupsBySelectedLeaf[leaf] = append(t.GroupsBySelectedLeaf[leaf], name)
	}

	return t, nil
}

func (t *DelayTopology) resolveSelectedLeaf(name string, seen map[string]bool) string {
	if name == "" || seen[name] {
		return ""
	}
	seen[name] = true

	node, ok := t.Nodes[name]
	if !ok {
		return name // 叶子节点
	}

	if !node.IsGroup {
		return name
	}

	if node.Now != "" {
		return t.resolveSelectedLeaf(node.Now, seen)
	}

	for _, child := range node.All {
		if leaf := t.resolveSelectedLeaf(child, seen); leaf != "" {
			return leaf
		}
	}
	return ""
}

func (t *DelayTopology) normalizeTargets(input []string) []string {
	seen := make(map[string]struct{})
	var out []string

	for _, name := range input {
		if name == "" || isSystemProxyName(name) {
			continue
		}

		node, exists := t.Nodes[name]
		if !exists {
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				out = append(out, name)
			}
			continue
		}

		if !node.IsGroup {
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				out = append(out, name)
			}
			continue
		}

		leaf := t.resolveSelectedLeaf(name, map[string]bool{})
		if leaf == "" || isSystemProxyName(leaf) {
			continue
		}

		if _, ok := seen[leaf]; !ok {
			seen[leaf] = struct{}{}
			out = append(out, leaf)
		}
	}
	return out
}

func (t *DelayTopology) allLeafNodes() []string {
	seen := make(map[string]struct{})
	var out []string

	for name, node := range t.Nodes {
		if node.IsGroup || isSystemProxyName(name) {
			continue
		}
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	return out
}
