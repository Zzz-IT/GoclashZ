//go:build windows

package appcore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"goclashz/core/runtimeassets"
	"sync"
	"sync/atomic"
	"time"
)

// ControlCoordinator serializes all user-facing proxy and TUN changes. The
// supervisor remains responsible for runtime reconciliation; this type owns
// command ordering and the desired-state commit/rollback boundary.
type ControlCoordinator struct {
	mu         sync.Mutex
	controller *Controller
	revision   atomic.Uint64
	// transitioning 为 true 时表示有重事务（配置切换 / 内核重启）正在执行，
	// watchdog 的周期性和谐应跳过，避免事务中途被并行调和打乱。
	transitioning atomic.Bool
}

const (
	// controlCommandTimeout 是单次控制命令的默认预算。托盘入口不传 deadline，
	// 必须在这里补齐，避免与 UI 的 25s 语义分裂（F-A2-16）。
	controlCommandTimeout = 25 * time.Second
	// controlAsyncTimeout 是「两段式收尾」里后台阶段的预算：
	// 需要覆盖 helper stop-core 的完整预算（优雅 20s + 强杀 5s）与清理耗时（F-A2-05）。
	controlAsyncTimeout = 45 * time.Second
)

func NewControlCoordinator(controller *Controller) *ControlCoordinator {
	return &ControlCoordinator{controller: controller}
}

func (cc *ControlCoordinator) Revision() uint64 {
	return cc.revision.Load()
}

// Transitioning 报告是否有 runtime 重事务正在执行（供 supervisor 跳过周期性调和）。
func (cc *ControlCoordinator) Transitioning() bool {
	return cc != nil && cc.transitioning.Load()
}

func (cc *ControlCoordinator) SetSystemProxy(ctx context.Context, enable bool) error {
	return cc.set(ctx, enable, false, "system-proxy-toggle")
}

func (cc *ControlCoordinator) SetTun(ctx context.Context, enable bool) error {
	return cc.set(ctx, enable, true, "tun-toggle")
}

func (cc *ControlCoordinator) ToggleSystemProxy(ctx context.Context) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	// 端口代理模式下，切换目标取自 PortProxy 而非 SystemProxy，
	// 否则 SystemProxy 永远是 false，target 永远是 true，关不掉
	behavior := cc.controller.Behavior.Get()
	var target bool
	if behavior.PortProxyMode {
		target = !cc.controller.Desired.Get().PortProxy
	} else {
		target = !cc.controller.Desired.Get().SystemProxy
	}
	return cc.setLocked(ctx, target, false, "system-proxy-toggle")
}

func (cc *ControlCoordinator) ToggleTun(ctx context.Context) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	target := !cc.controller.Desired.Get().Tun
	return cc.setLocked(ctx, target, true, "tun-toggle")
}

func (cc *ControlCoordinator) set(ctx context.Context, enable, tun bool, reason string) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return cc.setLocked(ctx, enable, tun, reason)
}

