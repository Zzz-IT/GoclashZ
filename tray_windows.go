//go:build windows

package main

import (
	"context"
	"fmt"
	"goclashz/core/appcore"
	"time"

	"github.com/energye/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type trayAction struct {
	name    string
	timeout time.Duration
	run     func(context.Context) error
}

type trayItems struct {
	sysProxy   *systray.MenuItem
	tun        *systray.MenuItem
	modeRule   *systray.MenuItem
	modeGlobal *systray.MenuItem
	modeDirect *systray.MenuItem
	restart    *systray.MenuItem
}

func (a *App) StartTray(parent context.Context) {
	trayCtx, cancel := context.WithCancel(parent)

	a.trayCtx = trayCtx
	a.trayCancel = cancel
	a.trayActions = make(chan trayAction, 16)
	a.trayRenderReq = make(chan appcore.AppState, 4)

	go a.trayActionWorker(trayCtx)
	go a.trayRenderWorker(trayCtx)

	go func() {
		select {
		case <-time.After(500 * time.Millisecond):
			a.SetupSystray()
		case <-trayCtx.Done():
			return
		}
	}()
}

func (a *App) StopTray() {
	a.trayStopping.Store(true)

	if a.trayCancel != nil {
		a.trayCancel()
	}

	if a.trayReady.Load() {
		systray.Quit()
	}
}

func (a *App) SetupSystray() {
	if a.trayStopping.Load() {
		return
	}

	a.trayOnce.Do(func() {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					a.trayReady.Store(false)
					a.trayStopping.Store(true)
					fmt.Printf("托盘运行异常: %v\n", r)
				}
			}()

			systray.Run(a.onTrayReady, a.onTrayExit)
		}()
	})
}

func (a *App) onTrayReady() {
	systray.SetIcon(iconData)
	systray.SetTitle("GoclashZ")
	systray.SetTooltip("GoclashZ - Mihomo GUI")

	systray.SetDClickTimeMinInterval(500)
	systray.SetOnDClick(func(menu systray.IMenu) {
		go a.safeToggleMainWindow()
	})

	mShow := systray.AddMenuItem("显示界面", "显示主窗口")
	systray.AddSeparator()

	mSysProxy := systray.AddMenuItem("系统代理", "开启/关闭系统代理")
	mTun := systray.AddMenuItem("TUN 模式", "开启/关闭 TUN 模式")

	systray.AddSeparator()
	mModes := systray.AddMenuItem("出站模式", "切换 Clash 路由模式")
	mModeRule := mModes.AddSubMenuItem("规则 (Rule)", "Rule 模式")
	mModeGlobal := mModes.AddSubMenuItem("全局 (Global)", "Global 模式")
	mModeDirect := mModes.AddSubMenuItem("直连 (Direct)", "Direct 模式")

	systray.AddSeparator()
	mRestart := systray.AddMenuItem("重启内核", "重启 Clash 内核")
	mQuit := systray.AddMenuItem("退出程序", "彻底退出 GoclashZ")

	a.trayMu.Lock()
	a.mSysProxy = mSysProxy
	a.mTun = mTun
	a.mModeRule = mModeRule
	a.mModeGlobal = mModeGlobal
	a.mModeDirect = mModeDirect
	a.mRestart = mRestart
	a.trayMu.Unlock()

	a.trayReady.Store(true)
	a.trayStopping.Store(false)

	mShow.Click(func() {
		go a.safeShowMainWindow()
	})

	mSysProxy.Click(func() {
		a.enqueueTrayAction("toggle-system-proxy", 20*time.Second, func(ctx context.Context) error {
			state := a.core.GetAppState()
			return a.ToggleSystemProxy(!state.SystemProxy)
		})
	})

	mTun.Click(func() {
		a.enqueueTrayAction("toggle-tun", 25*time.Second, func(ctx context.Context) error {
			state := a.core.GetAppState()
			return a.ToggleTunMode(!state.Tun)
		})
	})

	mModeRule.Click(func() {
		a.enqueueTrayAction("switch-rule-mode", 10*time.Second, func(ctx context.Context) error {
			return a.UpdateClashMode("rule")
		})
	})

	mModeGlobal.Click(func() {
		a.enqueueTrayAction("switch-global-mode", 10*time.Second, func(ctx context.Context) error {
			return a.UpdateClashMode("global")
		})
	})

	mModeDirect.Click(func() {
		a.enqueueTrayAction("switch-direct-mode", 10*time.Second, func(ctx context.Context) error {
			return a.UpdateClashMode("direct")
		})
	})

	mRestart.Click(func() {
		a.enqueueTrayAction("restart-core", 25*time.Second, func(ctx context.Context) error {
			return a.core.RestartCoreWithReason(ctx, "manual")
		})
	})

	mQuit.Click(func() {
		go a.safeQuit()
	})

	a.SyncTrayState()
}

