package clash

import (
	"fmt"
	"io"
	"os"
	"time"
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

// ReplaceFileWithBackup 安全替换文件：目标 -> 备份(.bak)，新文件 -> 目标
func ReplaceFileWithBackup(newPath, destPath string) error {
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
		// 替换失败，尝试恢复备份
		if _, statErr := os.Stat(backupPath); statErr == nil {
			_ = retryRename(backupPath, destPath, 10, 300*time.Millisecond)
		}

		return fmt.Errorf("替换新文件失败: %w", err)
	}

	// 5. 替换成功，清理备份
	_ = os.Remove(backupPath)
	return nil
}

// ValidateWindowsPE 简单校验文件是否为有效的 Windows 可执行文件或 DLL
func ValidateWindowsPE(path string, minSize int64) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// 检查 MZ 头
	header := make([]byte, 2)
	if _, err := io.ReadFull(f, header); err != nil {
		return err
	}

	if header[0] != 'M' || header[1] != 'Z' {
		return fmt.Errorf("不是有效的 Windows PE 文件")
	}

	info, err := f.Stat()
	if err != nil {
		return err
	}

	if info.Size() < minSize {
		return fmt.Errorf("文件体积异常: %d bytes", info.Size())
	}

	return nil
}