func (cc *ControlCoordinator) setLocked(ctx context.Context, enable, tun bool, reason string) error {
	// 调用方没带 deadline 时补一个（托盘入口走的就是这条路，F-A2-16）。
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, controlCommandTimeout)
		defer cancel()
	}

	// 整段控制命令都算"重事务"：watchdog 在此期间跳过周期性调和（F-A2-17），
	// 避免它排队等在 reconcileMu 上、拿着已过期的 8s ctx 进去跑。
	cc.transitioning.Store(true)
	defer cc.transitioning.Store(false)

	previous := cc.controller.Desired.Get()
	desired := previous
	behavior := cc.controller.Behavior.Get()

	if tun {
		// TUN 逻辑：永远不触碰 PortProxy
		desired.Tun = enable
		if enable {
			desired.CoreRunning = true
		} else if !desired.SystemProxy && !desired.Tun {
			desired.CoreRunning = false
		}
	} else if behavior.PortProxyMode {
		// 仅端口代理模式：使用独立的 PortProxy 字段，与 CoreRunning/SystemProxy/Tun 完全解耦
		// TUN 开关不会触碰 PortProxy，因此两者互不影响
		desired.PortProxy = enable
		desired.SystemProxy = false
		// 关闭端口代理时，若 TUN 也未启用，则同步清零 CoreRunning
		// 防止之前由系统代理设置的 CoreRunning=true 残留，导致内核无法停止
		if !enable && !desired.Tun {
			desired.CoreRunning = false
		}
		// 不在 enable=true 时主动设置 CoreRunning，由 NeedCore=PortProxy 驱动即可
	} else {
		// 正常系统代理逻辑：永远不触碰 PortProxy
		desired.SystemProxy = enable
		if enable {
			desired.CoreRunning = true
		} else if !desired.SystemProxy && !desired.Tun {
			desired.CoreRunning = false
		}
	}
	cc.controller.fillDesiredTarget(&desired)

	if desired == previous {
		// Idempotent commands still reconcile actual state. This repairs an
		// orphaned Windows proxy or a stopped core without inventing a second
		// desired-state transition.
		//
		// 关闭 TUN 的幂等点击同样走后台（F-A2-05）：真实状态可能还停在"内核未停干净"，
		// 同步等 stop-core 会再次吃掉 25s 预算。
		if tun && !enable {
			cc.runAsyncReconcile(reason)
			return nil
		}
		return cc.controller.Supervisor.Reconcile(ctx, reason)
	}

	// 只有发生真实状态跃迁时才跑前置门禁（helper 快 ping + 资产快检）——
	// 历史实现把 prepareTunEnable 放在"意图未变"判定之前，幂等点击也要付全价（F-A2-20）。
	if tun && enable {
		if err := cc.controller.prepareTunEnable(ctx); err != nil {
			return err
		}
	}

	if err := cc.controller.Desired.SetAndSave(desired); err != nil {
		return err
	}
	cc.revision.Add(1)
	cc.controller.SyncState()

	// 两段式收尾（F-A2-05）：关闭 TUN 意味着停内核（helper stop-core 最坏 30s，
	// 且该 RPC 不接受 ctx）。同步等它只会把 UI 的 25s 预算吃穿，把"慢"显示成"失败"。
	// 这里先提交意图并立即返回，停止/清理交给后台 Reconcile 收敛。
	if tun && !enable {
		cc.runAsyncReconcile(reason)
		cc.revision.Add(1)
		cc.controller.SyncState()
		return nil
	}

	if err := cc.controller.Supervisor.Reconcile(ctx, reason); err != nil {
		// A 类（瞬时可自愈）失败不撤销用户意图：保留 desired 并转后台重试（F-A2-08）。
		// 只有 B/C 类（需要用户处理）才回滚并把错误抛给 UI。
		if isTransientControlError(err) {
			cc.controller.logWarn("控制动作遇到瞬时失败，保留用户意图并转入后台重试: %v", err)
			cc.runAsyncReconcile(reason + "-retry")
			cc.revision.Add(1)
			cc.controller.SyncState()
			return nil
		}

		// Use SetAndSaveIf (CAS) instead of an unconditional SetAndSave.
		// Config and mode switches may write desired state outside the
		// coordinator's mutex boundary; a blind overwrite would silently
		// discard those concurrent changes.
		if ok, rollbackErr := cc.controller.Desired.SetAndSaveIf(desired, previous); rollbackErr != nil {
			cc.controller.logError("控制状态回滚失败: %v", rollbackErr)
		} else if !ok {
			cc.controller.logError("控制状态回滚已跳过：检测到并发写入，保留更新后的状态")
		}
		cc.revision.Add(1)
		cc.controller.SyncState()
		return err
	}

	cc.revision.Add(1)
	cc.controller.SyncState()
	return nil
}

// runAsyncReconcile 在后台以独立预算跑一次调和：不持有 cc.mu，不阻塞调用方。
// 用于「两段式收尾」与「瞬时失败保留意图、后台重试」两条路径（F-A2-05 / F-A2-08）。
func (cc *ControlCoordinator) runAsyncReconcile(reason string) {
	c := cc.controller

	go func() {
		base := context.Background()
		if c.ctx != nil {
			base = c.ctx
		}
		ctx, cancel := context.WithTimeout(base, controlAsyncTimeout)
		defer cancel()

		if err := c.Supervisor.Reconcile(ctx, reason); err != nil {
			c.logWarn("后台收尾调和未完全收敛(%s): %v", reason, err)
		}
	}()
}

