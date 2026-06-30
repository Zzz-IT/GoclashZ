//go:build windows

package appcore

import (
	"context"
	"errors"
	"goclashz/core/logger"
	"goclashz/core/sys"
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

	mu sync.Mutex
}

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
		controller:  c,
		desired:     d,
		reconcileCh: make(chan reconcileRequest, 10),
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

// ReconcileAsync 异步触发 reconcile，使用独立 timeout ctx
func (s *CoreSupervisor) ReconcileAsync(reason string) {
	ctx, cancel := context.WithTimeout(s.ctx, 8*time.Second)
	req := reconcileRequest{Reason: reason, Ctx: ctx}
	select {
	case s.reconcileCh <- req:
		go func() {
			// 等请求被消费后 cancel
			select {
			case <-ctx.Done():
			case <-time.After(10 * time.Second):
				cancel()
			}
		}()
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
			desired := s.desired.Get()
			if desired.CoreRunning {
				ctx, cancel := context.WithTimeout(s.ctx, 8*time.Second)
				if err := s.Reconcile(ctx, "watchdog"); err != nil {
					logger.Warnf("Reconcile(watchdog) failed: %v", err)
				}
				cancel()
			}
		}
	}
}

// Reconcile 执行状态调和，使用传入的操作级 ctx
func (s *CoreSupervisor) Reconcile(ctx context.Context, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	desired := s.desired.Get()
	behavior := s.controller.Behavior.Get()

	if desired.ActiveConfig == "" {
		desired.ActiveConfig = behavior.ActiveConfig
		desired.Mode = behavior.ActiveMode
	}

	if desired.Tun && !sys.IsWintunInstalled() {
		s.controller.setLastError("缺失 Wintun 驱动，请在设置中安装 Wintun")
		s.controller.SyncState()
		return ErrWintunMissing
	}

	plan := s.buildRuntimePlan(desired, behavior, reason)

	if !plan.NeedCore {
		s.controller.DisableAll()
		return nil
	}

	if err := s.controller.reconcileCoreProcess(ctx, plan); err != nil {
		s.controller.setLastError(err.Error())
		s.controller.SyncState()
		return err
	}

	if err := s.controller.reconcileSystemProxy(plan); err != nil {
		s.controller.setLastError(err.Error())
		s.controller.SyncState()
		return err
	}

	s.controller.setRuntimeStateFromPlan(plan)
	s.controller.SyncState()
	return nil
}

func (s *CoreSupervisor) ShutdownRuntime(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.controller.DisableAll()
}

func (s *CoreSupervisor) buildRuntimePlan(desired DesiredState, _ AppBehavior, reason string) RuntimePlan {
	plan := RuntimePlan{
		NeedCore:     desired.CoreRunning || desired.SystemProxy || desired.Tun,
		NeedTun:      desired.Tun,
		NeedSysProxy: desired.SystemProxy,
		ActiveConfig: desired.ActiveConfig,
		Mode:         desired.Mode,
		Reason:       reason,
	}

	// 检查是否需要重启（TUN 变化或配置变化）
	appState := s.controller.GetAppState()
	if appState.IsRunning && (desired.Tun != appState.Tun || desired.ActiveConfig != appState.ActiveConfig) {
		plan.RestartRequired = true
	}
	return plan
}
