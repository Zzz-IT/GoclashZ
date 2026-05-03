//go:build windows

package downloader

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DownloadAppUpdate 使用通用下载机下载应用更新包
func DownloadAppUpdate(ctx context.Context, info *AppUpdateInfo, destDir string) (string, error) {
	if info == nil {
		return "", fmt.Errorf("更新信息为空")
	}
	if strings.TrimSpace(info.DownloadURL) == "" {
		return "", fmt.Errorf("没有可用的应用更新下载地址")
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}

	fileName := sanitizeUpdateAssetName(info.AssetName)
	if fileName == "" {
		// 兜底方案
		fileName = fmt.Sprintf("GoclashZ_%s_Setup.exe", strings.TrimPrefix(info.Version, "v"))
	}

	destPath := filepath.Join(destDir, fileName)

	// 🚀 复用原子下载机
	err := DownloadAtomic(ctx, Options{
		URLs:      []string{info.DownloadURL},
		DestPath:  destPath,
		UserAgent: "GoclashZ-Updater",
		MaxBytes:  300 << 20, // 限制 300MB
		Resume:    true,
		Validator: func(tmpPath string) error {
			return ValidateWindowsExecutable(tmpPath)
		},
	})
	if err != nil {
		return "", err
	}

	return destPath, nil
}

// ValidateWindowsExecutable 验证是否为有效的 Windows 可执行文件
func ValidateWindowsExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	// 最小体积校验 (1MB)
	if info.Size() < 1024*1024 {
		return fmt.Errorf("更新包体积异常 (小于 1MB)")
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// 读取前两个字节检查 "MZ" 头
	header := make([]byte, 2)
	if _, err := io.ReadFull(f, header); err != nil {
		return err
	}

	if string(header) != "MZ" {
		return fmt.Errorf("更新包不是有效的 Windows 可执行文件 (缺少 MZ 标识)")
	}

	return nil
}

func sanitizeUpdateAssetName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == "" {
		return ""
	}
	// 只允许 .exe
	if !strings.HasSuffix(strings.ToLower(name), ".exe") {
		return ""
	}
	return name
}