// isTransientControlError 判断控制动作失败是否属于「A 类：瞬时可自愈」。
//
// A 类：ctx 超时/取消、适配器晚到、helper 唤醒慢、资产读写被占用/超时。
// B/C 类（helper 未安装/需修复、无活动配置、配置语法错、权限、端口占用）仍按失败处理，
// 以免把「用户必须处理的问题」吞成静默重试（F-A2-08）。
func isTransientControlError(err error) bool {
	if err == nil {
		return false
	}

	// 先排除明确的「需要用户处理」错误，避免误吞。
	if errors.Is(err, ErrNoActiveConfig) ||
		errors.Is(err, ErrHelperInstallRequired) ||
		errors.Is(err, ErrHelperRepairRequired) ||
		errors.Is(err, ErrWintunMissing) {
		return false
	}

	if isTunAdapterPendingErr(err) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) {
		return true
	}

	// 资产锁被并发占用属于"另一个修复在跑"，下一轮即可重试（跨批遗留②）。
	if errors.Is(err, runtimeassets.ErrAssetLocked) {
		return true
	}

	msg := strings.ToLower(err.Error())
	for _, hard := range []string{
		"helper_install_required", "helper_repair_required", "no active config",
		"配置预检", "运行组件不可用", "access is denied", "拒绝访问", "权限",
		"address already in use", "端口被占用", "文件不存在",
	} {
		if strings.Contains(msg, hard) {
			return false
		}
	}
	for _, soft := range []string{
		"timeout", "timed out", "超时", "deadline exceeded",
		"未就绪", "尚未就绪", "被取消", "context canceled", "停止确认",
	} {
		if strings.Contains(msg, soft) {
			return true
		}
	}
	return false
}

// ======================= 运行时操作统一入口 =======================
//
// ControlCoordinator 是唯一的 runtime 操作入口：
//   - 单开关（系统代理 / TUN）：set / setLocked
//   - 内核重启：RestartCore（对外 RestartCoreWithReason 只是它的兼容壳）
//   - 配置切换 / 配置重载：SwitchConfig / ReloadConfig（事务）
//
// 真正的内核重启实现是 Controller.restartCoreWithReason（内部私有）。
// supervisor 调和时直接调用私有实现，避免在已持有本协调器锁的事务中重入造成死锁。

// TransitionStage 标识一次 runtime 事务失败的阶段，便于 UI 与日志精确定位。
type TransitionStage string

const (
	TransitionStageDisableSystemProxy TransitionStage = "disable-system-proxy"
	TransitionStageStopTun            TransitionStage = "stop-tun"
	TransitionStageStopCore           TransitionStage = "stop-core"
	TransitionStageApplyConfig        TransitionStage = "apply-config"
	TransitionStageStartCore          TransitionStage = "start-core"
	TransitionStageWaitProviders      TransitionStage = "wait-providers"
	TransitionStageRestoreTun         TransitionStage = "restore-tun"
	TransitionStageRestoreSysProxy    TransitionStage = "restore-system-proxy"
)

// TransitionError 表示一次 runtime 事务在某个阶段失败（Error() 携带阶段名）。
type TransitionError struct {
	Stage TransitionStage
	Err   error
}

func (e *TransitionError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("runtime 事务失败于阶段 %s: %v", e.Stage, e.Err)
}

// Unwrap 让 errors.Is/As 能穿透到原始错误。
func (e *TransitionError) Unwrap() error { return e.Err }

// ConfigTransitionOptions 描述一次「运行中配置切换 / 配置重载」事务。
type ConfigTransitionOptions struct {
	// Reason 用于事件与日志（config-switch / subscription-update / config-save）。
	Reason string
	// Apply 在事务中段执行：写入新的配置选择（Behavior.ActiveConfig + desired state）。
	// 为 nil 表示「沿用当前配置重载」（订阅更新、配置文本保存）。
	Apply func() error
	// Rollback 在事务失败回滚时恢复配置选择（Apply 的反向操作）。
	Rollback func() error
}

// RestartCore 是内核重启的唯一对外入口。
//
// 重启本身沿用既有语义（不改动系统代理注册表），只在「期望开启系统代理」时
// 按当前端口重新下发一次（端口未变时是幂等空操作，配置重载改了端口时自动修复）。
// 期望关闭时不主动改写注册表，避免误关其它程序的代理配置（与既有重启行为一致）。
func (cc *ControlCoordinator) RestartCore(ctx context.Context, reason string) error {
	cc.transitioning.Store(true)
	defer cc.transitioning.Store(false)

	cc.mu.Lock()
	defer cc.mu.Unlock()

	c := cc.controller

	if err := c.restartCoreWithReason(ctx, reason); err != nil {
		return err
	}

	if c.Desired.Get().SystemProxy {
		if err := c.ensureSystemProxyEnabled(); err != nil {
			return &TransitionError{Stage: TransitionStageRestoreSysProxy, Err: err}
		}
	}

	c.SyncState()
	return nil
}

