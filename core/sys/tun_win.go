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
	path := GetWintunPath()
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}

	// 校验1：大小在合理范围内 (32KB ~ 5MB)，防止 0 字节损坏文件
	if info.Size() < 32*1024 || info.Size() > 5*1024*1024 {
		return false
	}

	// 校验2：验证 PE 文件的特征码 (MZ 标识)
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	header := make([]byte, 2)
	if _, err := io.ReadFull(f, header); err != nil {
		return false
	}
	return header[0] == 'M' && header[1] == 'Z'
}

func InstallWintun(ctx context.Context, force bool) (string, error) {
	targetPath := GetWintunPath()
	fmt.Println("👉 正在重新下载并安装官方 Wintun 0.14.1 驱动...")

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
		URL:             WintunDownloadURL,
		DestPath:        zipPath,
		MaxBytes:        10 * 1024 * 1024,
		Resume:          true,
		// Wintun 官方源不是 GitHub Release Asset，无法使用 GitHub digest。
		// 此处保留大小与 PE 头基础校验。
		VerifyGitHubSHA: false, // 👈 显式关闭哈希校验
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
			// 🌟 👈 [新增]：提取出来的文件也要过一遍完整性校验，防止下载中途损坏
			if len(data) < 32*1024 || len(data) > 5*1024*1024 || data[0] != 'M' || data[1] != 'Z' {
				_ = os.Remove(tmpDllPath)
				return fmt.Errorf("解压出的 wintun.dll 校验失败: 文件不完整")
			}

			if err := downloader.ReplaceFile(tmpDllPath, finalDllPath); err != nil {
				_ = os.Remove(tmpDllPath)
				return fmt.Errorf("目标文件被占用，请关闭相关功能后重试: %w", err)
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
