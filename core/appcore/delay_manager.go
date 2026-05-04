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

type DelaySource string

const (
	DelaySourceManual    DelaySource = "manual"
	DelaySourceAuto      DelaySource = "auto"
	DelaySourceStartup   DelaySource = "startup"
	DelaySourceScheduled DelaySource = "scheduled"
	DelaySourceEnabled   DelaySource = "enabled"
	DelaySourceRestore   DelaySource = "restore"
)

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

// DelayTestOptions 测速策略配置
type DelayTestOptions struct {
	Source         DelaySource   // 测速来源
	SilentUI       bool          // 为 true 时，不发送 proxy-test-start/finished
	RetryFailed    bool          // 失败后是否补测 (仅限深度/后台模式)
	TotalTimeout   time.Duration // 测速总超时
	StopSilentCore bool          // 结束后是否尝试关闭内核

	ProbeTimeout time.Duration // 单次测速 Mihomo 超时
	ProbeExtra   time.Duration // Context 宽限时长
	Concurrency  int           // 并发数
}

func manualDelayOptions() DelayTestOptions {
	return DelayTestOptions{
		Source:         DelaySourceManual,
		SilentUI:       false,
		RetryFailed:    false,
		TotalTimeout:   45 * time.Second,
		StopSilentCore: true,
		ProbeTimeout:   4 * time.Second,
		ProbeExtra:     1 * time.Second,
		Concurrency:    10,
	}
}

func autoDelayOptions(source DelaySource) DelayTestOptions {
	return DelayTestOptions{
		Source:         source,
		SilentUI:       true,
		RetryFailed:    false,
		TotalTimeout:   60 * time.Second,
		StopSilentCore: true,
		ProbeTimeout:   3500 * time.Millisecond,
		ProbeExtra:     800 * time.Millisecond,
		Concurrency:    6,
	}
}

func singleDelayOptions() DelayTestOptions {
	return DelayTestOptions{
		Source:         DelaySourceManual,
		SilentUI:       false,
		ProbeTimeout:   5 * time.Second,
		ProbeExtra:     1 * time.Second,
		Concurrency:    1,
	}
}

type DelayTestManager struct {
	mu    sync.Mutex
	state DelayRunState

	batchSource DelaySource
	batchNodes  map[string]struct{}
	waiters     map[string][]chan DelayResult

	batchCancel context.CancelFunc
	batchDone   chan struct{}

	activeSingles map[string]struct{}

	sem  chan struct{}
	emit EventSink
	ctrl *Controller
}

func NewDelayTestManager(emit EventSink, ctrl *Controller) *DelayTestManager {
	return &DelayTestManager{
		emit:          emit,
		ctrl:          ctrl,
		state:         DelayIdle,
		sem:           make(chan struct{}, 15), // 提高信号量容量以匹配并发
		batchNodes:    make(map[string]struct{}),
		waiters:       make(map[string][]chan DelayResult),
		activeSingles: make(map[string]struct{}),
	}
}

func (m *DelayTestManager) cancelAutoBatchAndWait(ctx context.Context, maxWait time.Duration) bool {
	m.mu.Lock()
	// 如果不是自动任务在跑，或者没任务，直接返回
	if m.state == DelayIdle || m.batchSource == DelaySourceManual || m.batchCancel == nil || m.batchDone == nil {
		m.mu.Unlock()
		return true
	}

	cancel := m.batchCancel
	done := m.batchDone
	m.mu.Unlock()

	cancel()

	timer := time.NewTimer(maxWait)
	defer timer.Stop()

	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	case <-ctx.Done():
		return false
	}
}