// SwitchConfig 以事务方式切换/重载运行中的配置。
//
// 事务顺序：关系统代理 → 停 TUN → 停内核 → 换配置 → 起内核 → 等 API + 代理组
// → 恢复 TUN（真实适配器确认）→ 恢复系统代理。
// 失败时回滚配置选择与运行态，并在返回错误里标明失败阶段。
func (cc *ControlCoordinator) SwitchConfig(ctx context.Context, opts ConfigTransitionOptions) error {
	cc.transitioning.Store(true)
	defer cc.transitioning.Store(false)

	cc.mu.Lock()
	defer cc.mu.Unlock()
	return cc.switchConfigLocked(ctx, opts)
}

// ReloadConfig 用当前配置重载运行态（订阅更新 / 配置文本保存）。
func (cc *ControlCoordinator) ReloadConfig(ctx context.Context, reason string) error {
	return cc.SwitchConfig(ctx, ConfigTransitionOptions{Reason: reason})
}

func (cc *ControlCoordinator) switchConfigLocked(ctx context.Context, opts ConfigTransitionOptions) error {
	c := cc.controller

	reason := strings.TrimSpace(opts.Reason)
	if reason == "" {
		reason = "config-switch"
	}

	previousDesired := c.Desired.Get()
	previousConfig := c.Behavior.Get().ActiveConfig
	proxyBefore := c.captureSystemProxyState()

	// 1) 关系统代理：只关闭「可判定为本程序写入」的代理，
	//    读取失败（unknown）时不盲关，也不会在结尾盲开。
	if proxyBefore.owned {
		if err := c.ensureSystemProxyDisabled(); err != nil {
			return &TransitionError{Stage: TransitionStageDisableSystemProxy, Err: err}
		}
	}

	// 2) 停 TUN / 停内核：内核是 TUN 的宿主，停内核即释放 TUN；
	//    停止确认交给 helper（core-status），主进程不做任何路由/适配器清理。
	stopStage := TransitionStageStopCore
	if previousDesired.Tun {
		stopStage = TransitionStageStopTun
	}
	if err := c.stopCoreForTransition(ctx); err != nil {
		// 停止/停止确认失败：配置尚未改动，直接把运行态恢复到切换前并返回阶段错误。
		cc.rollbackTransition(ctx, opts, previousDesired, previousConfig)
		return &TransitionError{Stage: stopStage, Err: err}
	}
	if previousDesired.Tun {
		c.observeTunAdapterAfterStop()
	}

	// 3) 换配置
	if opts.Apply != nil {
		if err := opts.Apply(); err != nil {
			cc.rollbackTransition(ctx, opts, previousDesired, previousConfig)
			return &TransitionError{Stage: TransitionStageApplyConfig, Err: err}
		}
	}

	desiredAfter := c.Desired.Get()
	if desiredAfter.ActiveConfig == "" {
		behavior := c.Behavior.Get()
		desiredAfter.ActiveConfig = behavior.ActiveConfig
		desiredAfter.Mode = behavior.ActiveMode
	}

	needCore := desiredAfter.CoreRunning || desiredAfter.SystemProxy || desiredAfter.Tun || desiredAfter.PortProxy
	if needCore {
		// 4) 起内核（内部等待 API 就绪）
		// TUN 适配器等待单独限预算：默认 20s 会把切配置的同步调用拖到几十秒，
		// 没等到就交给静默阶梯收敛，不在用户操作路径上死等。
		startCtx := withTunWaitBudget(ctx, configSwitchTunWaitBudget)
		if err := c.ensureCoreRunningWithDesiredState(startCtx, desiredAfter); err != nil {
			cc.rollbackTransition(ctx, opts, previousDesired, previousConfig)
			return &TransitionError{Stage: TransitionStageStartCore, Err: err}
		}

		// 5) 等待代理组/provider 就绪后，事务才算真正可用
		if err := c.waitProxyGroupsReady(ctx); err != nil {
			cc.rollbackTransition(ctx, opts, previousDesired, previousConfig)
			return &TransitionError{Stage: TransitionStageWaitProviders, Err: err}
		}

		// 6) 恢复 TUN：只认真实适配器探测结果，绝不用 desired 伪造。
		//    这一步不再同步轮询（事务路径不允许再挂 20s）：先做一次只读探测，
		//    没命中就交给后台收敛 + 静默阶梯（escalateTunReadinessIfNeeded）。
		if desiredAfter.Tun {
			if _, err := c.probeTunActual(); err != nil {
				if isTunAdapterPendingErr(err) {
					c.logWarn("TUN 恢复尚未就绪，转为后台探测（事务继续）: %v", err)
					c.SyncState()
				} else {
					c.setLastErrorCode("TUN_RESTORE_FAILED", "TUN 未能恢复: "+err.Error())
					c.SyncState()
					return &TransitionError{Stage: TransitionStageRestoreTun, Err: err}
				}
			}
		}
	} else {
		c.clearTunActual()
	}

	// 7) 恢复系统代理：按用户意图下发；状态未知时只告警，不盲目开启
	if !proxyBefore.known {
		c.notify("warn", "SYSTEM_PROXY_STATE_UNKNOWN", "系统代理状态读取失败，已跳过自动恢复（不会盲目开启）", "config-switch")
	} else if desiredAfter.SystemProxy {
		if err := c.ensureSystemProxyEnabled(); err != nil {
			return &TransitionError{Stage: TransitionStageRestoreSysProxy, Err: err}
		}
	} else if c.systemProxyActiveNow() {
		// 意图为关闭但实际仍处于开启：兜底收紧，避免指向已停止的旧端口。
		if err := c.ensureSystemProxyDisabled(); err != nil {
			return &TransitionError{Stage: TransitionStageRestoreSysProxy, Err: err}
		}
	}

	cc.emitRuntimeEvents(reason)
	c.SyncState()
	return nil
}

