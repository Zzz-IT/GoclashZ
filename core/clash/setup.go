package clash

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"goclashz/core/utils"
)

// 🎯 优化：复用 Wintun 风格的缓冲拷贝逻辑，提升大文件处理速度并增加请求稳定性
func downloadAndExtractKernel(destDir, finalExePath string) error {
	kernelURL := "https://ghproxy.net/https://github.com/MetaCubeX/mihomo/releases/download/v1.18.3/mihomo-windows-amd64-v1.18.3.zip"
	zipPath := filepath.Join(destDir, "core_temp.zip")

	// 1. 下载 ZIP (优化请求与接收流)
	client := &http.Client{Timeout: 120 * time.Second} // 内核稍大，给 2 分钟超时
	req, err := http.NewRequest("GET", kernelURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) goclashz")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载内核网络错误: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器返回错误状态码: %d", resp.StatusCode)
	}

	out, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %v", err)
	}

	// 使用缓冲拷贝提高磁盘写入速度
	buf := make([]byte, 32*1024)
	_, err = io.CopyBuffer(out, resp.Body, buf)
	out.Close()
	if err != nil {
		os.Remove(zipPath)
		return fmt.Errorf("写入压缩包异常中断: %v", err)
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

			outFile, err := os.Create(finalExePath)
			if err != nil {
				rc.Close()
				r.Close()
				return err
			}

			// 再次使用缓冲提升解压释放速度
			_, err = io.CopyBuffer(outFile, rc, buf)
			outFile.Close()
			rc.Close()

			if err != nil {
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
func PrepareEnv() error {
	binDir := utils.GetCoreBinDir() // 取向安全的 DataDir
	exePath := filepath.Join(binDir, "clash.exe")
	configPath := filepath.Join(utils.GetDataDir(), "config.yaml")

	if _, err := os.Stat(binDir); os.IsNotExist(err) {
		os.MkdirAll(binDir, 0755)
	}

	if _, err := os.Stat(exePath); os.IsNotExist(err) {
		fmt.Println("👉 未检测到内核，正在自动下载至安全目录...")
		if err := downloadAndExtractKernel(binDir, exePath); err != nil {
			return err
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
	if _, err := os.Stat(wintunPath); os.IsNotExist(err) {
		fmt.Println("👉 未检测到 wintun.dll，正在下载以支持 TUN 模式...")
		if err := downloadWintun(binDir); err != nil {
			fmt.Printf("⚠️ wintun.dll 下载失败 (TUN 模式将不可用): %v\n", err)
		} else {
			fmt.Println("✅ wintun.dll 准备就绪")
		}
	}

	return nil
}

func downloadWintun(destDir string) error {
	wintunURL := "https://ghproxy.net/https://github.com/Zzz-IT/GoclashZ/releases/download/v0.0.1/wintun.dll"
	resp, err := http.Get(wintunURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(filepath.Join(destDir, "wintun.dll"))
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// ExtractKernel 纯粹的辅助函数：从 ZIP 中抽取内核 .exe 并保存
func ExtractKernel(zipPath, destExePath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	buf := make([]byte, 32*1024)
	for _, f := range r.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".exe") {
			rc, err := f.Open()
			if err != nil {
				return err
			}

			outFile, err := os.Create(destExePath)
			if err != nil {
				rc.Close()
				return err
			}

			_, err = io.CopyBuffer(outFile, rc, buf)
			outFile.Close()
			rc.Close()

			return err
		}
	}
	return fmt.Errorf("未在压缩包中找到 .exe 文件")
}
