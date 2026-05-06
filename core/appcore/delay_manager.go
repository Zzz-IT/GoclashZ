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

const (
	// 单点测速硬保护超时 (14s)，防止 UI 无限等待
	SingleOuterTimeout = 14 * time.Second
	// 单点排队超时 (1s)，并发槽位满时快速返回 busy
	SingleQueueTimeout = 1 * time.Second
	// 单点测速 API 超时 (8s)
	SingleDelayTimeout = 8 * time.Second
	// 单点 Context 宽限时长
	SingleCtxGrace = 800 * time.Millisecond

	// 冷启动预热探测超时
	ColdStartProbeTimeout = 2500 * time.Millisecond
	// 冷启动重试超时
	ColdStartRetryTimeout = 6500 * time.Millisecond
	// 冷启动重试宽限
	ColdStartRetryGrace = 600 * time.Millisecond

	// 批量测速 API 超时
	ManualBatchDelayTimeout = 8000
	AutoBatchDelayTimeout   = 5000
)

type DelayResult struct {
	Name    string
	Delay   int
	Status  string
	Err     error
	Message string
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
		ProbeTimeout:   time.Duration(ManualBatchDelayTimeout) * time.Millisecond,
		ProbeExtra:     800 * time.Millisecond,
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
		ProbeTimeout:   time.Duration(AutoBatchDelayTimeout) * time.Millisecond,
		ProbeExtra:     800 * time.Millisecond,
		Concurrency:    6,
	}
}

func singleDelayOptions() DelayTestOptions {
	return DelayTestOptions{
		Source:       DelaySourceManual,
		SilentUI:     false,
		ProbeTimeout: SingleDelayTimeout,
		ProbeExtra:   SingleCtxGrace,
		Concurrency:  1,
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
		activeSingles: make(map[string]struct{}),
		sem:           make(chan struct{}, 10),
		batchNodes:    make(map[string]struct{}),
		waiters:       make(map[string][]chan DelayResult),
	}
}

