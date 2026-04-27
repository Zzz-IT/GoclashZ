package sys

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"goclashz/core/downloader"
	"goclashz/core/utils"
)

const (
	WintunDownloadURL = "https://www.wintun.net/builds/wintun-0.14.1.zip"
)

func GetWintunPath() string {
	return filepath.Join(utils.GetCoreBinDir(), "wintun.dll")
}

func IsWintunInstalled() bool {
	_, err := os.Stat(GetWintunPath())
	return err == nil
}

func InstallWintun(ctx context.Context, force bool) (string, error) {
	if !force && IsWintunInstalled() {
		return "ALREADY_LATEST", nil
	}

	targetPath := GetWintunPath()
	fmt.Printf("👉 正在%s Wintun 驱动...\n", func() string {
		if force {
			return "重新下载并覆盖"
		}
		return "自动下载官方"
	}())

	if err := downloadAndExtractWintun(ctx, targetPath); err != nil {
		return "", fmt.Errorf("Wintun 驱动安装失败: %v", err)
	}

	return "SUCCESS", nil
}

func downloadAndExtractWintun(ctx context.Context, finalDllPath string) error {
	destDir := filepath.Dir(finalDllPath)
	os.MkdirAll(destDir, 0755)
	zipPath := filepath.Join(destDir, "wintun_temp.zip")
	var err error

	// 1. 下载 ZIP (使用统一原子下载器)
	err = downloader.DownloadAtomic(ctx, downloader.Options{
		URL:         WintunDownloadURL,
		DestPath:    zipPath,
		MaxBytes:    10 * 1024 * 1024,
		Resume:      true,
		TrustPolicy: downloader.TrustRequireHash,
	})
	if err != nil {
		return fmt.Errorf("下载 Wintun 驱动失败: %v", err)
	}

	// 2. 解压并提取
	var r *zip.ReadCloser
	defer func() {
		if r != nil {
			r.Close()
		}
		os.Remove(zipPath)
	}()

	r, err = zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("解压失败: %v", err)
	}

	found := false
	for _, f := range r.File {
		if strings.HasSuffix(strings.ToLower(f.Name), "amd64/wintun.dll") {
			rc, err := f.Open()
			if err != nil {
				return err
			}

			tmpDllPath := finalDllPath + ".tmp"
			_ = os.Remove(tmpDllPath)

			outFile, err := os.Create(tmpDllPath)
			if err != nil {
				rc.Close()
				return err
			}

			_, copyErr := io.Copy(outFile, rc)
			closeErr := outFile.Close()
			rc.Close()

			if copyErr != nil {
				_ = os.Remove(tmpDllPath)
				return copyErr
			}
			if closeErr != nil {
				_ = os.Remove(tmpDllPath)
				return closeErr
			}

			data, err := os.ReadFile(tmpDllPath)
			if err != nil {
				_ = os.Remove(tmpDllPath)
				return err
			}
			if len(data) < 32*1024 || len(data) > 5*1024*1024 || data[0] != 'M' || data[1] != 'Z' {
				_ = os.Remove(tmpDllPath)
				return fmt.Errorf("wintun.dll 校验失败")
			}

			if err := os.Rename(tmpDllPath, finalDllPath); err != nil {
				_ = os.Remove(tmpDllPath)
				return err
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("在压缩包中未找到适配 64 位系统的驱动文件")
	}

	fmt.Println("✅ Wintun 驱动提取并安装成功！")
	return nil
}
