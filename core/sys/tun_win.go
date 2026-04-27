package sys

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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

	// 1. 下载 ZIP (优化请求与接收流)
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", WintunDownloadURL, nil)
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
	// 使用缓冲拷贝提高磁盘写入速度
	buf := make([]byte, 32*1024)
	_, err = io.CopyBuffer(out, resp.Body, buf)
	out.Close()

	if err != nil {
		os.Remove(zipPath)
		return fmt.Errorf("数据流接收异常中断: %v", err)
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

			_, copyErr := io.CopyBuffer(outFile, rc, buf)
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
