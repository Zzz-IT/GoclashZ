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

// PrepareEnv 检查并准备运行环境（自动下载内核、生成初始配置）
func PrepareEnv(dirPath string, exePath string) error {
	// 1. 如果目录不存在，自动创建 core/bin 文件夹
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		err := os.MkdirAll(dirPath, 0755)
		if err != nil {
			return fmt.Errorf("创建目录失败: %v", err)
		}
	}

	// 2. 检查 clash.exe 是否存在，不存在则自动下载
	if _, err := os.Stat(exePath); os.IsNotExist(err) {
		fmt.Println("👉 未检测到内核，正在从云端自动下载 (请保持网络畅通)...")
		err := downloadAndExtractKernel(dirPath, exePath)
		if err != nil {
			return fmt.Errorf("下载内核失败: %v", err)
		}
	}

	// 3. 检查 config.yaml 是否存在，不存在则生成一个最基础的配置
	configPath := filepath.Join(dirPath, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Println("👉 未检测到配置文件，正在生成默认极简配置...")
		// 一个极简的 Clash 配置，保证能成功启动 7890 端口
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

// downloadAndExtractKernel 借鉴 Stelliberty 的流式、带超时和 UA 伪装的工业级下载器
func downloadAndExtractKernel(destDir, finalExePath string) error {
	// 使用 Github 镜像源 (如果以后你有自己的服务器，可以换成你自己的直链)
	url := "https://ghproxy.net/https://github.com/MetaCubeX/mihomo/releases/download/v1.18.3/mihomo-windows-amd64-v1.18.3.zip"
	zipPath := filepath.Join(destDir, "core.zip")

	// 1. 创建健壮的 HTTP Client (设置 60 秒硬超时，防止 EOF 死锁)
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	// 2. 构建请求并伪装 User-Agent (防止被防 CC 墙拦截)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 GoclashZ/1.0")

	fmt.Println("👉 正在发起下载请求，伪装身份已开启...")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("网络请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器拒绝了请求，状态码: %d", resp.StatusCode)
	}

	// 3. 创建本地临时 ZIP 文件
	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}

	// 4. 流式写入磁盘 (极低内存占用，抗网络波动)
	fmt.Println("👉 正在流式接收数据包，请稍候...")
	_, err = io.Copy(out, resp.Body)
	out.Close() // 接收完立即关闭文件句柄，释放占用

	if err != nil {
		// 如果中途断网或 EOF，把下载了一半的坏文件删掉，避免下次启动死机
		os.Remove(zipPath)
		return fmt.Errorf("数据流接收异常中断: %v", err)
	}

	fmt.Println("✅ 压缩包下载完毕，正在提取内核...")

	// 5. 解压 ZIP 提取 exe 文件
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		os.Remove(zipPath) // 损坏的压缩包也删掉
		return fmt.Errorf("解压失败，文件可能已损坏: %v", err)
	}
	defer r.Close()

	// 遍历压缩包内的文件，寻找 .exe
	for _, f := range r.File {
		if strings.HasSuffix(f.Name, ".exe") {
			rc, err := f.Open()
			if err != nil {
				return err
			}

			// 创建最终的 clash.exe
			outFile, err := os.Create(finalExePath)
			if err != nil {
				rc.Close()
				return err
			}

			// 将解压出的流写入 clash.exe
			_, err = io.Copy(outFile, rc)
			outFile.Close()
			rc.Close()

			if err != nil {
				return err
			}
			break // 找到了 exe 并提取完毕，退出循环
		}
	}

	// 6. 扫尾工作：删除下载的 .zip 压缩包
	os.Remove(zipPath)
	fmt.Println("🎉 内核彻底部署成功，准备点火！")
	return nil
}
