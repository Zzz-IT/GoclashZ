//go:build windows

package clash

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"regexp"
	"sync"

	"goclashz/core/downloader"
	"goclashz/core/utils"
)

// PrepareEnv 检查内核并生成基础配置
func PrepareEnv(ctx context.Context) error {
	MigrateCoreAssetsToBin()
	binDir := utils.GetCoreBinDir() // 取向安全的 DataDir
	
	if _, err := os.Stat(filepath.Join(binDir, "clash.exe")); os.IsNotExist(err) {
		// 优先触发一次下载（或者由前端引导）
		if _, err := UpdateCore(ctx); err != nil {
			return fmt.Errorf("内核文件缺失且自动下载失败: %v", err)
		}
	}

	// 提前创建配置文件夹
	os.MkdirAll(utils.GetSubscriptionsDir(), 0755)
	
	// 初始化默认配置 (如果不存在)
	defaultCfg := filepath.Join(utils.GetDataDir(), "config.yaml")
	if _, err := os.Stat(defaultCfg); os.IsNotExist(err) {
		_ = os.WriteFile(defaultCfg, []byte("mode: rule\n"), 0644)
	}

	return nil
}

// MigrateCoreAssetsToBin 将旧版遗留在 data 根目录的资产迁移到 core/bin 下
func MigrateCoreAssetsToBin() {
	dataDir := utils.GetDataDir()
	binDir := utils.GetCoreBinDir()
	os.MkdirAll(binDir, 0755)

	assets := []string{
		"clash.exe", "wintun.dll", 
		"geoip.metadb", "geosite.dat", "country.mmdb", "asn.dat",
	}

	for _, name := range assets {
		oldPath := filepath.Join(dataDir, name)
		newPath := filepath.Join(binDir, name)

		if _, err := os.Stat(oldPath); err == nil {
			// 旧路径存在文件
			if _, err := os.Stat(newPath); os.IsNotExist(err) {
				// 新路径不存在，执行移动
				_ = os.Rename(oldPath, newPath)
			} else {
				// 新旧都存在，则删除旧的（保持清理）
				_ = os.Remove(oldPath)
			}
		}
	}
}

var localCoreVersionCache struct {
	mu      sync.Mutex
	path    string
	size    int64
	modTime int64
	version string
}

var coreVersionRe = regexp.MustCompile(`v?\d+\.\d+\.\d+(?:[-+][^\s]+)?`)

// GetLocalCoreVersion 获取本地内核版本号
func GetLocalCoreVersion(ctx context.Context) string {
	path := filepath.Join(utils.GetCoreBinDir(), "clash.exe")

	stat, err := os.Stat(path)
	if err != nil || stat.IsDir() {
		return "未安装"
	}

	localCoreVersionCache.mu.Lock()
	if localCoreVersionCache.path == path &&
		localCoreVersionCache.size == stat.Size() &&
		localCoreVersionCache.modTime == stat.ModTime().UnixMilli() &&
		localCoreVersionCache.version != "" {
		version := localCoreVersionCache.version
		localCoreVersionCache.mu.Unlock()
		return version
	}
	localCoreVersionCache.mu.Unlock()

	version := readLocalCoreVersionByCommand(ctx, path)

	localCoreVersionCache.mu.Lock()
	localCoreVersionCache.path = path
	localCoreVersionCache.size = stat.Size()
	localCoreVersionCache.modTime = stat.ModTime().UnixMilli()
	localCoreVersionCache.version = version
	localCoreVersionCache.mu.Unlock()

	return version
}

func readLocalCoreVersionByCommand(ctx context.Context, path string) string {
	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, path, "-v")
	cmd.Dir = utils.GetCoreBinDir()
	utils.HideCommandWindow(cmd, 0)

	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return "已安装，版本未知"
	}

	s := strings.TrimSpace(string(out))
	if s == "" {
		return "已安装，版本未知"
	}

	if m := coreVersionRe.FindString(s); m != "" {
		if strings.HasPrefix(m, "v") {
			return m
		}
		return "v" + m
	}

	return s
}

func downloadAndExtractKernel(ctx context.Context, binDir, exePath string) error {
	// 下载内核 Zip
	zipPath := filepath.Join(binDir, "clash.zip")
	defer os.Remove(zipPath)

	url := "https://github.com/MetaCubeX/mihomo/releases/download/v1.18.1/mihomo-windows-amd64-v1.18.1.zip"
	
	err := downloader.DownloadAtomic(ctx, downloader.Options{
		URLs:           []string{url},
		DestPath:       zipPath,
		RequireGitHubSHA: false, // 降低对 SHA 校验的强制依赖，走文件头校验
		Validator: func(tmpPath string) error {
			return validateKernelZip(tmpPath)
		},
	})
	if err != nil {
		return err
	}

	// 解压
	return extractKernel(zipPath, exePath)
}

func validateKernelZip(path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("invalid zip archive: %v", err)
	}
	defer r.Close()
	return nil
}

func extractKernel(zipPath, exePath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	// 查找第一个 .exe 文件
	var targetFile *zip.File
	for _, f := range r.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".exe") {
			targetFile = f
			break
		}
	}

	if targetFile == nil {
		return fmt.Errorf("zip 中未找到 .exe 可执行文件")
	}

	rc, err := targetFile.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	// 写入到临时文件再重命名，确保原子性
	tmpExe := exePath + ".tmp"
	f, err := os.OpenFile(tmpExe, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	
	if _, err := io.Copy(f, rc); err != nil {
		f.Close()
		return err
	}
	f.Close()

	return os.Rename(tmpExe, exePath)
}

func UpdateCore(ctx context.Context) (string, error) {
	binDir := utils.GetCoreBinDir()
	exePath := filepath.Join(binDir, "clash.exe")
	if err := downloadAndExtractKernel(ctx, binDir, exePath); err != nil {
		return "", err
	}
	return GetLocalCoreVersion(ctx), nil
}

func GeoDBFileName(key string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "geoip":
		return "geoip.metadb", nil
	case "geosite":
		return "geosite.dat", nil
	case "mmdb":
		return "country.mmdb", nil
	case "asn":
		return "asn.dat", nil
	default:
		return "", fmt.Errorf("unknown geo database key: %s", key)
	}
}

func GeoDBPath(key string) (string, error) {
	name, err := GeoDBFileName(key)
	if err != nil {
		return "", err
	}
	return filepath.Join(utils.GetCoreBinDir(), name), nil
}

func UpdateGeoDB(ctx context.Context, key string, url string) error {
	destPath, err := GeoDBPath(key)
	if err != nil {
		return err
	}

	return downloader.DownloadAtomic(ctx, downloader.Options{
		URLs:     []string{url},
		DestPath: destPath,
		Resume:   true,
		Validator: func(tmpPath string) error {
			return ValidateGeoDBFile(key, tmpPath, destPath)
		},
	})
}

func ValidateGeoDBFile(key, tmpPath, destPath string) error {
	info, err := os.Stat(tmpPath)
	if err != nil {
		return err
	}

	// 太小基本就是 HTML 错误页、空文件或下载失败
	if info.Size() < 1024 {
		return fmt.Errorf("%s 文件体积异常: %d bytes", key, info.Size())
	}

	switch strings.ToLower(strings.TrimSpace(key)) {
	case "mmdb":
		if filepath.Ext(destPath) != ".mmdb" {
			return fmt.Errorf("mmdb 目标路径扩展名异常: %s", destPath)
		}
	case "geoip", "geosite", "asn":
		// dat/metadb 不强行解析格式，先做体积保护
	default:
		return fmt.Errorf("unknown geo database key: %s", key)
	}

	return nil
}
