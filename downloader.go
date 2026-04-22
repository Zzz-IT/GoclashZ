package main

import (
	"io"
	"net/http"
	"os"
	"time"
)

// 🚀 1. 定义全局带超时的 HTTP 客户端
// 彻底解决机场节点卡死导致前端“无限转圈”的问题
var httpClient = &http.Client{
	Timeout: 30 * time.Second, 
}

// DownloadFile 安全地下载文件（防损坏）
func DownloadFile(url string, destPath string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	
	// 伪装 User-Agent，防止被某些严格的机场 WAF 拦截
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GoclashZ/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 🚀 2. 原子写入机制 (Atomic Write)
	// 先将文件下载为 .tmp 临时文件
	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, resp.Body)
	out.Close() // 必须先关闭文件句柄，否则 Windows 下无法重命名

	// 如果下载过程中途断网或发生错误，清理残缺的临时文件，保护原配置不被破坏
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	// 下载完整后，瞬间覆盖原文件（操作系统级原子操作，绝对安全）
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
	err := DownloadFile(url, destPath) // 调用我们之前写好的带原子写入的 DownloadFile
	if err != nil {
		// 🚨 兜底机制：如果新内核下载失败或损坏，把旧内核的名字改回来，保证软件还能用
		_ = os.Rename(oldPath, destPath)
		return err
	}

	return nil
}
