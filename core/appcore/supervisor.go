//go:build windows

package appcore

import (
	"context"
	"errors"
	"fmt"
	"goclashz/core/clash"
	"goclashz/core/logger"
	"goclashz/core/runtimeassets"
	"sync"
	"time"
)

// 哨兵错误
var (
	ErrNoActiveConfig        = errors.New("no active config selected")
	ErrHelperInstallRequired = errors.New("helper_install_required")
	ErrHelperRepairRequired  = errors.New("helper_repair_required")
	ErrTunNeedHelperOrAdmin  = errors.New("tun_need_helper_or_admin")
	ErrWintunMissing         = errors.New("wintun_missing")
)

type CoreSupervisor struct {
	controller *Controller

	ctx    context.Context
	cancel context.CancelFunc

	desired *DesiredStateStore

	reconcileCh  chan reconcileRequest
	watchdogTick *time.Ticker

	// mu 只保护生命周期字段（ctx/cancel/watchdogTick），必须短持有。
	mu sync.Mutex
	// reconcileSem 串行化调和过程本身（容量 1 的信号量）。
	// 历史用 sync.Mutex：watchdog 会无上限排队，拿到锁时 8s ctx 早已过期，
	// 于是一次"本该跳过"的心跳变成串行失败 + 噪声日志（F-A2-17）。
	// 改成信号量后：普通调和按 ctx 预算等待，watchdog 用 TryReconcile 直接跳过。
	reconcileSem chan struct{}
}

// ErrReconcileBusy 表示另一个调和/事务正在执行；调用方应视为"本轮跳过"。
var ErrReconcileBusy = errors.New("reconcile busy: 另一个调和或重事务正在执行")

type reconcileRequest struct {
	Reason string
	Ctx    context.Context
}

type RuntimePlan struct {
	NeedCore        bool
	NeedTun         bool
	NeedSysProxy    bool
	ActiveConfig    string
	Mode            string
	RestartRequired bool
	Reason          string
}

func NewCoreSupervisor(c *Controller, d *DesiredStateStore) *CoreSupervisor {
	return &CoreSupervisor{
		reconcileSem: make(chan struct{}, 1),
		controller:   c,
		desired:      d,
		reconcileCh:  make(chan reconcileRequest, 10),
	}
}

func (s *CoreSupervisor) Start(ctx context.Context) {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.watchdogTick = time.NewTicker(5 * time.Second)
	s.mu.Unlock()

	go s.loop()
}

func (s *CoreSupervisor) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.watchdogTick != nil {
		s.watchdogTick.Stop()
	}
}

// ReconcileAsync 异步触发 reconcile，使用独立 timeout ctx。
// 预算必须覆盖"内核启动 + API 就绪 + TUN 适配器等待(20s)"，
// 否则异步调和会把正常的 TUN 冷启动截断成失败（历史上就是这么弹窗的）。
func (s *CoreSupervisor) ReconcileAsync(reason string) {
	// F-A2-19：Start 之前 s.ctx 为 nil、Stop 之后是已取消的 ctx。
	// 两种情况都不能拿来做 WithTimeout 的父 ctx（nil 会 panic，已取消会让请求立刻作废）。
	s.mu.Lock()
	base := s.ctx
	s.mu.Unlock()
	if base == nil || base.Err() != nil {
		logger.Warnf("ReconcileAsync(%s) 被忽略：supervisor 未启动或已停止", reason)
		return
	}

	ctx, cancel := context.WithTimeout(base, 45*time.Second)
	req := reconcileRequest{Reason: reason, Ctx: ctx}
	select {
	case s.reconcileCh <- req:
		// 取消交给 context 超时（45s）处理
		_ = cancel
	default:
		cancel()
	}
}

