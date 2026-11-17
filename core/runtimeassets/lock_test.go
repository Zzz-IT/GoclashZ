package runtimeassets

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWithAssetLockSerializes 覆盖资产锁的串行化语义（F-A1-19 / F-A1-13）。
func TestWithAssetLockSerializes(t *testing.T) {
	const name = "unittest-serialize"
	defer os.Remove(assetLockFilePath(name))

	var inFlight, maxInFlight int32
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := withAssetLock(context.Background(), name, 2*time.Second, func() error {
				n := atomic.AddInt32(&inFlight, 1)
				for {
					old := atomic.LoadInt32(&maxInFlight)
					if n <= old || atomic.CompareAndSwapInt32(&maxInFlight, old, n) {
						break
					}
				}
				time.Sleep(3 * time.Millisecond)
				atomic.AddInt32(&inFlight, -1)
				return nil
			})
			if err != nil {
				t.Errorf("withAssetLock 失败: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxInFlight); got != 1 {
		t.Fatalf("同名资产应串行，最大并发=%d", got)
	}
	if _, err := os.Stat(assetLockFilePath(name)); err == nil {
		t.Fatal("锁文件应在释放后删除")
	}
}

// TestAssetLockStaleRecovery 覆盖"进程崩溃遗留锁文件"的回收。
func TestAssetLockStaleRecovery(t *testing.T) {
	const name = "unittest-stale"
	path := assetLockFilePath(name)
	if err := os.MkdirAll(assetLockFilePath(name), 0755); err != nil {
		t.Skipf("无法准备运行目录: %v", err)
	}
	if err := os.Remove(assetLockFilePath(name)); err != nil {
		t.Skipf("无法清理目录: %v", err)
	}

	if err := os.WriteFile(path, []byte("pid=999999"), 0644); err != nil {
		t.Skipf("无法创建锁文件: %v", err)
	}
	old := time.Now().Add(-assetLockStaleAfter - time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Skipf("无法设置 mtime: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- withAssetLock(context.Background(), name, 2*time.Second, func() error { return nil })
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("陈旧锁应被回收: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("陈旧锁未被回收")
	}
	_ = os.Remove(path)
}

// TestAssetLockContextCanceled 覆盖等待锁时 ctx 取消 -> ErrAssetLocked（不永久阻塞）。
func TestAssetLockContextCanceled(t *testing.T) {
	const name = "unittest-cancel"
	defer os.Remove(assetLockFilePath(name))

	// 先占住锁
	hold := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = withAssetLock(context.Background(), name, time.Second, func() error {
			close(hold)
			<-release
			return nil
		})
	}()
	<-hold

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := WithAssetLock(ctx, name, 500*time.Millisecond, func() error { return nil })
	close(release)

	if !errors.Is(err, ErrAssetLocked) {
		t.Fatalf("持锁期间应返回 ErrAssetLocked, 实际: %v", err)
	}
}
