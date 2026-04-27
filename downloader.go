package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// DownloadFileResumable 支持断点续传的大文件下载器
func DownloadFileResumable(ctx context.Context, url string, destPath string) error {
	tmpPath := destPath + ".tmp"

	var downloaded int64
	// 检查是否存在未下载完的临时文件，获取已下载的字节数
	if info, err := os.Stat(tmpPath); err == nil {
		downloaded = info.Size()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "GoclashZ-Updater/1.0")

	// 如果有部分文件，发起 Range 请求断点续传
	if downloaded > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", downloaded))
	}

	resp, err := largeFileClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	flags := os.O_CREATE | os.O_WRONLY
	switch resp.StatusCode {
	case http.StatusPartialContent:
		// 服务器支持断点续传 (206)，追加写入
		flags |= os.O_APPEND
	case http.StatusOK:
		// 服务器不支持断点续传或文件变更 (200)，从头开始覆写
		flags |= os.O_TRUNC
		downloaded = 0
	default:
		return fmt.Errorf("下载请求异常，状态码: %d", resp.StatusCode)
	}

	// ✅ 修改后：使用匿名闭包强制进行 defer 释放，防止 io.Copy panic 导致句柄泄露
	err = func() error {
		out, err := os.OpenFile(tmpPath, flags, 0644)
		if err != nil {
			return err
		}
		// 闭包退出时必然执行，即便是 io.Copy 内部发生了 panic
		defer out.Close()
		_, err = io.Copy(out, resp.Body)
		return err
	}()

	if err != nil {
		return fmt.Errorf("下载流被中断: %v", err)
	}

	// 彻底下载成功，安全覆盖目标文件
	os.Remove(destPath)
	return os.Rename(tmpPath, destPath)
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

// 针对订阅等轻量级请求 (30秒超时防卡死)
var httpClient = &http.Client{
	Timeout: 30 * time.Second, 
}

// 🚀 新增：针对内核/数据库等大文件的专用客户端 (10分钟超时)
var largeFileClient = &http.Client{
	Timeout: 10 * time.Minute, 
}

// DownloadFile 安全地下载普通文件 (用于订阅)
func DownloadFile(ctx context.Context, url string, destPath string) error {
    return doDownload(ctx, httpClient, url, destPath)
}

// 🚀 新增：用于 app.go 和 downloader.go 中的 UpdateCore / Geo 数据库下载
func DownloadLargeFile(ctx context.Context, url string, destPath string) error {
    return doDownload(ctx, largeFileClient, url, destPath)
}

// 提取底层的原子下载逻辑
func doDownload(ctx context.Context, client *http.Client, url string, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GoclashZ/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 🚀 补上状态码校验：防止下载到 404/500 等 HTML 报错页面并将其保存为配置/数据库
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("请求失败，服务器返回 HTTP 状态码: %d", resp.StatusCode)
	}

	tmpPath := destPath + ".tmp"
	// ✅ 修改后：使用匿名闭包强制进行 defer 释放
	err = func() error {
		out, err := os.Create(tmpPath)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, resp.Body)
		return err
	}()

	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, destPath)
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
