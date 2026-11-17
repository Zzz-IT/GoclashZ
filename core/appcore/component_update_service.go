//go:build windows

package appcore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"goclashz/core/clash"
	"goclashz/core/logger"
)

// ErrComponentUpdateBusy 表示已有组件更新事务在推进（内核/驱动），
// 调用方应立即返回而不是阻塞在锁上（F-A5-05）。
var ErrComponentUpdateBusy = errors.New("组件更新进行中（内核/驱动），请等待完成后再试")

type ComponentUpdateOptions struct {
	Name         string
	StopCore     bool
	RestartCore  bool
	Prepare      func(ctx context.Context, onProgress func(bytesDone, totalBytes, speedBps, etaSec int64)) (map[string]string, error)
	Commit       func(ctx context.Context, prepared map[string]string) (map[string]string, error)
	AfterSuccess func(map[string]string)
}

// runComponentUpdateTransaction 统一处理运行时组件（内核、驱动）的更新事务
func (c *Controller) runComponentUpdateTransaction(
	ctx context.Context,
	taskName string,
	opt ComponentUpdateOptions,
) {
	// Guard: if a task with this name is already "running", do not launch a
	// second one.  Setting the initial state *here* (synchronously, before the
	// goroutine starts) closes the race window where a rapid second call would
	// see no active entry in UpdateTasks and proceed to cancel the first task.
	if existing, ok := c.UpdateTasks.Get(taskName); ok && existing.Status == "running" {
		// 静默跳过也要留痕：否则用户连点两次「更新」时，第二次被丢掉完全无迹可查（F-A5-06）。
		logger.Warnf("组件更新 | task=%s 已有实例在运行（stage=%s），本次请求被忽略", taskName, existing.Stage)
		return
	}
	c.UpdateTasks.Set(taskName, UpdateTaskState{
		Key:       taskName,
		Title:     opt.Name,
		Status:    "running",
		Stage:     "preparing",
		StartedAt: time.Now().Unix(),
	})

	// 事件契约（F-A5-23）：-start / -success / -cancelled / -error 一律由任务框架
	// （core/tasks.Manager）统一发一次。事务内部以前还会再发一遍，导致同一动作前端
	// 收到两次同名事件；如果将来两处 payload 类型不一致，wails 会直接 cancel 事件。
	c.Tasks.Run(ctx, taskName, true, func(ctx context.Context) error {
		// Refresh the state entry that was pre-registered above; preserve
		// StartedAt so the UI doesn't reset the elapsed-time counter.
		state, _ := c.UpdateTasks.Get(taskName)
		state.Key = taskName
		state.Title = opt.Name
		state.Status = "running"
		state.Stage = "preparing"
		state.Error = ""
		c.UpdateTasks.Set(taskName, state)

		// 1. Prepare 阶段（下载）**不**持组件更新全局锁：
		//    61MB 内核 / 驱动包的下载可能持续几分钟，持锁会让「还原备份」「用种子替换内核」
		//    等入口无限期阻塞且没有任何反馈（F-A5-05）。
		//    真正的互斥收敛到 commit 窗口（第 3 步），那一段是可控的秒级操作。
		startedAt := time.Now().Unix()
		prepared, err := opt.Prepare(ctx, func(bytesDone, totalBytes, speedBps, etaSec int64) {
			state, _ := c.UpdateTasks.Get(taskName)
			state.Status = "running"
			state.Stage = "downloading"
			state.BytesDone = bytesDone
			state.BytesTotal = totalBytes
			state.SpeedBps = speedBps
			state.ETASeconds = etaSec
			state.StartedAt = startedAt
			c.UpdateTasks.Set(taskName, state)
		})
		if err != nil {
			c.SyncState()

			state, ok := c.UpdateTasks.Get(taskName)
			if !ok {
				state = UpdateTaskState{
					Key:   taskName,
					Title: opt.Name,
				}
			}

			isCanceled := errors.Is(err, context.Canceled) ||
				strings.Contains(strings.ToLower(err.Error()), "canceled") ||
				strings.Contains(strings.ToLower(err.Error()), "cancelled")

			if isCanceled {
				state.Status = "cancelled"
				state.Error = "已暂停"
				logger.Warnf("组件更新 | task=%s stage=prepare 已取消（耗时 %ds）", taskName, time.Now().Unix()-startedAt)
			} else {
				state.Status = "error"
				state.Error = fmt.Sprintf("准备失败: %v", err)
				// F-A5-06：更新失败必须落日志，否则「无日志、无提示、只有任务行」的复盘黑洞依旧。
				logger.Errorf("组件更新失败 | task=%s stage=prepare err=%v", taskName, err)
			}
			state.FinishedAt = time.Now().Unix()
			c.UpdateTasks.Set(taskName, state)

			return fmt.Errorf("%s准备失败: %w", opt.Name, err)
		}

		state, _ = c.UpdateTasks.Get(taskName)
		state.Status = "running"
		state.Stage = "committing"
		c.UpdateTasks.Set(taskName, state)

		// 3. commit 窗口独占组件更新锁 + 内核生命周期锁：
		//    只覆盖「停核 → 替换文件 → 起核」，等待上限与其它事务的 commit 时长同阶。
		//    加锁顺序固定为 componentUpdateMu → coreLifecycleMu，与
		//    ReplaceCoreWithSeed / RestoreBackup 一致，不会死锁。
		c.componentUpdateMu.Lock()
		defer c.componentUpdateMu.Unlock()

		c.coreLifecycleMu.Lock()

		wasRunning := clash.IsRunning()

		c.mu.RLock()
		wantSysProxy := c.sysProxyActive
		wantTun := c.tunActive
		c.mu.RUnlock()

		// 判定是否需要恢复运行。
		// 如果内核正在运行，或者逻辑上开启了代理/TUN，更新完成后应尝试恢复。
		shouldRestart := wasRunning || wantSysProxy || wantTun

		if opt.StopCore && wasRunning {
			c.stopCoreProcessLocked()
			c.coreLifecycleMu.Unlock()

			// 停止后立刻同步状态，停掉 traffic/proxy monitor
			c.SyncState()
		} else {
			c.coreLifecycleMu.Unlock()
		}

		// 4. Commit 阶段：仅执行极快的文件替换（此时内核已停）
		result, err := opt.Commit(ctx, prepared)
		if err != nil {
			// 更新失败，尝试恢复原有的运行状态
			if shouldRestart {
				_ = c.EnsureCoreRunning(ctx)
			}

			c.SyncState()
			c.UpdateTasks.Set(taskName, UpdateTaskState{
				Key:        taskName,
				Title:      opt.Name,
				Status:     "error",
				Error:      fmt.Sprintf("提交失败: %v", err),
				FinishedAt: time.Now().Unix(),
			})
			// F-A5-06：commit 失败（文件被占用 / 权限 / helper 超时）必须落日志。
			logger.Errorf("组件更新失败 | task=%s stage=commit err=%v", taskName, err)
			return fmt.Errorf("%s失败: %w", opt.Name, err)
		}

		// 4. 更新成功后的回调
		if opt.AfterSuccess != nil {
			opt.AfterSuccess(result)
		}

		// 5. 恢复运行
		if opt.RestartCore && shouldRestart {
			err = c.EnsureCoreRunning(ctx)

			if err != nil {
				c.SyncState()
				c.UpdateTasks.Set(taskName, UpdateTaskState{
					Key:        taskName,
					Title:      opt.Name,
					Status:     "error",
					Error:      fmt.Sprintf("内核恢复失败: %v", err),
					FinishedAt: time.Now().Unix(),
				})
				// 文件已替换成功但起不来：这是最需要复盘的一类「半成功」。
				logger.Errorf("组件更新半成功 | task=%s stage=restart err=%v（文件已替换，内核未能恢复运行）", taskName, err)
				return fmt.Errorf("%s成功，但内核恢复启动失败: %w", opt.Name, err)
			}
		}

		c.UpdateTasks.Set(taskName, UpdateTaskState{
			Key:        taskName,
			Title:      opt.Name,
			Status:     "success",
			Progress:   100,
			FinishedAt: time.Now().Unix(),
		})
		logger.Infof("组件更新成功 | task=%s 耗时 %ds（含下载）", taskName, time.Now().Unix()-startedAt)

		c.SyncState()
		return nil
	})
}
