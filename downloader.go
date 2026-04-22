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
