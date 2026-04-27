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

// DownloadFileResumable 支持 HTTP Range 断点续传。
// 会将未完成内容保存在 destPath+".tmp"，并用 destPath+".tmp.meta.json" 保存 ETag/Last-Modified。
func DownloadFileResumable(ctx context.Context, url string, destPath string) error {
	return downloader.DownloadAtomic(ctx, downloader.Options{
		URL:                 url,
		DestPath:            destPath,
		MaxBytes:            500 * 1024 * 1024,
		Resume:              true,
		ResolveGitHubDigest: true,
	})
}

// downloadAppUpdateWithRetry 带自动重连的下载循环封装
func downloadAppUpdateWithRetry(ctx context.Context, url, destPath string) error {
	var lastErr error
	for i := 0; i < 5; i++ {
		lastErr = DownloadCriticalFile(ctx, url, destPath)
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
		MaxBytes: 10 * 1024 * 1024,
		Resume:   false,
	})
}

// DownloadLargeFile 用于内核/数据库等大文件的专用下载
func DownloadLargeFile(ctx context.Context, url string, destPath string) error {
	return downloader.DownloadAtomic(ctx, downloader.Options{
		URL:                 url,
		DestPath:            destPath,
		MaxBytes:            500 * 1024 * 1024,
		Resume:              true,
		ResolveGitHubDigest: true,
	})
}

// DownloadCriticalFile 用于内核、驱动、更新包等高风险组件，强制要求哈希校验
func DownloadCriticalFile(ctx context.Context, url string, destPath string) error {
	return downloader.DownloadAtomic(ctx, downloader.Options{
		URL:                 url,
		DestPath:            destPath,
		MaxBytes:            500 * 1024 * 1024,
		Resume:              true,
		ResolveGitHubDigest: true,
		TrustPolicy:         downloader.TrustRequireHash,
	})
}

// UpdateCore 安全更新内核文件（绕过正在运行的文件锁）
func UpdateCore(ctx context.Context, url string, destPath string) error {
	// 1. 🚀 核心修复：生成带有时间戳的唯一备份名，彻底避开“上次的 .old 文件仍被系统锁定”的死局
	oldPath := fmt.Sprintf("%s.%d.old", destPath, time.Now().Unix())

	// 尝试重命名。如果文件不存在（第一次安装），忽略错误
	if err := os.Rename(destPath, oldPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("内核文件被系统强力锁定，请手动关闭代理后重试: %v", err)
	}

	// 2. 此时原位置 destPath 已经空出来，安全下载新内核
	err := DownloadCriticalFile(ctx, url, destPath) // 👈 强制要求哈希校验
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