func (m *DelayTestManager) acquireDelaySlot(ctx context.Context, queueTimeout time.Duration) error {
	if queueTimeout <= 0 {
		select {
		case m.sem <- struct{}{}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	timer := time.NewTimer(queueTimeout)
	defer timer.Stop()

	select {
	case m.sem <- struct{}{}:
		return nil
	case <-timer.C:
		return ErrDelayTestBusy
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *DelayTestManager) releaseDelaySlot() {
	select {
	case <-m.sem:
	default:
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

func classifyDelayError(err error) string {
	if err == nil {
		return "success"
	}

	msg := strings.ToLower(err.Error())

	// 1. 超时分类
	if strings.Contains(msg, "deadline") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "context canceled") {
		return "timeout"
	}

	// 2. 网络连接/协议分类
	if strings.Contains(msg, "tls") ||
		strings.Contains(msg, "handshake") ||
		strings.Contains(msg, "connect error") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") {
		return "connect-error"
	}

	// 3. 其他错误 (如 HTTP 非 200, 格式错误等)
	return "test-error"
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
		return DelayResult{Name: name, Delay: 0, Status: "timeout", Err: ctx.Err(), Message: "任务取消"}
	}

	return m.testOneDurationRaw(ctx, name, testURL, timeout, extra)
}

func (m *DelayTestManager) testOneDurationRaw(
	ctx context.Context,
	name string,
	testURL string,
	timeout time.Duration,
	extra time.Duration,
) DelayResult {
	reqCtx, cancel := context.WithTimeout(ctx, timeout+extra)
	defer cancel()

	timeoutMs := int(timeout / time.Millisecond)

	delay, err := clash.GetProxyDelay(reqCtx, name, testURL, timeoutMs)

	status := classifyDelayError(err)
	msg := ""
	if err != nil {
		msg = err.Error()
	}

	if err != nil || delay <= 0 {
		return DelayResult{
			Name:    name,
			Delay:   0,
			Status:  status,
			Err:     err,
			Message: msg,
		}
	}

	return DelayResult{
		Name:    name,
		Delay:   delay,
		Status:  "success",
		Message: "",
	}
}

func (m *DelayTestManager) emitDelayResult(topo *DelayTopology, res DelayResult) {
	m.emit.Emit("proxy-delay-update", map[string]interface{}{
		"name":    res.Name,
		"delay":   res.Delay,
		"status":  res.Status,
		"message": res.Message,
		"source":  "leaf",
	})

	if topo != nil {
		for _, groupName := range topo.GroupsBySelectedLeaf[res.Name] {
			m.emit.Emit("proxy-delay-update", map[string]interface{}{
				"name":    groupName,
				"delay":   res.Delay,
				"status":  res.Status,
				"message": res.Message,
				"source":  "group-derived",
				"from":    res.Name,
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
	if m.state != DelayIdle {
		m.mu.Unlock()
		cancel()
		close(done)

		if !opts.SilentUI {
			m.emit.Emit("proxy-test-finished", ErrDelayTestBusy.Error())
		}
		return
	}

	m.state = DelayRunning
	m.batchSource = opts.Source
	m.batchCancel = cancel
	m.batchDone = done
	m.batchNodes = make(map[string]struct{})
	m.waiters = make(map[string][]chan DelayResult)
	m.mu.Unlock()

	finishMsg := "测速完成"

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

		if !opts.SilentUI {
			m.emit.Emit("proxy-test-finished", finishMsg)
		}

		close(done)
	}()

	// 🚀 核心接入：静默内核保障
	cleanup, _, err := m.ctrl.EnsureDelayCore(ctx)
	if err != nil {
		finishMsg = "测速启动失败：" + err.Error()
		return
	}
	defer cleanup()

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

	// 🚀 核心接入：静默内核保障
	cleanup, warmupRequired, err := m.ctrl.EnsureDelayCore(ctx)
	if err != nil {
		return 0, err
	}
	defer cleanup()

	topo, err := buildDelayTopology()
	if err != nil {
		return 0, err
	}

	// 解析出叶子节点
	target := name
	if node, ok := topo.Nodes[name]; ok && node.IsGroup {
		leaf := topo.resolveSelectedLeaf(name, map[string]bool{})
		if leaf == "" {
			return 0, fmt.Errorf("group has no selectable leaf")
		}
		target = leaf
	}

	// 1. 同节点防重复并发
	if !m.beginSingleNode(target) {
		return 0, ErrDelayTestBusy
	}
	defer m.endSingleNode(target)

	// 2. 信号量排队 (1s 超时)
	if err := m.acquireDelaySlot(ctx, SingleQueueTimeout); err != nil {
		return 0, err
	}
	defer m.releaseDelaySlot()

	// 3. 执行测速
	testURL := m.getTestURL()
	var res DelayResult

	if warmupRequired {
		// 冷启动预热探测：成功就直接用，失败不立即 emit，避免 UI 闪超时
		res = m.testOneDurationRaw(ctx, target, testURL, ColdStartProbeTimeout, ColdStartRetryGrace)

		if (res.Err != nil || res.Delay <= 0) && isRetryableDelayFailure(res) {
			time.Sleep(200 * time.Millisecond)
			res = m.testOneDurationRaw(ctx, target, testURL, ColdStartRetryTimeout, ColdStartRetryGrace)
		}
	} else {
		res = m.testOneDurationRaw(ctx, target, testURL, SingleDelayTimeout, SingleCtxGrace)
	}

	m.emitDelayResult(topo, res)

	if res.Err != nil || res.Delay <= 0 {
		return 0, res.Err
	}

	return res.Delay, nil
}

func isRetryableDelayFailure(res DelayResult) bool {
	if res.Delay > 0 {
		return false
	}

	status := strings.ToLower(strings.TrimSpace(res.Status))
	if status == "timeout" || status == "connect-error" || status == "test-error" {
		return true
	}

	if res.Err == nil {
		return false
	}

	msg := strings.ToLower(res.Err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline") ||
		strings.Contains(msg, "tls") ||
		strings.Contains(msg, "connect")
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
