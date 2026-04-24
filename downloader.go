package main

import (
	"io"
	"net/http"
	"os"
	"time"
)

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
