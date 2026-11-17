package runtimeassets

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"goclashz/core/utils"
)

// ErrAssetLocked 表示同一资产正被另一个修复/迁移流程占用（通常是 helper 或另一个入口），
// 调用方应把它当作"稍后重试"而不是"资产损坏"。
var ErrAssetLocked = errors.New("asset locked: 资产正被其它流程处理")

const (
	// assetLockStaleAfter 之后视为陈旧锁（进程崩溃留下的锁文件），可被回收。
	assetLockStaleAfter = 30 * time.Second
	assetLockPollEvery  = 50 * time.Millisecond
)

var (
	assetLockMu   sync.Mutex
	assetMutexMap = make(map[string]*sync.Mutex)
)

func assetMutexFor(name string) *sync.Mutex {
	assetLockMu.Lock()
	defer assetLockMu.Unlock()

	mu, ok := assetMutexMap[name]
	if !ok {
		mu = &sync.Mutex{}
		assetMutexMap[name] = mu
	}
	return mu
}

func assetLockFilePath(name string) string {
	return filepath.Join(utils.GetRuntimeDir(), ".asset-"+name+".lock")
}

// lockWithContext 在预算内获取互斥锁（TryLock + 轮询 + ctx）。用于串行化子进程探测之类
// 的短操作；返回 false 表示预算/ctx 用完，调用方应把它当作"这次跳过"。
func lockWithContext(ctx context.Context, mu *sync.Mutex, budget time.Duration) bool {
	if mu.TryLock() {
		return true
	}

	deadline := time.Now().Add(budget)
	for {
		if ctx != nil && ctx.Err() != nil {
			return false
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(assetLockPollEvery)
		if mu.TryLock() {
			return true
		}
	}
}

// withAssetLock 串行化"写同一资产"的所有动作（F-A1-13）：
//   - 进程内：按资产名互斥（app 内部的 repair/adopt/migrate 不会互相穿插）；
//   - 跨进程：%DATA%\runtime\.asset-<name>.lock（O_EXCL + 陈旧回收），覆盖 helper
//     或另一个实例同时替换同一文件。
//
// 跨进程锁拿不到时降级为"只做进程内互斥"并记日志：它是防并发写坏的加固，不是正确性前提。
// wait 是两次尝试之间的总预算；ctx 取消立即返回。
func withAssetLock(ctx context.Context, name string, wait time.Duration, fn func() error) error {
	mu := assetMutexFor(name)
	if !lockWithContext(ctx, mu, wait) {
		if ctx != nil && ctx.Err() != nil {
			return fmt.Errorf("%w: %s: %v", ErrAssetLocked, name, ctx.Err())
		}
		return fmt.Errorf("%w: %s（等待进程内锁超时）", ErrAssetLocked, name)
	}
	defer mu.Unlock()

	release, err := acquireFileLock(ctx, name, wait)
	if err != nil {
		log.Printf("[runtimeassets] 跨进程资产锁不可用，降级为进程内互斥: %v", err)
	} else if release != nil {
		defer release()
	}

	return fn()
}

// WithAssetLock 是 withAssetLock 的导出形态，供**跨包**复用同一把资产锁
// （例如 core/appcore 的组件更新器替换 clash.exe/wintun.dll 时）。
// 语义：进程内按名互斥 + 跨进程锁文件；拿不到锁时等待至多 wait，超时返回 ErrAssetLocked。
// 包内调用请继续用 withAssetLock（少一层转发）。
func WithAssetLock(ctx context.Context, name string, wait time.Duration, fn func() error) error {
	return withAssetLock(ctx, name, wait, fn)
}

// acquireFileLock 用 O_EXCL 建锁文件；存在且未陈旧则按 wait 轮询等待。
func acquireFileLock(ctx context.Context, name string, wait time.Duration) (func(), error) {
	path := assetLockFilePath(name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(wait)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			_, _ = fmt.Fprintf(f, "pid=%d at=%s", os.Getpid(), time.Now().Format(time.RFC3339))
			_ = f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}

		// 崩溃遗留的锁文件：超过 stale 时间直接回收，避免永久卡死自愈。
		if st, serr := os.Stat(path); serr == nil && time.Since(st.ModTime()) > assetLockStaleAfter {
			_ = os.Remove(path)
			continue
		}

		if ctx != nil && ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrAssetLocked, name, ctx.Err())
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: %s（跨进程锁等待超时）", ErrAssetLocked, name)
		}
		time.Sleep(assetLockPollEvery)
	}
}