func (s *CoreSupervisor) loop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case req := <-s.reconcileCh:
			if err := s.Reconcile(req.Ctx, req.Reason); err != nil {
				logger.Warnf("Reconcile(%s) failed: %v", req.Reason, err)
			}
		case <-s.watchdogTick.C:
			// 有配置切换 / 内核重启等重事务在执行时跳过周期性调和，
			// 避免事务中途被并行调和打断（事务自身会收敛到目标状态）。
			if s.controller.controls != nil && s.controller.controls.Transitioning() {
				continue
			}
			desired := s.desired.Get()
			// 检查任意一个需要内核的条件，包括端口代理模式（PortProxy）
			// 原来只检查 CoreRunning，导致端口代理模式下内核崩溃后不会自动重启
			if desired.CoreRunning || desired.SystemProxy || desired.Tun || desired.PortProxy {
				ctx, cancel := context.WithTimeout(s.ctx, 8*time.Second)
				ran, err := s.TryReconcile(ctx, "watchdog")
				if !ran {
					// 已有调和/重事务在执行：本轮直接跳过，不排队、不刷日志（F-A2-17）。
					cancel()
					if desired.Tun {
						s.controller.repairTunActualIfNeeded()
					}
					continue
				}
				if err != nil {
					logger.Warnf("Reconcile(watchdog) failed: %v", err)
				}
				cancel()
			}
			// TUN 实际状态修复：内核以 TUN 模式在跑但 ActualTun 仍为 false 时，
			// 只做一次只读适配器探测校正（不重启内核、不清理任何系统资源）。
			// 若长时间仍未就绪，由静默阶梯升级（L1 有界等待 → L2 重启一次 → L3 提示）。
			if desired.Tun {
				s.controller.repairTunActualIfNeeded()
				s.controller.escalateTunReadinessIfNeeded()
			} else {
				s.controller.resetTunLadder()
			}
		}
	}
}

// Reconcile 执行状态调和，使用传入的操作级 ctx。
// 只持有 reconcileSem（串行化调和），不持有生命周期锁 mu：
// 内核重启 / API 等待 / TUN 适配器轮询等长耗时步骤不会阻塞 Start/Stop。
func (s *CoreSupervisor) Reconcile(ctx context.Context, reason string) error {
	if err := s.acquireReconcile(ctx); err != nil {
		return err
	}
	defer s.releaseReconcile()

	return s.reconcileLocked(ctx, reason)
}

// TryReconcile 非阻塞尝试一次调和：已有调和在执行时立即返回 (false, nil)。
// watchdog 用它，避免在 reconcileSem 上排队直到 ctx 过期（F-A2-17）。
func (s *CoreSupervisor) TryReconcile(ctx context.Context, reason string) (bool, error) {
	select {
	case s.reconcileSem <- struct{}{}:
	default:
		return false, nil
	}
	defer s.releaseReconcile()

	return true, s.reconcileLocked(ctx, reason)
}

// acquireReconcile 在 ctx 预算内获取调和权；ctx 到期时返回 ErrReconcileBusy。
func (s *CoreSupervisor) acquireReconcile(ctx context.Context) error {
	select {
	case s.reconcileSem <- struct{}{}:
		return nil
	default:
	}

	select {
	case s.reconcileSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", ErrReconcileBusy, ctx.Err())
	}
}

func (s *CoreSupervisor) releaseReconcile() { <-s.reconcileSem }

