package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"goclashz/core/downloader"
)

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// ==========================================
// --- 软件本体更新专用下载器 (支持断点续传) ---
// ==========================================

// DownloadFileResumable 支持断点续传的大文件下载器 (目前底层通过重试模拟)
func DownloadFileResumable(ctx context.Context, url string, destPath string) error {
	// 软件本体更新专用：使用 10 分钟超时客户端
	return downloader.DownloadLargeAtomic(ctx, url, destPath, "", 0)
}

// downloadAppUpdateWithRetry 带自动重连的下载循环封装
func downloadAppUpdateWithRetry(ctx context.Context, url, destPath string) error {
	var lastErr error
	// 断网或异常时，最多尝试续传重连 5 次，总共容忍约 15 秒的网络波动
	for i := 0; i < 5; i++ {
		lastErr = DownloadFileResumable(ctx, url, destPath)
		if lastErr == nil {
			return nil
		}
		if !sleepOrDone(ctx, 3*time.Second) {
			return ctx.Err()
		}
	}
	return fmt.Errorf("下载多次中断且重连均失败: %v", lastErr)
}

// DownloadFile 安全地下载普通文件 (用于订阅)
func DownloadFile(ctx context.Context, url string, destPath string) error {
	return downloader.DownloadAtomic(ctx, downloader.Options{
		URL:      url,
		DestPath: destPath,
		MaxBytes: 10 * 1024 * 1024, // 订阅文件限制 10MB
	})
}

// DownloadLargeFile 用于内核/数据库等大文件的专用下载
func DownloadLargeFile(ctx context.Context, url string, destPath string) error {
	return downloader.DownloadLargeAtomic(ctx, url, destPath, "", 0)
}

// UpdateCore 安全更新内核文件（绕过正在运行的文件锁）
func UpdateCore(ctx context.Context, url string, destPath string) error {
	// 1. 🚀 核心修复：生成带有时间戳的唯一备份名，彻底避开“上次的 .old 文件仍被系统锁定”的死局
	oldPath := fmt.Sprintf("%s.%d.old", destPath, time.Now().Unix())

	// 尝试重命名。如果文件不存在（第一次安装），忽略错误
	if err := os.Rename(destPath, oldPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("内核文件被系统强力锁定，请手动关闭代理后重试: %v", err)
	}

	// 2. 此时原位置 destPath 已经空出来了，安全下载新内核
	err := DownloadLargeFile(ctx, url, destPath) // 👈 替换为大文件专用方法
	if err != nil {
		// 🚨 兜底机制：如果新内核下载失败或损坏，把旧内核的名字改回来，保证软件还能用
		_ = os.Rename(oldPath, destPath)
		return err
	}

	// 3. 🚀 核心修复：启动后台静默协程，清理以前遗留的所有 .old 垃圾文件
	go cleanOldKernels(filepath.Dir(destPath))

	return nil
}

// 异步垃圾清理函数
func cleanOldKernels(dir string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".old") {
			// 尝试删除，如果被杀软锁定就忽略，等下次软件启动再删
			_ = os.Remove(filepath.Join(dir, f.Name()))
		}
	}
}
