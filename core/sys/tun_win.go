package sys

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// 使用 GitHub 标准 raw 路由，方便镜像站加速
	WintunDownloadURL = "https://github.com/MetaCubeX/mihomo/raw/Meta/docs/wintun.dll"
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

// InstallWintun 安装驱动（需配合 downloader.go 中的 downloadFileWithRetry 使用）
func InstallWintun(downloadFunc func(string, string) error) error {
	if IsWintunInstalled() {
		return nil
	}

	targetPath := GetWintunPath()
	
	// 调用外部传入的健壮下载器
	err := downloadFunc(targetPath, WintunDownloadURL)
	if err != nil {
		return fmt.Errorf("Wintun 驱动下载失败: %v", err)
	}

	return nil
}