// reconcileLocked 是调和的实现体；调用方必须已持有 reconcileSem。
func (s *CoreSupervisor) reconcileLocked(ctx context.Context, reason string) error {
	desired := s.desired.Get()
	behavior := s.controller.Behavior.Get()

	if desired.ActiveConfig == "" {
		desired.ActiveConfig = behavior.ActiveConfig
		desired.Mode = behavior.ActiveMode
	}

	if desired.Tun {
		// 快检优先（F-A2-01）：watchdog 每 5s 都会走到这里，不能把
		// 95MB 哈希 + clash.exe -v 挂在心跳上。只有快检明确不可用（缺失/损坏）时，
		// 才触发全量检查 + seed 修复——修复分支本来就需要全量结论。
		fastStatus := runtimeassets.GetStatusFast(ctx)
		if fastAssetBlocked(fastStatus.Assets[runtimeassets.AssetWintun]) ||
			fastAssetBlocked(fastStatus.Assets[runtimeassets.AssetCore]) {
			if status, err := runtimeassets.EnsureReady(ctx, runtimeassets.RequireTun, runtimeassets.RepairInvalid); err != nil {
				wintun := status.Assets[runtimeassets.AssetWintun]
				core := status.Assets[runtimeassets.AssetCore]

				// 只有「wintun.dll 确实不存在的文件」才值得阻断 TUN。
				// 历史上这里把任何 EnsureReady 失败都当成 ErrWintunMissing，
				// 导致 wintun 明明已存在并在用（只是自检/复制这一步失败）也弹"缺少 Wintun 驱动"。
				trulyMissing := !wintun.Ready && wintun.ErrorCode == runtimeassets.ErrMissing

				if trulyMissing && !clash.IsRunning() {
					s.controller.setLastErrorCode("ASSET_WINTUN_MISSING", "Wintun 不可用: "+err.Error())
					s.controller.SyncState()
					return ErrWintunMissing
				}

				// 其余情况（文件在但校验被占用/种子库缺失/内核已在运行）：只告警，
				// 让真实适配器探测来决定 TUN 到底有没有起来。
				logger.Warnf("运行时资产检查未通过（不阻断 TUN，交由适配器探测裁决）: %v (coreReady=%v wintunReady=%v wintunErr=%s)",
					err, core.Ready, wintun.Ready, wintun.Error)
			}
		}
	}

	plan := s.buildRuntimePlan(desired, behavior, reason)

	if !plan.NeedCore {
		// 用户关闭了代理/TUN：停内核后向 helper 确认停止（运行期路径）。
		s.controller.disableAllWithConfirmation(ctx)
		return nil
	}

	if err := s.controller.reconcileCoreProcess(ctx, plan); err != nil {
		s.controller.setLastErrorCode("CORE_START_FAILED", err.Error())
		s.controller.SyncState()
		return err
	}

	// Commit userCoreRunning（以及「不再需要 TUN 时」的 ActualTun 归零）
	// immediately after the core process is confirmed healthy, so that any
	// SyncState call emitted during reconcileSystemProxy (or its error path)
	// carries correct runtime fields rather than stale intermediate values.
	// ⚠️ 这里不再把 desired.Tun 直接写成 ActualTun（原问题根因），
	// ActualTun 必须由下面的真实适配器探测决定。
	s.controller.setRuntimeStateFromPlan(plan)

	// TUN：内核进程健康 ≠ TUN 可用。以真实适配器探测结果为唯一依据；
	// 失败则置错误状态并保留实际值（不会因为 desired.Tun 而伪造 true）。
	// 这里只做单次确认：新启动的内核已在 ensureCoreRunningWithDesiredState
	// 内完成过轮询等待，避免 watchdog 每 5s 再触发一次 8s 长等待。
	if plan.NeedTun {
		if err := s.controller.confirmTunActual(); err != nil {
			// 只记录日志、不弹错误通知：watchdog 每 5s 就会调和一次，
			// 反复通知会刷屏；ActualTun 已经如实为 false，UI 可见真实状态。
			logger.Warnf("TUN 适配器未就绪（ActualTun 保持实际值）: %v", err)
			s.controller.SyncState()
			return err
		}
	}

	if err := s.controller.reconcileSystemProxy(plan); err != nil {
		s.controller.setLastErrorCode("SYSTEM_PROXY_RECONCILE_FAILED", err.Error())
		s.controller.SyncState()
		return err
	}

	s.controller.SyncState()
	return nil
}

func (s *CoreSupervisor) ShutdownRuntime(reason string) {
	// 串行化到调和锁：避免与正在进行的 Reconcile 互相穿插；
	// 不持有生命周期锁 mu，关闭流程不会被长时间阻塞。
	// 与普通调和互斥；这里用 Background 预算（关闭流程本身不能被调用方 ctx 截断）。
	if err := s.acquireReconcile(context.Background()); err != nil {
		logger.Warnf("ShutdownRuntime(%s) 等待调和锁失败: %v", reason, err)
	}
	defer s.releaseReconcile()
	s.controller.DisableAll()
}

func (s *CoreSupervisor) buildRuntimePlan(desired DesiredState, _ AppBehavior, reason string) RuntimePlan {
	plan := RuntimePlan{
		NeedCore:     desired.CoreRunning || desired.SystemProxy || desired.Tun || desired.PortProxy,
		NeedTun:      desired.Tun,
		NeedSysProxy: desired.SystemProxy,
		ActiveConfig: desired.ActiveConfig,
		Mode:         desired.Mode,
		Reason:       reason,
	}

	// 检查是否需要重启（TUN 启动意图变化或配置变化）。
	// ⚠️ 比较对象必须是「内核启动时的 TUN 意图」(runtimeState.Tun)，
	// 不能用 ActualTun：适配器探测失败时 ActualTun=false，若据此判定
	// 会导致每个 watchdog tick 都误判需要重启，形成重启风暴。
	appState := s.controller.GetAppState()
	launchedTun := s.controller.coreLaunchedWithTun()
	if appState.IsRunning && (desired.Tun != launchedTun || desired.ActiveConfig != appState.ActiveConfig) {
		plan.RestartRequired = true
	}
	return plan
}
