//go:build windows

package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type InstallProfile struct {
	Mode      string `json:"mode"`
	AppDir    string `json:"appDir"`
	DataDir   string `json:"dataDir"`
	CreatedBy string `json:"createdBy"`
}

func readInstallProfile(appDir string) (InstallProfile, bool) {
	path := filepath.Join(appDir, "install-profile.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return InstallProfile{}, false
	}

	var p InstallProfile
	if err := json.Unmarshal(data, &p); err != nil {
		return InstallProfile{}, false
	}

	if strings.TrimSpace(p.DataDir) == "" {
		return InstallProfile{}, false
	}

	return p, true
}

var (
	appDir        string
	dataDir       string
	legacyDataDir string
)

func init() {
	initDirs()
}

func initDirs() {
	appDir = resolveAppDir()

	legacyDataDir = resolveLegacyAppDataDir()
	dataDir = resolveStableDataDir(appDir)

	_ = os.MkdirAll(dataDir, 0755)
	_ = os.MkdirAll(filepath.Join(dataDir, "profiles"), 0755)
	_ = os.MkdirAll(filepath.Join(dataDir, "Subscriptions"), 0755)
	_ = os.MkdirAll(filepath.Join(dataDir, "Settings"), 0755)
	_ = os.MkdirAll(filepath.Join(dataDir, "runtime"), 0755)
	_ = os.MkdirAll(filepath.Join(dataDir, "core", "bin"), 0755)
	_ = os.MkdirAll(filepath.Join(dataDir, "logs"), 0755)
	_ = os.MkdirAll(filepath.Join(dataDir, "updates"), 0755)
	_ = os.MkdirAll(filepath.Join(dataDir, "backups"), 0755)
}

func resolveAppDir() string {
	exePath, err := os.Executable()
	var dir string
	if err != nil {
		dir = "."
	} else {
		dir = filepath.Dir(exePath)
	}

	// 兼容 Wails Dev 模式与 Go 临时目录
	if strings.Contains(exePath, "go-build") ||
		strings.Contains(os.TempDir(), dir) ||
		strings.Contains(exePath, "wails-dev") {
		wd, err := os.Getwd()
		if err == nil {
			dir = wd
		}
	}

	// 兼容 build/bin 本地直接运行测试
	if filepath.Base(dir) == "bin" && filepath.Base(filepath.Dir(dir)) == "build" {
		dir = filepath.Dir(filepath.Dir(dir))
	}

	return dir
}

func resolveStableDataDir(appDir string) string {
	if dir := parseDataDirArg(); dir != "" {
		return filepath.Clean(dir)
	}

	if dir := strings.TrimSpace(os.Getenv("GOCLASHZ_DATA_DIR")); dir != "" {
		return filepath.Clean(dir)
	}

	if p, ok := readInstallProfile(appDir); ok {
		switch p.Mode {
		case "standard":
			return filepath.Clean(os.ExpandEnv(p.DataDir))
		case "self-contained", "portable":
			return filepath.Clean(p.DataDir)
		}
	}

	if _, err := os.Stat(filepath.Join(appDir, ".portable")); err == nil {
		return filepath.Join(appDir, "data")
	}

	local, err := os.UserCacheDir()
	if err == nil && strings.TrimSpace(local) != "" {
		return filepath.Join(local, "GoclashZ")
	}

	return filepath.Join(appDir, "data")
}

func resolveLegacyAppDataDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "GoclashZ")
}

func parseDataDirArg() string {
	args := os.Args
	for i := 0; i < len(args); i++ {
		if args[i] == "--data-dir" && i+1 < len(args) {
			return strings.TrimSpace(args[i+1])
		}

		if strings.HasPrefix(args[i], "--data-dir=") {
			return strings.TrimSpace(strings.TrimPrefix(args[i], "--data-dir="))
		}
	}
	return ""
}

// GetAppDir 返回程序所在目录 (只读)
func GetAppDir() string {
	return appDir
}

func GetSeedCoreBinDir() string {
	return filepath.Join(appDir, "seed", "core", "bin")
}

func GetSeedAssetManifestPath() string {
	return filepath.Join(appDir, "seed", "core", "asset-manifest.json")
}

func GetRuntimeDir() string {
	return filepath.Join(dataDir, "runtime")
}

func GetRuntimeConfigPath() string {
	return filepath.Join(GetRuntimeDir(), "config.yaml")
}

func GetPidFilePath() string {
	return filepath.Join(GetRuntimeDir(), "clash.pid")
}

// GetCoreBinDir 返回运行时 clash.exe 所在目录 (可写)
func GetCoreBinDir() string {
	return filepath.Join(dataDir, "core", "bin")
}

// GetLegacyDataCoreBinDir 返回旧的 clash.exe 所在目录 (只读，仅用于迁移)
// 实际返回旧版 Roaming 路径下的 core/bin
func GetLegacyDataCoreBinDir() string {
	if legacyDataDir != "" {
		return filepath.Join(legacyDataDir, "core", "bin")
	}
	return filepath.Join(dataDir, "core", "bin")
}

// GetDataDir 返回全局用户数据目录 (动态决定)
func GetDataDir() string {
	return dataDir
}

// GetLegacyDataDir 返回旧的 AppData 目录 (只读，仅用于迁移或诊断)
func GetLegacyDataDir() string {
	return legacyDataDir
}

// GetProfilesDir 返回存放 index.json 的目录
func GetProfilesDir() string {
	return filepath.Join(dataDir, "profiles")
}

// GetSubscriptionsDir 返回存放 YAML 和 Rules 文件的目录
func GetSubscriptionsDir() string {
	return filepath.Join(dataDir, "Subscriptions")
}

// GetSettingsDir 返回设置文件夹的绝对路径
func GetSettingsDir() string {
	return filepath.Join(dataDir, "Settings")
}

// SanitizeFilename 🛡️ 防御路径穿越：确保文件名不会逃逸出目标目录
func SanitizeFilename(name string) (string, error) {
	safe := filepath.Base(filepath.Clean(name))
	if safe == "." || safe == "/" || safe == "\\" {
		return "", fmt.Errorf("非法的文件名拒绝访问")
	}
	return safe, nil
}

// GetGlobalTheme 读取全局主题配置
func GetGlobalTheme() string {
	themeFile := filepath.Join(GetDataDir(), "theme_setting.txt")
	if data, err := os.ReadFile(themeFile); err == nil {
		return strings.TrimSpace(string(data))
	}
	return "dark"
}

// SaveGlobalTheme 保存全局主题配置
func SaveGlobalTheme(theme string) error {
	themeFile := filepath.Join(GetDataDir(), "theme_setting.txt")
	return os.WriteFile(themeFile, []byte(theme), 0644)
}
