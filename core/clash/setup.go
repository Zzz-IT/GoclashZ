package clash

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"goclashz/core/utils"
)

// 🎯 核心修复：移除 defer，手动释放句柄，解决 core.zip 删不掉的 Bug
func downloadAndExtractKernel(destDir, finalExePath string) error {
	kernelURL := "https://ghproxy.net/https://github.com/MetaCubeX/mihomo/releases/download/v1.18.3/mihomo-windows-amd64-v1.18.3.zip"
	zipPath := filepath.Join(destDir, "core.zip")

	// 1. 下载
	resp, err := http.Get(kernelURL)
	if err != nil {
		return fmt.Errorf("下载内核失败: %v", err)
	}
	defer resp.Body.Close()

	out, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("创建压缩包失败: %v", err)
	}
	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		return fmt.Errorf("写入压缩包失败: %v", err)
	}

	// 2. 解压
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		os.Remove(zipPath)
		return fmt.Errorf("解压读取失败: %v", err)
	}
	// ❌ 这里千万不能写 defer r.Close()

	for _, f := range r.File {
		if strings.HasSuffix(f.Name, ".exe") {
			rc, err := f.Open()
			if err != nil {
				r.Close() // 提前释放
				return err
			}
			
			outFile, err := os.Create(finalExePath)
			if err != nil {
				rc.Close()
				r.Close() // 提前释放
				return err
			}
			
			_, err = io.Copy(outFile, rc)
			outFile.Close()
			rc.Close()
			
			if err != nil {
				r.Close() // 提前释放
				return err
			}
			break
		}
	}

	// ✅ 手动关闭压缩包句柄，释放 Windows 文件锁
	r.Close()
	
	// 现在可以100%成功删除 core.zip 了
	os.Remove(zipPath) 
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
