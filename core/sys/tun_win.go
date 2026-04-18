package sys

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// 使用 Wintun 官方稳定的 release 压缩包
	WintunDownloadURL = "https://www.wintun.net/builds/wintun-0.14.1.zip"
)

// getWintunPath 获取驱动应该存放的绝对路径 (与 clash.exe 同级)
func GetWintunPath() string {
	exePath, _ := os.Executable()
	return filepath.Join(filepath.Dir(exePath), "core", "bin", "wintun.dll")
}

// IsWintunInstalled 检查驱动是否存在
func IsWintunInstalled() bool {
	_, err := os.Stat(GetWintunPath())
	return err == nil || !os.IsNotExist(err)
}

// InstallWintun 安装驱动（不再依赖外部下载器，直接处理 ZIP 解压）
func InstallWintun() error {
	if IsWintunInstalled() {
		return nil
	}

	targetPath := GetWintunPath()
	fmt.Println("👉 未检测到 Wintun 驱动，正在自动下载官方组件包...")

	if err := downloadAndExtractWintun(targetPath); err != nil {
		return fmt.Errorf("Wintun 驱动安装失败: %v", err)
	}

	return nil
}

// 核心功能：下载官方 ZIP 并提取对应的 dll 文件
func downloadAndExtractWintun(finalDllPath string) error {
	destDir := filepath.Dir(finalDllPath)
	os.MkdirAll(destDir, 0755)
	zipPath := filepath.Join(destDir, "wintun_temp.zip")

	// 1. 下载 ZIP
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("GET", WintunDownloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) goclashz")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("网络请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器拒绝了请求，状态码: %d", resp.StatusCode)
	}

	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, resp.Body)
	out.Close()

	if err != nil {
		os.Remove(zipPath)
		return fmt.Errorf("数据流接收异常中断: %v", err)
	}

	// 2. 解压并寻找 amd64 架构的 wintun.dll
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		os.Remove(zipPath)
		return fmt.Errorf("解压失败: %v", err)
	}
	defer r.Close()

	found := false
	for _, f := range r.File {
		// 官方压缩包内包含多种架构，我们要精确提取 amd64 版本
		// 路径格式通常为 wintun/bin/amd64/wintun.dll
		if strings.HasSuffix(strings.ToLower(f.Name), "amd64/wintun.dll") {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			outFile, err := os.Create(finalDllPath)
			if err != nil {
				rc.Close()
				return err
			}
			_, err = io.Copy(outFile, rc)
			outFile.Close()
			rc.Close()
			
			if err != nil {
				return err
			}
			found = true
			break
		}
	}
	
	// 3. 清理临时压缩包
	os.Remove(zipPath)

	if !found {
		return fmt.Errorf("在压缩包中未找到适配 64 位系统的驱动文件")
	}

	fmt.Println("✅ Wintun 驱动提取并安装成功！")
	return nil
}
