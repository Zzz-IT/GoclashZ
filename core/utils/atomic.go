//go:build windows

package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}

	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("atomic rename failed: %w", err)
	}

	cleanup = false
	return nil
}

// ErrAssetBusy 表示目标文件正被其它进程占用（典型场景：clash.exe 已把 wintun.dll
// 以 image 方式映射）。调用方可用 errors.Is 把它与真正的 IO 错误区分开，据此选择
// "稍后重试/待替换"而不是直接报故障。
var ErrAssetBusy = errors.New("asset busy: 目标文件被占用")

// CopyFile 以"临时文件 + rename"的方式原子替换 dst。
//
// 注意：Windows 不允许替换被 image 映射（或被独占打开）的文件，所以先探测可写性并
// 短暂重试；临时文件名带随机后缀，避免并发复制同一目标时互相截断（历史缺陷：固定
// 的 dst+".tmp"）。
func CopyFile(src, dst string) error {
	return CopyFileContext(context.Background(), src, dst)
}

// CopyFileContext 与 CopyFile 相同，但在"等待目标释放"和"逐块复制"两处都响应 ctx：
// 取消时返回包装了 ErrCopyCanceled 的错误，并清理临时文件（不会留下半截文件）。
// 供自愈路径使用——watchdog/UI 的预算不该被一次 61MB 复制无视。
func CopyFileContext(ctx context.Context, src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	// 目标被占用时，写完临时文件再 rename 也必然失败（白读几十 MB），先探测 + 重试。
	if err := waitReplaceableContext(ctx, dst, 6, 250*time.Millisecond); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmpFile, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".*.tmp")
	if err != nil {
		return err
	}

	tmp := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()

	if err := tmpFile.Chmod(0755); err != nil {
		_ = tmpFile.Close()
		return err
	}

	if _, err := copyWithContext(ctx, tmpFile, in); err != nil {
		_ = tmpFile.Close()
		return err
	}

	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}

	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("%w: 替换 %s 失败: %v", ErrAssetBusy, dst, err)
	}

	cleanup = false
	return nil
}

// ErrCopyCanceled 表示复制被 ctx 取消/超时中断（不是 IO 故障，可安全重试）。
var ErrCopyCanceled = errors.New("copy canceled")

// copyWithContext 分块复制（1MB/块），每块前检查 ctx，保证大文件复制可被预算打断。
func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 1<<20)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, fmt.Errorf("%w: %v", ErrCopyCanceled, err)
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return written, writeErr
			}
			written += int64(n)
		}
		if readErr == io.EOF {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}

// waitReplaceable 探测 dst 当前能否被替换：能被独占（O_RDWR）打开说明没有进程以
// 不共享方式占用它。目标不存在视为可直接创建。
func waitReplaceable(dst string, attempts int, delay time.Duration) error {
	return waitReplaceableContext(context.Background(), dst, attempts, delay)
}

// waitReplaceableContext 是 waitReplaceable 的 ctx 感知版本：预算内每 delay 重试一次，
// ctx 取消时立即返回 ErrCopyCanceled（不把"用户/预算已放弃"伪装成"文件忙"）。
func waitReplaceableContext(ctx context.Context, dst string, attempts int, delay time.Duration) error {
	st, err := os.Stat(dst)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// 目标是目录：不是"忙"，重试也没有意义（避免白等 1.25s 并把 ErrIsDir 误报成 busy）。
	if st.IsDir() {
		return fmt.Errorf("目标路径是目录，无法替换: %s", dst)
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		f, err := os.OpenFile(dst, os.O_RDWR, 0)
		if err == nil {
			_ = f.Close()
			return nil
		}
		lastErr = err
		if i >= attempts-1 {
			break
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("%w: 等待 %s 释放时被取消: %v", ErrCopyCanceled, filepath.Base(dst), ctx.Err())
		case <-timer.C:
		}
	}

	return fmt.Errorf("%w: %s 暂时不可替换: %v", ErrAssetBusy, dst, lastErr)
}
