package clash

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"goclashz/core/downloader"
	"goclashz/core/sys"
	"goclashz/core/utils"
)

const (
	minKernelSize = 1024 * 1024 // 1MB
	minWintunSize = 32 * 1024   // 32KB
	maxWintunSize = 5 * 1024 * 1024
)

func isUsableFile(path string, minSize int64) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Size() >= minSize
}

func looksLikePE(data []byte) bool {
	return len(data) >= 2 && data[0] == 'M' && data[1] == 'Z'
}

const (
	kernelURL = "https://ghproxy.net/https://github.com/MetaCubeX/mihomo/releases/download/v1.18.3/mihomo-windows-amd64-v1.18.3.zip"
)

func downloadAndExtractKernel(ctx context.Context, destDir, finalExePath string) error {
	zipPath := filepath.Join(destDir, "core_temp.zip")

	// 1. 下载 ZIP (使用统一原子下载器 + GitHub 自动校验)
	err := downloader.DownloadAtomic(ctx, downloader.Options{
		URL:             kernelURL,
		DestPath:        zipPath,
		MaxBytes:        200 * 1024 * 1024,
		Resume:          true,
		VerifyGitHubSHA: true, // 强制内核校验
	})
	if err != nil {
		return fmt.Errorf("内核下载或 GitHub 校验失败: %v", err)
	}

	// 2. 解压并提取 (修复解除文件锁定逻辑)
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		os.Remove(zipPath)
		return fmt.Errorf("解压读取失败: %v", err)
	}

	found := false
	for _, f := range r.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".exe") {
			rc, err := f.Open()
			if err != nil {
				r.Close()
				return err
			}

			tmpExePath := finalExePath + ".tmp"
			_ = os.Remove(tmpExePath)
			outFile, err := os.Create(tmpExePath)
			if err != nil {
				rc.Close()
				r.Close()
				return err
			}

			// 再次使用缓冲提升解压释放速度
			_, err = io.Copy(outFile, rc)
			outFile.Close()
			rc.Close()

			if err != nil {
				_ = os.Remove(tmpExePath)
				r.Close()
				return err
			}

			if !isUsableFile(tmpExePath, minKernelSize) {
				_ = os.Remove(tmpExePath)
				r.Close()
				return fmt.Errorf("内核解压后校验失败：文件过小或损坏")
			}

			if err := os.Rename(tmpExePath, finalExePath); err != nil {
				_ = os.Remove(tmpExePath)
				r.Close()
				return err
			}
			found = true
			break
		}
	}

	// 3. 安全清理临时压缩包 (必须先释放 Reader)
	r.Close()
	os.Remove(zipPath)

	if !found {
		return fmt.Errorf("在内核压缩包中未找到 .exe 执行文件")
	}

	return nil
}

// PrepareEnv 检查内核并生成基础配置
func PrepareEnv(ctx context.Context) error {
	binDir := utils.GetCoreBinDir() // 取向安全的 DataDir
	exePath := filepath.Join(binDir, "clash.exe")
	configPath := filepath.Join(utils.GetDataDir(), "config.yaml")

	if _, err := os.Stat(binDir); os.IsNotExist(err) {
		os.MkdirAll(binDir, 0755)
	}

	if !isUsableFile(exePath, minKernelSize) {
		fmt.Println("👉 未检测到内核或内核已损坏，正在自动下载至安全目录...")
		_ = os.Remove(exePath)
		if err := downloadAndExtractKernel(ctx, binDir, exePath); err != nil {
			return err
		}
		if !isUsableFile(exePath, minKernelSize) {
			_ = os.Remove(exePath)
			return fmt.Errorf("内核下载后校验失败：文件过小或损坏")
		}
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Println("👉 生成默认极简配置...")
		baseConfig := `mixed-port: 7890
allow-lan: false
mode: rule
log-level: info
`
		os.WriteFile(configPath, []byte(baseConfig), 0644)
	}

	// 5. 检查并下载 wintun.dll
	wintunPath := filepath.Join(binDir, "wintun.dll")
	if !isUsableFile(wintunPath, minWintunSize) {
		fmt.Println("👉 未检测到 wintun.dll 或文件已损坏，正在下载以支持 TUN 模式...")
		_ = os.Remove(wintunPath)
		if _, err := sys.InstallWintun(ctx, false); err != nil {
			fmt.Printf("⚠️ wintun.dll 下载失败 (TUN 模式将不可用): %v\n", err)
		} else {
			fmt.Println("✅ wintun.dll 准备就绪")
		}
	}

	return nil
}


// ExtractKernel 纯粹的辅助函数：从 ZIP 中抽取内核 .exe 并保存
func ExtractKernel(zipPath, destExePath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	buf := make([]byte, 32*1024)
	tmpPath := destExePath + ".tmp"
	_ = os.Remove(tmpPath)

	for _, f := range r.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".exe") {
			rc, err := f.Open()
			if err != nil {
				return err
			}

			outFile, err := os.Create(tmpPath)
			if err != nil {
				rc.Close()
				return err
			}

			_, copyErr := io.CopyBuffer(outFile, rc, buf)
			closeErr := outFile.Close()
			rc.Close()

			if copyErr != nil {
				_ = os.Remove(tmpPath)
				return copyErr
			}
			if closeErr != nil {
				_ = os.Remove(tmpPath)
				return closeErr
			}

			if !isUsableFile(tmpPath, minKernelSize) {
				_ = os.Remove(tmpPath)
				return fmt.Errorf("内核解压后校验失败：文件过小或损坏")
			}

			if err := os.Rename(tmpPath, destExePath); err != nil {
				_ = os.Remove(tmpPath)
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("未在压缩包中找到 .exe 文件")
}
