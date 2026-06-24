//go:build windows

package appcore

import (
	"context"
	"errors"
	"fmt"
	"goclashz/core/logger"
	"goclashz/core/sys"
	"sync"
	"time"
)

// 哨兵错误
var (
	ErrNoActiveConfig        = errors.New("no active config selected")
	ErrHelperInstallRequired = errors.New("helper_install_required")
	ErrTunNeedHelperOrAdmin  = errors.New("tun_need_helper_or_admin")
	ErrWintunMissing         = errors.New("wintun_missing")
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
		reconcileCh: make(chan string, 10),
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
	}
}

func (s *CoreSupervisor) loop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case reason := <-s.reconcileCh:
			if err := s.Reconcile(reason); err != nil {
				logger.Warnf("Reconcile(%s) failed: %v", reason, err)
			}
		case <-s.watchdogTick.C:
			desired := s.desired.Get()
			if desired.CoreRunning {
				if err := s.Reconcile("watchdog"); err != nil {
					logger.Warnf("Reconcile(watchdog) failed: %v", err)
				}
			}
		}
	}
}

// Reconcile 执行状态调和，返回 error 供调用方处理
func (s *CoreSupervisor) Reconcile(reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	desired := s.desired.Get()
	behavior := s.controller.Behavior.Get()

	if desired.ActiveConfig == "" {
		desired.ActiveConfig = behavior.ActiveConfig
		desired.Mode = behavior.ActiveMode
	}

	if desired.Tun {
		client := sys.NewHelperClient()
		if err := client.Ping(); err != nil && !sys.CheckAdmin() {
			s.controller.setLastError("TUN 模式需要后台服务 (GoclashZHelper) 或以管理员身份运行")
			s.controller.SyncState()
			return ErrTunNeedHelperOrAdmin
		}
	}

	if desired.Tun && !sys.IsWintunInstalled() {
		s.controller.setLastError("缺失 Wintun 驱动，请在设置中安装 Wintun")
		s.controller.SyncState()
		return ErrWintunMissing
	}

	needCore := desired.CoreRunning || desired.SystemProxy || desired.Tun
	if !needCore {
		s.controller.DisableAll()
		return nil
	}

	appState := s.controller.GetAppState()

	if desired.Tun != appState.Tun && appState.IsRunning {
		if err := s.controller.RestartCoreWithReason(s.ctx, reason); err != nil {
			s.controller.setLastError(err.Error())
			s.controller.SyncState()
			return fmt.Errorf("restart core failed: %w", err)
		}
	} else {
		if err := s.controller.EnsureCoreRunning(s.ctx); err != nil {
			s.controller.setLastError(err.Error())
			s.controller.SyncState()
			return err
		}
	}

	if desired.SystemProxy {
		s.controller.ensureSystemProxyEnabled()
	} else {
		s.controller.ensureSystemProxyDisabled()
	}

	s.controller.setRuntimeStateFromDesired(desired)
	s.controller.SyncState()
	return nil
}

func (s *CoreSupervisor) ShutdownRuntime(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.controller.DisableAll()
}