// emitRuntimeEvents 发送与内核重启同口径的事件（避免 UI 缓存陈旧）。
func (cc *ControlCoordinator) emitRuntimeEvents(reason string) {
	cc.controller.events.Emit("core-restarted", map[string]any{
		"reason": reason,
	})

	switch reason {
	case "manual", "config-switch", "subscription-update", "restore", "config-save":
		cc.controller.events.Emit("delay-cache-clear", reason)
	}
}

// rollbackTransition 在事务失败时把配置选择与运行态恢复到切换前。
// 回滚自身的失败只记录日志，不覆盖原始失败阶段。
func (cc *ControlCoordinator) rollbackTransition(ctx context.Context, opts ConfigTransitionOptions, previousDesired DesiredState, previousConfig string) {
	c := cc.controller

	c.logWarn("配置切换失败，开始回滚到切换前配置: %s", previousConfig)

	if opts.Rollback != nil {
		if err := opts.Rollback(); err != nil {
			c.logError("回滚配置选择失败: %v", err)
		}
	}

	current := c.Desired.Get()
	if ok, err := c.Desired.SetAndSaveIf(current, previousDesired); err != nil {
		c.logError("回滚 desired state 失败: %v", err)
	} else if !ok {
		c.logWarn("回滚 desired state 已跳过：检测到并发写入，保留更新后的状态")
	}

	// 回滚可能有独立的生命周期，不能依赖已经取消/超时的调用方 ctx。
	base := context.Background()
	if c.ctx != nil {
		base = c.ctx
	}
	rollbackCtx, cancel := context.WithTimeout(base, 45*time.Second)
	defer cancel()

	if err := c.stopCoreForTransition(rollbackCtx); err != nil {
		c.logError("回滚时停止内核失败: %v", err)
	}

	desired := c.Desired.Get()
	if desired.ActiveConfig == "" {
		behavior := c.Behavior.Get()
		desired.ActiveConfig = behavior.ActiveConfig
		desired.Mode = behavior.ActiveMode
	}
	needCore := desired.CoreRunning || desired.SystemProxy || desired.Tun || desired.PortProxy
	if needCore {
		if err := c.ensureCoreRunningWithDesiredState(rollbackCtx, desired); err != nil {
			c.logError("回滚后重启内核失败: %v", err)
			c.setLastErrorCode("CONFIG_ROLLBACK_INCOMPLETE", "配置切换失败且回滚未完全恢复: "+err.Error())
		}
	}

	c.SyncState()
}
