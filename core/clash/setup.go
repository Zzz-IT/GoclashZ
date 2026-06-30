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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"goclashz/core/downloader"
	"goclashz/core/runtimeassets"
	"goclashz/core/sys"
	"goclashz/core/utils"
)

// PrepareEnv 检查内核并生成基础配置
func PrepareEnv(ctx context.Context) error {
	status, err := runtimeassets.EnsureReady(ctx, runtimeassets.RepairInvalid)
	if err != nil {
		core := status.Assets[runtimeassets.AssetCore]
		return fmt.Errorf("内核不可用: %s (%s)", core.Error, core.Path)
	}

	_ = os.MkdirAll(utils.GetSubscriptionsDir(), 0755)

	defaultCfg := utils.GetRuntimeConfigPath()
	if _, err := os.Stat(defaultCfg); os.IsNotExist(err) {
		_ = utils.WriteFileAtomic(defaultCfg, []byte("mode: rule\n"), 0644)
	}

	return nil
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

func PrepareCoreUpdate(ctx context.Context, assetURL string, strategy func() downloader.DownloadStrategy, onProgress func(int64, int64, int64, int64)) (map[string]string, error) {
	coreBinaryMu.Lock()
	defer coreBinaryMu.Unlock()

	binDir := utils.GetCoreBinDir()
	exePath := filepath.Join(binDir, "clash.exe")
	zipPath := filepath.Join(binDir, "clash.update.zip")
	stagedExe := exePath + ".new"

	_ = os.Remove(zipPath)
	_ = os.Remove(stagedExe)
	defer os.Remove(zipPath)

	if strings.TrimSpace(assetURL) == "" {
		return nil, fmt.Errorf("内核下载地址为空")
	}

	if err := downloader.DownloadLargeAssetAtomic(ctx, downloader.Options{
		URLs:                []string{assetURL},
		DestPath:            zipPath,
		Strategy:            strategy,
		MaxBytes:            200 << 20,
		UserAgent:           "GoclashZ-CoreUpdater",
		AttemptsPerEndpoint: 3,
		Validator: func(tmpPath string) error {
			return validateKernelZip(tmpPath)
		},
		OnProgress: onProgress,
	}); err != nil {
		return nil, err
	}

	if err := extractKernelToFile(zipPath, stagedExe); err != nil {
		_ = os.Remove(stagedExe)
		return nil, err
	}

	if err := utils.ValidateWindowsPE(stagedExe, 5*1024*1024, 300*1024*1024); err != nil {
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

	// 如果 core\bin 不可写，通过 helper 服务替换
	if !isDirWritable(filepath.Dir(exePath)) {
		client := sys.NewHelperClient()
		if err := client.ReplaceCoreFile(sys.ReplaceCoreFileParams{
			Source: stagedExe,
			Target: exePath,
		}); err != nil {
			return "", fmt.Errorf("通过 Helper 替换内核失败: %w", err)
		}
	} else {
		if err := WaitFileReleased(exePath, 5*time.Second); err != nil {
			return "", err
		}

		if err := ReplaceFileWithBackup(stagedExe, exePath); err != nil {
			return "", err
		}
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

func CheckLatestCore(ctx context.Context, strategy func() downloader.DownloadStrategy) (version, assetURL, releaseURL string, err error) {
	clients := downloader.BuildOrderedClients(strategy, 60*time.Second)

	var release CoreReleaseInfo
	var lastErr error

	for _, client := range clients {
		req, reqErr := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"https://api.github.com/repos/MetaCubeX/mihomo/releases/latest",
			nil,
		)
		if reqErr != nil {
			lastErr = reqErr
			continue
		}

		req.Header.Set("User-Agent", "GoclashZ-CoreUpdateChecker")
		req.Header.Set("Accept", "application/vnd.github+json")

		resp, reqErr := client.Do(req)
		if reqErr != nil {
			lastErr = reqErr
			continue
		}

		func() {
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				lastErr = fmt.Errorf("GitHub API 返回 HTTP %d", resp.StatusCode)
				return
			}
			lastErr = json.NewDecoder(resp.Body).Decode(&release)
		}()

		if lastErr == nil && release.TagName != "" {
			break
		}
	}

	if lastErr != nil || release.TagName == "" {
		if lastErr == nil {
			lastErr = fmt.Errorf("未获取到有效的最新版本信息")
		}
		return "", "", "", lastErr
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

	original := v
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")

	// 去掉 prerelease/build metadata。
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		v = v[:idx]
	}

	if v == "" {
		return out, fmt.Errorf("invalid version: %s", original)
	}

	parts := strings.Split(v, ".")
	if len(parts) == 0 || parts[0] == "" {
		return out, fmt.Errorf("invalid version: %s", original)
	}

	// 如果部分缺失，则补 0
	for i := 0; i < 3; i++ {
		if i < len(parts) {
			if parts[i] == "" {
				return out, fmt.Errorf("invalid version: %s", original)
			}
			n, err := strconv.Atoi(parts[i])
			if err != nil {
				return out, fmt.Errorf("invalid version: %s", original)
			}
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

func UpdateGeoDB(ctx context.Context, key string, url string, strategy func() downloader.DownloadStrategy, onProgress func(bytesDone, totalBytes, speedBps int64, etaSec int64)) error {
	destPath, err := GeoDBPath(key)
	if err != nil {
		return err
	}

	return downloader.DownloadLargeAssetAtomic(ctx, downloader.Options{
		URLs:                []string{url},
		DestPath:            destPath,
		Strategy:            strategy,
		MaxBytes:            geoDBMaxBytes(key),
		UserAgent:           "GoclashZ-GeoUpdater",
		AttemptsPerEndpoint: 3,
		Validator: func(tmpPath string) error {
			return ValidateGeoDBFile(key, tmpPath, destPath)
		},
		OnProgress: onProgress,
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
