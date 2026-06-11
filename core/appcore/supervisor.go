//go:build windows

package appcore

import (
	"context"
	"goclashz/core/sys"
	"sync"
	"time"
)

type CoreSupervisor struct {
	controller *Controller

	ctx    context.Context
	cancel context.CancelFunc

	desired *DesiredStateStore

	reconcileCh  chan string
	watchdogTick *time.Ticker

	mu sync.Mutex
}

func NewCoreSupervisor(c *Controller, d *DesiredStateStore) *CoreSupervisor {
	return &CoreSupervisor{
		controller:  c,
		desired:     d,
		reconcileCh: make(chan string, 10), // Buffered to prevent blocking
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

func (s *CoreSupervisor) ReconcileAsync(reason string) {
	select {
	case s.reconcileCh <- reason:
	default:
		// Queue full, drop or log
	}
}

func (s *CoreSupervisor) loop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case reason := <-s.reconcileCh:
			s.Reconcile(reason)
		case <-s.watchdogTick.C:
			// Ensure running if needed
			desired := s.desired.Get()
			if desired.CoreRunning {
				s.Reconcile("watchdog")
			}
		}
	}
}

func (s *CoreSupervisor) Reconcile(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	desired := s.desired.Get()
	behavior := s.controller.Behavior.Get()

	if desired.ActiveConfig == "" {
		desired.ActiveConfig = behavior.ActiveConfig
		desired.Mode = behavior.ActiveMode
	}



	if desired.Tun && !sys.CheckAdmin() {
		s.controller.setLastError("TUN 需要管理员权限")
		s.controller.SyncState()
		return
	}

	if desired.Tun && !sys.IsWintunInstalled() {
		s.controller.setLastError("缺失 Wintun 驱动")
		s.controller.SyncState()
		return
	}

	needCore := desired.CoreRunning || desired.SystemProxy || desired.Tun
	if !needCore {
		s.controller.DisableAll()
		return
	}

	appState := s.controller.GetAppState()

	// 核心修复：如果核心正在运行，且 TUN 期望状态与运行时状态不一致，必须重启核心来重新生成配置
	if desired.Tun != appState.Tun && appState.IsRunning {
		err := s.controller.RestartCoreWithReason(s.ctx, reason)
		if err != nil {
			s.controller.setLastError(err.Error())
			s.controller.SyncState()
			return
		}
	} else {
		err := s.controller.EnsureCoreRunning(s.ctx)
		if err != nil {
			s.controller.setLastError(err.Error())
			s.controller.SyncState()
			return
		}
	}

	// 代理状态控制
	if desired.SystemProxy {
		s.controller.ensureSystemProxyEnabled()
	} else {
		s.controller.ensureSystemProxyDisabled()
	}

	s.controller.setRuntimeStateFromDesired(desired)
	s.controller.SyncState()
}

func (s *CoreSupervisor) ShutdownRuntime(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 退出时可以关闭当前代理，但不要把 desired 写回磁盘
	s.controller.DisableAll()
}
