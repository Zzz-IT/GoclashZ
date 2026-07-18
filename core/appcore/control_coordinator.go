//go:build windows

package appcore

import (
	"context"
	"sync"
	"sync/atomic"
)

// ControlCoordinator serializes all user-facing proxy and TUN changes. The
// supervisor remains responsible for runtime reconciliation; this type owns
// command ordering and the desired-state commit/rollback boundary.
type ControlCoordinator struct {
	mu         sync.Mutex
	controller *Controller
	revision   atomic.Uint64
}

func NewControlCoordinator(controller *Controller) *ControlCoordinator {
	return &ControlCoordinator{controller: controller}
}

func (cc *ControlCoordinator) Revision() uint64 {
	return cc.revision.Load()
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
	target := !cc.controller.Desired.Get().SystemProxy
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
	if tun && enable {
		if err := cc.controller.prepareTunEnable(ctx); err != nil {
			return err
		}
	}

	previous := cc.controller.Desired.Get()
	desired := previous
	if tun {
		desired.Tun = enable
	} else {
		desired.SystemProxy = enable
	}
	if enable {
		desired.CoreRunning = true
	} else if !desired.SystemProxy && !desired.Tun {
		desired.CoreRunning = false
	}
	cc.controller.fillDesiredTarget(&desired)

	if desired == previous {
		// Idempotent commands still reconcile actual state. This repairs an
		// orphaned Windows proxy or a stopped core without inventing a second
		// desired-state transition.
		return cc.controller.Supervisor.Reconcile(ctx, reason)
	}
	if err := cc.controller.Desired.SetAndSave(desired); err != nil {
		return err
	}
	cc.revision.Add(1)
	cc.controller.SyncState()

	if err := cc.controller.Supervisor.Reconcile(ctx, reason); err != nil {
		// The coordinator serializes writers, so this rollback cannot overwrite
		// a newer control command.
		if rollbackErr := cc.controller.Desired.SetAndSave(previous); rollbackErr != nil {
			cc.controller.logError("控制状态回滚失败: %v", rollbackErr)
		}
		cc.revision.Add(1)
		cc.controller.SyncState()
		return err
	}

	cc.revision.Add(1)
	cc.controller.SyncState()
	return nil
}
