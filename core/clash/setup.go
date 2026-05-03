//go:build windows

package clash

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
		// 初始化时如果不通代理，PrepareCoreUpdate 内部逻辑会处理
		prepared, err := PrepareCoreUpdate(ctx, "https://github.com/MetaCubeX/mihomo/releases/download/v1.18.1/mihomo-windows-amd64-v1.18.1.zip", "")
		if err == nil {
			_, _ = CommitCoreUpdate(ctx, prepared)
		} else {
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

func ClearLocalCoreVersionCache() {
	localCoreVersionCache.mu.Lock()
	defer localCoreVersionCache.mu.Unlock()

	localCoreVersionCache.path = ""
	localCoreVersionCache.size = 0
	localCoreVersionCache.modTime = 0
	localCoreVersionCache.version = ""
}

func getLocalCoreVersionLocked(ctx context.Context) string {
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

// GetLocalCoreVersion 获取本地内核版本号
func GetLocalCoreVersion(ctx context.Context) string {
	coreBinaryMu.Lock()
	defer coreBinaryMu.Unlock()

	return getLocalCoreVersionLocked(ctx)
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


func validateKernelZip(path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("invalid zip archive: %v", err)
	}
	defer r.Close()
	return nil
}

func extractKernelToFile(zipPath, targetExe string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

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

	_ = os.Remove(targetExe)
	f, err := os.OpenFile(targetExe, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, rc); err != nil {
		f.Close()
		_ = os.Remove(targetExe)
		return err
	}

	return f.Close()
}

var coreBinaryMu sync.Mutex

func PrepareCoreUpdate(ctx context.Context, assetURL string, proxyURL string) (map[string]string, error) {
	coreBinaryMu.Lock()
	defer coreBinaryMu.Unlock()

	binDir := utils.GetCoreBinDir()
	exePath := filepath.Join(binDir, "clash.exe")
	zipPath := filepath.Join(binDir, "clash.update.zip")
	stagedExe := exePath + ".new"

	_ = os.Remove(zipPath)
	_ = os.Remove(stagedExe)

	if strings.TrimSpace(assetURL) == "" {
		return nil, fmt.Errorf("内核下载地址为空")
	}

	if err := downloader.DownloadLargeAssetAtomic(ctx, downloader.Options{
		URLs:                []string{assetURL},
		DestPath:            zipPath,
		ProxyURL:            proxyURL,
		PreferProxy:         proxyURL != "",
		MaxBytes:            200 << 20,
		UserAgent:           "GoclashZ-CoreUpdater",
		AttemptsPerEndpoint: 3,
		Validator: func(tmpPath string) error {
			return validateKernelZip(tmpPath)
		},
	}); err != nil {
		return nil, err
	}

	if err := extractKernelToFile(zipPath, stagedExe); err != nil {
		_ = os.Remove(stagedExe)
		return nil, err
	}

	if err := ValidateWindowsPE(stagedExe, 5*1024*1024); err != nil {
		_ = os.Remove(stagedExe)
		return nil, err
	}

	return map[string]string{
		"stagedExe": stagedExe,
		"exePath":   exePath,
	}, nil
}

func CommitCoreUpdate(ctx context.Context, prepared map[string]string) (string, error) {
	coreBinaryMu.Lock()
	defer coreBinaryMu.Unlock()

	stagedExe := prepared["stagedExe"]
	exePath := prepared["exePath"]

	if stagedExe == "" || exePath == "" {
		return "", fmt.Errorf("内核更新 staging 信息缺失")
	}

	if err := WaitFileReleased(exePath, 5*time.Second); err != nil {
		return "", err
	}

	if err := ReplaceFileWithBackup(stagedExe, exePath); err != nil {
		return "", err
	}

	ClearLocalCoreVersionCache()
	return getLocalCoreVersionLocked(ctx), nil
}

type CoreReleaseInfo struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func CheckLatestCore(ctx context.Context, proxyURL string) (version, assetURL, releaseURL string, err error) {
	client := downloader.NewProxyClient(proxyURL)

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"https://api.github.com/repos/MetaCubeX/mihomo/releases/latest",
		nil,
	)
	if err != nil {
		return "", "", "", err
	}

	req.Header.Set("User-Agent", "GoclashZ-CoreUpdateChecker")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", "", fmt.Errorf("GitHub API 返回 HTTP %d", resp.StatusCode)
	}

	var release CoreReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", "", "", err
	}

	assetURL = selectMihomoWindowsAmd64Asset(release.Assets)
	if assetURL == "" {
		return "", "", "", fmt.Errorf("未找到 mihomo windows amd64 release asset")
	}

	return release.TagName, assetURL, release.HTMLURL, nil
}

func selectMihomoWindowsAmd64Asset(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}) string {
	for _, asset := range assets {
		name := strings.ToLower(asset.Name)

		if !strings.HasSuffix(name, ".zip") {
			continue
		}
		if !strings.Contains(name, "mihomo") {
			continue
		}
		if !strings.Contains(name, "windows") {
			continue
		}
		if !(strings.Contains(name, "amd64") || strings.Contains(name, "x64")) {
			continue
		}

		return asset.BrowserDownloadURL
	}

	return ""
}

func CompareCoreVersion(remote, local string) (int, error) {
	r, err := parseVersionParts(remote)
	if err != nil {
		return 0, err
	}

	l, err := parseVersionParts(local)
	if err != nil {
		// 本地版本未知、未安装或无法解析时，允许更新。
		return 1, nil
	}

	for i := 0; i < 3; i++ {
		if r[i] > l[i] {
			return 1, nil
		}
		if r[i] < l[i] {
			return -1, nil
		}
	}

	return 0, nil
}

func parseVersionParts(v string) ([3]int, error) {
	var out [3]int

	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")

	// 去掉 prerelease/build metadata。
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		v = v[:idx]
	}

	parts := strings.Split(v, ".")
	// 如果部分缺失，则补 0
	for i := 0; i < 3; i++ {
		if i < len(parts) {
			n, _ := strconv.Atoi(parts[i])
			out[i] = n
		} else {
			out[i] = 0
		}
	}

	return out, nil
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

func UpdateGeoDB(ctx context.Context, key string, url string, proxyURL string) error {
	destPath, err := GeoDBPath(key)
	if err != nil {
		return err
	}

	return downloader.DownloadLargeAssetAtomic(ctx, downloader.Options{
		URLs:                []string{url},
		DestPath:            destPath,
		ProxyURL:            proxyURL,
		PreferProxy:         proxyURL != "",
		MaxBytes:            geoDBMaxBytes(key),
		UserAgent:           "GoclashZ-GeoUpdater",
		AttemptsPerEndpoint: 3,
		Validator: func(tmpPath string) error {
			return ValidateGeoDBFile(key, tmpPath, destPath)
		},
	})
}

func geoDBMaxBytes(_ string) int64 {
	// 默认限制 200MB，防止异常重定向下载了巨大的错误页或二进制
	return 200 << 20
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