func (m *DelayTestManager) waitForManualBatchNode(ctx context.Context, name string) (DelayResult, bool) {
	m.mu.Lock()
	// 只有手动批量测速才允许挂起等待
	if m.state != DelayRunning || m.batchSource != DelaySourceManual {
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

func (m *DelayTestManager) testOneDuration(
	ctx context.Context,
	name string,
	testURL string,
	timeout time.Duration,
	extra time.Duration,
) DelayResult {
	select {
	case m.sem <- struct{}{}:
		defer func() { <-m.sem }()
	case <-ctx.Done():
		return DelayResult{Name: name, Delay: 0, Status: "timeout", Err: ctx.Err()}
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout+extra)
	defer cancel()

	timeoutMs := int(timeout / time.Millisecond)

	delay, err := clash.GetProxyDelay(reqCtx, name, testURL, timeoutMs)
	if err != nil || delay <= 0 {
		return DelayResult{Name: name, Delay: 0, Status: "timeout", Err: err}
	}

	return DelayResult{Name: name, Delay: delay, Status: "success"}
}

func (m *DelayTestManager) emitDelayResult(topo *DelayTopology, res DelayResult) {
	m.emit.Emit("proxy-delay-update", map[string]interface{}{
		"name":   res.Name,
		"delay":  res.Delay,
		"status": res.Status,
		"source": "leaf",
	})

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
	// 用户手动测速，优先取消自动测速
	m.cancelAutoBatchAndWait(ctx, 500*time.Millisecond)

	m.TestAllProxiesWithOptions(ctx, nodeNames, manualDelayOptions())
}

func (m *DelayTestManager) TestAllProxiesAuto(ctx context.Context, source string) {
	m.TestAllProxiesWithOptions(ctx, nil, autoDelayOptions(DelaySource(source)))
}

func (m *DelayTestManager) TestAllProxiesWithOptions(
	parent context.Context,
	nodeNames []string,
	opts DelayTestOptions,
) {
	if opts.TotalTimeout <= 0 {
		opts.TotalTimeout = 45 * time.Second
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 10
	}

	ctx, cancel := context.WithTimeout(parent, opts.TotalTimeout)
	done := make(chan struct{})

	m.mu.Lock()
	if m.state != DelayIdle || len(m.activeSingles) > 0 {
		m.mu.Unlock()
		cancel()
		close(done)

		if !opts.SilentUI {
			for _, name := range nodeNames {
				m.emit.Emit("proxy-delay-update", map[string]interface{}{
					"name":   name,
					"delay":  0,
					"status": "busy",
				})
			}
			m.emit.Emit("proxy-test-finished", "当前已有测速任务正在运行")
		}
		return
	}

	m.state = DelayPreparing
	m.batchSource = opts.Source
	m.batchCancel = cancel
	m.batchDone = done
	m.mu.Unlock()

	finishMsg := "测速完成"
	silentCore := false

	defer func() {
		cancel()

		m.mu.Lock()
		m.state = DelayIdle
		m.batchSource = ""
		m.batchCancel = nil
		m.batchDone = nil
		m.batchNodes = make(map[string]struct{})
		m.waiters = make(map[string][]chan DelayResult)
		m.mu.Unlock()

		if silentCore && opts.StopSilentCore {
			m.ctrl.StopCoreProcessIfIdle()
		}

		if !opts.SilentUI {
			m.emit.Emit("proxy-test-finished", finishMsg)
		}

		close(done)
	}()

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

	m.runBatch(ctx, topo, targets, opts)
}

func (m *DelayTestManager) getTestURL() string {
	if netCfg, err := clash.GetNetworkConfig(); err == nil && netCfg != nil {
		if u := strings.TrimSpace(netCfg.TestURL); u != "" {
			return u
		}
	}
	return clash.DefaultDelayTestURL
}

func (m *DelayTestManager) runBatch(
	ctx context.Context,
	topo *DelayTopology,
	nodeNames []string,
	opts DelayTestOptions,
) {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 10
	}

	testURL := m.getTestURL()
	jobs := make(chan string)
	results := make(chan DelayResult)

	workerCount := opts.Concurrency
	if len(nodeNames) < workerCount {
		workerCount = len(nodeNames)
	}

	var wg sync.WaitGroup

	// Worker 协程池
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for name := range jobs {
				select {
				case <-ctx.Done():
					results <- DelayResult{Name: name, Delay: 0, Status: "timeout", Err: ctx.Err()}
					continue
				default:
				}

				if !opts.SilentUI {
					m.emit.Emit("proxy-test-start", name)
				}

				res := m.testOneDuration(
					ctx,
					name,
					testURL,
					opts.ProbeTimeout,
					opts.ProbeExtra,
				)
				results <- res
			}
		}()
	}

	// 任务分发
	go func() {
		defer close(jobs)
		for _, name := range nodeNames {
			select {
			case <-ctx.Done():
				return
			case jobs <- name:
			}
		}
	}()

	// 等待完成并关闭结果通道
	go func() {
		wg.Wait()
		close(results)
	}()

	var failed []string

	// 收集并即时分发结果
	for res := range results {
		if res.Status != "success" && opts.RetryFailed {
			failed = append(failed, res.Name)
			continue
		}

		m.emitDelayResult(topo, res)
		m.notifyNodeResult(res)
	}

	if !opts.RetryFailed || len(failed) == 0 {
		return
	}

	// 仅深度/后台模式下的重试
	for _, name := range failed {
		select {
		case <-ctx.Done():
			return
		default:
			res := m.testOneDuration(
				ctx,
				name,
				testURL,
				7*time.Second,
				2*time.Second,
			)
			m.emitDelayResult(topo, res)
			m.notifyNodeResult(res)
		}
	}
}

func (m *DelayTestManager) TestProxy(ctx context.Context, name string) (int, error) {
	if name == "" {
		return 0, fmt.Errorf("empty proxy name")
	}

	// 用户单点优先，自动测速让路
	m.cancelAutoBatchAndWait(ctx, 300*time.Millisecond)

	topo, err := buildDelayTopology()
	if err != nil {
		return 0, err
	}

	target := name
	if node, ok := topo.Nodes[name]; ok && node.IsGroup {
		leaf := topo.resolveSelectedLeaf(name, map[string]bool{})
		if leaf == "" {
			return 0, fmt.Errorf("group has no selectable leaf")
		}
		target = leaf
	}

	// 只等待手动批量测速的结果
	if res, ok := m.waitForManualBatchNode(ctx, target); ok {
		if res.Err != nil || res.Delay <= 0 {
			return 0, fmt.Errorf("%s", res.Status)
		}
		m.emitDelayResult(topo, res)
		return res.Delay, nil
	}

	m.mu.Lock()
	isManualBatchBusy := m.state != DelayIdle
	m.mu.Unlock()

	if isManualBatchBusy {
		return 0, ErrDelayTestBusy
	}

	if !m.beginSingleNode(target) {
		return 0, ErrDelayTestBusy
	}
	defer m.endSingleNode(target)

	opts := singleDelayOptions()
	res := m.testOneDuration(ctx, target, m.getTestURL(), opts.ProbeTimeout, opts.ProbeExtra)

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
