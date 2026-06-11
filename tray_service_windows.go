//go:build windows

package main

import (
	"context"
	"fmt"
	"goclashz/core/appcore"
	"sync/atomic"
	"time"

	"github.com/energye/systray"
	"sync"
)

type TrayService struct {
	app         *App
	ctx         context.Context
	cancel      context.CancelFunc
	stateCh     chan appcore.AppState
	actionCh    chan func()
	mu          sync.Mutex
	lastState   appcore.AppState
	renderedState appcore.AppState
	
	// Menu items
	mSysProxy   *systray.MenuItem
	mTun        *systray.MenuItem
	mModeRule   *systray.MenuItem
	mModeGlobal *systray.MenuItem
	mModeDirect *systray.MenuItem
	mRestart    *systray.MenuItem
	
	trayReady   atomic.Bool
	started     atomic.Bool
	watchdogMu  sync.Mutex
	isRestarting bool
}

func NewTrayService(app *App) *TrayService {
	return &TrayService{
		app:      app,
		stateCh:  make(chan appcore.AppState, 5),
		actionCh: make(chan func(), 10),
	}
}

func (t *TrayService) Start(ctx context.Context) {
	if !t.started.CompareAndSwap(false, true) {
		return
	}
	t.mu.Lock()
	if t.cancel != nil {
		t.cancel()
	}
	t.ctx, t.cancel = context.WithCancel(ctx)
	t.mu.Unlock()

	go t.runSystray()
	go t.loop()
	go t.Watchdog()
}

func (t *TrayService) runSystray() {
	systray.Run(t.onReady, t.onExit)
}

func (t *TrayService) Restart() {
	t.watchdogMu.Lock()
	if t.isRestarting {
		t.watchdogMu.Unlock()
		return
	}
	t.isRestarting = true
	t.watchdogMu.Unlock()

	t.trayReady.Store(false)
	systray.Quit()
	
	// Wait a bit before restarting
	time.Sleep(2 * time.Second)
	
	select {
	case <-t.ctx.Done():
		t.watchdogMu.Lock()
		t.isRestarting = false
		t.watchdogMu.Unlock()
		return
	default:
	}

	go t.runSystray()
	
	t.watchdogMu.Lock()
	t.isRestarting = false
	t.watchdogMu.Unlock()
}

func (t *TrayService) Watchdog() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.watchdogMu.Lock()
			restarting := t.isRestarting
			t.watchdogMu.Unlock()
			
			// 如果托盘长期未就绪，则尝试自愈重启
			if !restarting && !t.trayReady.Load() {
				t.Restart()
			}
		}
	}
}

func (t *TrayService) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	t.trayReady.Store(false)
	systray.Quit()
}

func (t *TrayService) UpdateState(state appcore.AppState) {
	select {
	case t.stateCh <- state:
	default:
	}
}

func (t *TrayService) RunAction(fn func()) {
	select {
	case t.actionCh <- fn:
	default:
	}
}

func (t *TrayService) loop() {
	for {
		select {
		case <-t.ctx.Done():
			return
		case state := <-t.stateCh:
			t.lastState = state
			t.render()
		case action := <-t.actionCh:
			// Execute action safely
			func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Printf("Tray action error: %v\n", r)
					}
				}()
				action()
			}()
		}
	}
}

func (t *TrayService) onReady() {
	systray.SetIcon(iconData)
	systray.SetTitle("GoclashZ")
	systray.SetTooltip("GoclashZ - Mihomo GUI")

	systray.SetDClickTimeMinInterval(500)
	systray.SetOnDClick(func(menu systray.IMenu) {
		t.app.safeShowMainWindow() // 改为只显示，避免多次点击导致闪烁
	})

	mShow := systray.AddMenuItem("显示界面", "显示主窗口")
	systray.AddSeparator()

	t.mSysProxy = systray.AddMenuItem("系统代理", "开启/关闭系统代理")
	t.mTun = systray.AddMenuItem("TUN 模式", "开启/关闭 TUN 模式")

	systray.AddSeparator()
	mModes := systray.AddMenuItem("出站模式", "切换 Clash 路由模式")
	t.mModeRule = mModes.AddSubMenuItem("规则 (Rule)", "Rule 模式")
	t.mModeGlobal = mModes.AddSubMenuItem("全局 (Global)", "Global 模式")
	t.mModeDirect = mModes.AddSubMenuItem("直连 (Direct)", "Direct 模式")

	systray.AddSeparator()
	t.mRestart = systray.AddMenuItem("重启内核", "重启 Clash 内核")
	mQuit := systray.AddMenuItem("退出程序", "彻底退出 GoclashZ")

	t.trayReady.Store(true)

	mShow.Click(func() {
		t.app.safeShowMainWindow()
	})

	t.mSysProxy.Click(func() {
		t.RunAction(func() {
			state := t.lastState
			_ = t.app.core.ToggleSystemProxy(t.ctx, !state.SystemProxy)
		})
	})

	t.mTun.Click(func() {
		t.RunAction(func() {
			state := t.lastState
			_ = t.app.core.ToggleTunMode(t.ctx, !state.Tun)
		})
	})

	t.mModeRule.Click(func() {
		t.RunAction(func() {
			_ = t.app.core.UpdateClashMode(t.ctx, "rule")
		})
	})

	t.mModeGlobal.Click(func() {
		t.RunAction(func() {
			_ = t.app.core.UpdateClashMode(t.ctx, "global")
		})
	})

	t.mModeDirect.Click(func() {
		t.RunAction(func() {
			_ = t.app.core.UpdateClashMode(t.ctx, "direct")
		})
	})

	t.mRestart.Click(func() {
		t.RunAction(func() {
			_ = t.app.core.RestartCoreWithReason(t.ctx, "manual")
		})
	})

	mQuit.Click(func() {
		t.app.safeQuit()
	})
	
	t.render()
}

func (t *TrayService) onExit() {
	t.trayReady.Store(false)
}

func (t *TrayService) render() {
	if !t.trayReady.Load() {
		return
	}
	
	state := t.lastState
	
	if state.SystemProxy != t.renderedState.SystemProxy {
		if state.SystemProxy {
			t.mSysProxy.Check()
		} else {
			t.mSysProxy.Uncheck()
		}
	}

	if state.Tun != t.renderedState.Tun {
		if state.Tun {
			t.mTun.Check()
		} else {
			t.mTun.Uncheck()
		}
	}

	if state.Mode != t.renderedState.Mode {
		t.mModeRule.Uncheck()
		t.mModeGlobal.Uncheck()
		t.mModeDirect.Uncheck()
		switch state.Mode {
		case "rule":
			t.mModeRule.Check()
		case "global":
			t.mModeGlobal.Check()
		case "direct":
			t.mModeDirect.Check()
		}
	}

	if state.ActiveConfigName != t.renderedState.ActiveConfigName {
		if state.ActiveConfigName != "" {
			systray.SetTooltip(fmt.Sprintf("GoclashZ - %s", state.ActiveConfigName))
		} else {
			systray.SetTooltip("GoclashZ")
		}
	}
	
	t.renderedState = state
}