func (a *App) onTrayExit() {
	a.trayReady.Store(false)
	a.trayStopping.Store(true)

	a.trayMu.Lock()
	a.mSysProxy = nil
	a.mTun = nil
	a.mModeRule = nil
	a.mModeGlobal = nil
	a.mModeDirect = nil
	a.mRestart = nil
	a.trayMu.Unlock()
}

func (a *App) trayActionWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case action := <-a.trayActions:
			a.setTrayBusy(true)
			err := a.runTrayActionSafely(ctx, action)
			a.setTrayBusy(false)

			if err != nil {
				a.notifyTrayError(err.Error())
			}

			a.SyncTrayState()
		}
	}
}

func (a *App) runTrayActionSafely(parent context.Context, action trayAction) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("托盘操作 %s 发生异常: %v", action.name, r)
		}
	}()

	timeout := action.timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	return action.run(ctx)
}

func (a *App) trayRenderWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case state := <-a.trayRenderReq:
			a.applyTrayState(state)
		}
	}
}

func (a *App) applyTrayState(state appcore.AppState) {
	if !a.trayReady.Load() || a.trayStopping.Load() {
		return
	}

	a.withTrayItems(func(items trayItems) {
		if items.sysProxy != nil {
			if state.SystemProxy {
				items.sysProxy.Check()
			} else {
				items.sysProxy.Uncheck()
			}
		}

		if items.tun != nil {
			if state.Tun {
				items.tun.Check()
			} else {
				items.tun.Uncheck()
			}
		}

		if items.modeRule != nil && items.modeGlobal != nil && items.modeDirect != nil {
			items.modeRule.Uncheck()
			items.modeGlobal.Uncheck()
			items.modeDirect.Uncheck()

			switch state.Mode {
			case "rule":
				items.modeRule.Check()
			case "global":
				items.modeGlobal.Check()
			case "direct":
				items.modeDirect.Check()
			}
		}
	})
}

func (a *App) withTrayItems(fn func(trayItems)) {
	if fn == nil {
		return
	}

	if !a.trayReady.Load() || a.trayStopping.Load() {
		return
	}

	a.trayMu.RLock()
	items := trayItems{
		sysProxy:   a.mSysProxy,
		tun:        a.mTun,
		modeRule:   a.mModeRule,
		modeGlobal: a.mModeGlobal,
		modeDirect: a.mModeDirect,
		restart:    a.mRestart,
	}
	a.trayMu.RUnlock()

	fn(items)
}

func (a *App) setTrayBusy(busy bool) {
	a.trayBusy.Store(busy)

	a.withTrayItems(func(items trayItems) {
		if busy {
			items.sysProxy.Disable()
			items.tun.Disable()
			items.modeRule.Disable()
			items.modeGlobal.Disable()
			items.modeDirect.Disable()
			items.restart.Disable()
		} else {
			items.sysProxy.Enable()
			items.tun.Enable()
			items.modeRule.Enable()
			items.modeGlobal.Enable()
			items.modeDirect.Enable()
			items.restart.Enable()
		}
	})
}

func (a *App) enqueueTrayAction(name string, timeout time.Duration, run func(context.Context) error) {
	if run == nil {
		return
	}

	if a.trayStopping.Load() {
		return
	}

	action := trayAction{
		name:    name,
		timeout: timeout,
		run:     run,
	}

	select {
	case a.trayActions <- action:
	default:
		a.notifyTrayError("托盘操作繁忙，请稍后再试")
	}
}

func (a *App) SyncTrayState() {
	if !a.trayReady.Load() || a.trayStopping.Load() {
		return
	}

	state := a.core.GetAppState()

	select {
	case a.trayRenderReq <- state:
	default:
		// 队列满时丢弃旧渲染，保留最新状态
		select {
		case <-a.trayRenderReq:
		default:
		}

		select {
		case a.trayRenderReq <- state:
		default:
		}
	}
}

func (a *App) notifyTrayError(msg string) {
	if msg == "" || a.core == nil {
		return
	}

	events := a.core.GetEvents()
	if events != nil {
		events.Emit("notify-error", msg)
	}
}

func (a *App) safeShowMainWindow() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("显示主窗口异常: %v\n", r)
		}
	}()

	a.ShowMainWindow()
}

func (a *App) safeToggleMainWindow() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("切换主窗口异常: %v\n", r)
		}
	}()

	a.ToggleMainWindow()
}

func (a *App) safeQuit() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("退出程序异常: %v\n", r)
		}
	}()

	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}
