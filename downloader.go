package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// ==========================================
// --- 软件本体更新专用下载器 (支持断点续传) ---
// ==========================================

// DownloadFileResumable 支持断点续传的大文件下载器
func DownloadFileResumable(url string, destPath string) error {
	tmpPath := destPath + ".tmp"

	var downloaded int64
	// 检查是否存在未下载完的临时文件，获取已下载的字节数
	if info, err := os.Stat(tmpPath); err == nil {
		downloaded = info.Size()
	}

	req, err := http.NewRequest("GET", url, nil)
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

	out, err := os.OpenFile(tmpPath, flags, 0644)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, resp.Body)
	out.Close() // 必须先关闭文件，才能在后面执行重命名

	if err != nil {
		return fmt.Errorf("下载流被中断: %v", err)
	}

	// 彻底下载成功，安全覆盖目标文件
	os.Remove(destPath)
	return os.Rename(tmpPath, destPath)
}

// downloadAppUpdateWithRetry 带自动重连的下载循环封装
func downloadAppUpdateWithRetry(url, destPath string) error {
	var lastErr error
	// 断网或异常时，最多尝试续传重连 5 次，总共容忍约 15 秒的网络波动
	for i := 0; i < 5; i++ {
		lastErr = DownloadFileResumable(url, destPath)
		if lastErr == nil {
			return nil
		}
		time.Sleep(3 * time.Second)
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
func DownloadFile(url string, destPath string) error {
    return doDownload(httpClient, url, destPath)
}

// 🚀 新增：用于 app.go 和 downloader.go 中的 UpdateCore / Geo 数据库下载
func DownloadLargeFile(url string, destPath string) error {
    return doDownload(largeFileClient, url, destPath)
}

// 提取底层的原子下载逻辑
func doDownload(client *http.Client, url string, destPath string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GoclashZ/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, resp.Body)
	out.Close() 

	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, destPath)
}

// UpdateCore 安全更新内核文件（绕过正在运行的文件锁）
func UpdateCore(url string, destPath string) error {
	// 1. 🚀 核心修复：将正在运行的内核重命名为 .old 
	// Windows 允许重构正在运行的可执行文件，但不允许删除或修改内容。
	oldPath := destPath + ".old"
	_ = os.Remove(oldPath) // 先清理掉上一次可能残留的 .old 文件

	// 尝试重命名。如果文件不存在（第一次安装），忽略错误
	if err := os.Rename(destPath, oldPath); err != nil && !os.IsNotExist(err) {
		return err // 如果重命名失败（可能权限不足），直接返回错误，保护原文件
	}

	// 2. 此时原位置 destPath 已经空出来了，安全下载新内核
	err := DownloadLargeFile(url, destPath) // 👈 替换为大文件专用方法
	if err != nil {
		// 🚨 兜底机制：如果新内核下载失败或损坏，把旧内核的名字改回来，保证软件还能用
		_ = os.Rename(oldPath, destPath)
		return err
	}

	return nil
}
