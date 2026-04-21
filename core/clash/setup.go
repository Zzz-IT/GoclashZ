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
)

func PrepareEnv(dirPath string, exePath string) error {
	// 1. 创建目录
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		os.MkdirAll(dirPath, 0755)
	}

	// 2. 检查 clash.exe (缺失则下载内核)
	if _, err := os.Stat(exePath); os.IsNotExist(err) {
		fmt.Println("👉 未检测到内核，正在自动下载...")
		if err := downloadAndExtractKernel(dirPath, exePath); err != nil {
			return err
		}
	}

	// 3. 移除强制下载 wintun.dll 的逻辑，取消启动时对 TUN 的干预。

	// 4. 检查配置文件
	configPath := filepath.Join(dirPath, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Println("👉 未检测到配置文件，正在生成默认极简配置...")
		baseConfig := `mixed-port: 7890
allow-lan: false
mode: rule
log-level: info
`
		err := os.WriteFile(configPath, []byte(baseConfig), 0644)
		if err != nil {
			return fmt.Errorf("生成配置文件失败: %v", err)
		}
		fmt.Println("✅ 默认配置生成完毕！")
	}

	return nil
}

func downloadAndExtractKernel(destDir, finalExePath string) error {
	url := "https://ghproxy.net/https://github.com/MetaCubeX/mihomo/releases/download/v1.18.3/mihomo-windows-amd64-v1.18.3.zip"
	zipPath := filepath.Join(destDir, "core.zip")

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 GoclashZ/1.0")

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

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		os.Remove(zipPath)
		return fmt.Errorf("解压失败: %v", err)
	}

	for _, f := range r.File {
		if strings.HasSuffix(f.Name, ".exe") {
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
			_, err = io.Copy(outFile, rc)
			outFile.Close()
			rc.Close()
			if err != nil {
				r.Close()
				return err
			}
			break
		}
	}
	
	r.Close()
	os.Remove(zipPath)
	return nil
}
