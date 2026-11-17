package clash

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"goclashz/core/logger"
	"goclashz/core/utils"
)

// WaitFileReleased 等待文件被释放（变为可写状态）
func WaitFileReleased(path string, timeout time.Duration) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		// 尝试以读写模式打开文件，如果成功说明没有进程独占该文件
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err == nil {
			_ = f.Close()
			return nil
		}

		lastErr = err
		time.Sleep(250 * time.Millisecond)
	}

	if lastErr == nil {
		return fmt.Errorf("等待文件释放超时")
	}

	return lastErr
}

// retryRename 带重试的文件重命名
func retryRename(oldPath, newPath string, attempts int, delay time.Duration) error {
	var lastErr error

	for i := 0; i < attempts; i++ {
		if err := os.Rename(oldPath, newPath); err == nil {
			return nil
		} else {
			lastErr = err
			time.Sleep(delay)
		}
	}

	return lastErr
}

// ---- 资产替换锁（跨批遗留③，配合 core/runtimeassets 的 withAssetLock）----
//
// 锁文件路径与格式刻意与 runtimeassets 完全一致（%DATA%/runtime/.asset-<name>.lock，
// O_EXCL + 30s 陈旧回收），因此 app 自愈、helper、另一个实例会互相排队。
// 跨进程锁拿不到时降级为进程内互斥：它是防并发写坏的加固，不是正确性前提。

const assetSwapLockWait = 5 * time.Second

var (
	assetSwapMu     sync.Mutex
	assetSwapMutexs = make(map[string]*sync.Mutex)
)

func assetSwapMutexFor(name string) *sync.Mutex {
	assetSwapMu.Lock()
	defer assetSwapMu.Unlock()

	mu, ok := assetSwapMutexs[name]
	if !ok {
		mu = &sync.Mutex{}
		assetSwapMutexs[name] = mu
	}
	return mu
}

func assetSwapLockPath(name string) string {
	return filepath.Join(utils.GetRuntimeDir(), ".asset-"+name+".lock")
}

func acquireAssetSwapFileLock(name string, deadline time.Time) (func(), error) {
	path := assetSwapLockPath(name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}

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
		if st, serr := os.Stat(path); serr == nil && time.Since(st.ModTime()) > 30*time.Second {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("跨进程资产锁等待超时: %s", name)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// withAssetSwapLock 串行化对同一个资产文件的替换动作。
func withAssetSwapLock(destPath string, fn func() error) error {
	name := filepath.Base(destPath)
	mu := assetSwapMutexFor(name)

	deadline := time.Now().Add(assetSwapLockWait)
	for !mu.TryLock() {
		if time.Now().After(deadline) {
			return fmt.Errorf("等待资产 %s 的进程内替换锁超时", name)
		}
		time.Sleep(50 * time.Millisecond)
	}
	defer mu.Unlock()

	release, err := acquireAssetSwapFileLock(name, deadline)
	if err != nil {
		logger.Warnf("跨进程资产锁不可用，降级为进程内互斥: %v", err)
	} else if release != nil {
		defer release()
	}

	return fn()
}

// ReplaceFileWithBackup 安全替换文件：目标 -> 备份(.bak)，新文件 -> 目标。
// 整个过程持有资产替换锁，避免与自愈/helper/另一实例并发写同一文件。
func ReplaceFileWithBackup(newPath, destPath string) error {
	return withAssetSwapLock(destPath, func() error {
		return replaceFileWithBackupLocked(newPath, destPath)
	})
}

func replaceFileWithBackupLocked(newPath, destPath string) error {
	backupPath := destPath + ".bak"

	// 1. 确保目标文件不再被占用
	if err := WaitFileReleased(destPath, 5*time.Second); err != nil {
		return fmt.Errorf("等待目标文件释放失败: %w", err)
	}

	// 2. 清理旧备份
	_ = os.Remove(backupPath)

	// 3. 备份当前文件
	if _, err := os.Stat(destPath); err == nil {
		if err := retryRename(destPath, backupPath, 10, 300*time.Millisecond); err != nil {
			return fmt.Errorf("备份旧文件失败: %w", err)
		}
	}

	// 4. 将新文件重命名为目标文件
	if err := retryRename(newPath, destPath, 10, 300*time.Millisecond); err != nil {
		// 替换失败：尽量恢复备份，并把 backup 路径与回滚结果一起带进错误，
		// 避免"失败原因被吞、残留 .bak 无人知"（F-A2-13）。
		restoreErr := error(nil)
		if _, statErr := os.Stat(backupPath); statErr == nil {
			restoreErr = retryRename(backupPath, destPath, 10, 300*time.Millisecond)
		}
		if restoreErr != nil {
			return fmt.Errorf("替换新文件失败且回滚未完成: %w (backup=%s, restoreErr=%v)", err, backupPath, restoreErr)
		}
		return fmt.Errorf("替换新文件失败（已回滚）: %w (backup=%s)", err, backupPath)
	}

	// 5. 替换成功，清理备份
	_ = os.Remove(backupPath)
	return nil
}
