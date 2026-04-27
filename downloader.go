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

// DownloadFileResumable 支持 HTTP Range 断点续传和 GitHub 自动校验
func DownloadFileResumable(ctx context.Context, url string, destPath string) error {
	return downloader.DownloadAtomic(ctx, downloader.Options{
		URL:             url,
		DestPath:        destPath,
		MaxBytes:        500 * 1024 * 1024,
		Resume:          true,
		VerifyGitHubSHA: downloader.ShouldVerifyGitHubSHA(url),
	})
}

// downloadAppUpdateWithRetry 带自动重连的下载循环封装
func downloadAppUpdateWithRetry(ctx context.Context, url, destPath string) error {
	var lastErr error
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

// DownloadFile 安全地下载普通文件 (用于订阅，不校验 GitHub SHA)
func DownloadFile(ctx context.Context, url string, destPath string) error {
	return downloader.DownloadAtomic(ctx, downloader.Options{
		URL:             url,
		DestPath:        destPath,
		MaxBytes:        10 * 1024 * 1024,
		Resume:          false,
		VerifyGitHubSHA: false,
	})
}

// DownloadLargeFile 用于内核/数据库等大文件的专用下载
func DownloadLargeFile(ctx context.Context, url string, destPath string) error {
	return downloader.DownloadAtomic(ctx, downloader.Options{
		URL:             url,
		DestPath:        destPath,
		MaxBytes:        500 * 1024 * 1024,
		Resume:          true,
		VerifyGitHubSHA: downloader.ShouldVerifyGitHubSHA(url),
	})
}

// DownloadCriticalFile 对高风险组件强制进行 GitHub 校验
func DownloadCriticalFile(ctx context.Context, url string, destPath string) error {
	return downloader.DownloadAtomic(ctx, downloader.Options{
		URL:             url,
		DestPath:        destPath,
		MaxBytes:        500 * 1024 * 1024,
		Resume:          true,
		VerifyGitHubSHA: true, // 强制校验
	})
}

// UpdateCore 安全更新内核文件
func UpdateCore(ctx context.Context, url string, destPath string) error {
	oldPath := fmt.Sprintf("%s.%d.old", destPath, time.Now().Unix())

	if err := os.Rename(destPath, oldPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("内核文件被锁定: %v", err)
	}

	err := DownloadCriticalFile(ctx, url, destPath)
	if err != nil {
		_ = os.Rename(oldPath, destPath)
		return err
	}

	go cleanOldKernels(filepath.Dir(destPath))
	return nil
}

func cleanOldKernels(dir string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".old") {
			_ = os.Remove(filepath.Join(dir, f.Name()))
		}
	}
}
