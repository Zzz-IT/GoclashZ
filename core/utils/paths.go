//go:build windows

package utils

import (
	jsonv2 "encoding/json/v2"
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
	if err := jsonv2.Unmarshal(data, &p); err != nil {
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

	// 兼容 Wails Dev 模式与 Go 临时目录（判断"exe 是否位于临时目录内"，此前把参数写反了）
	lowerDir := strings.ToLower(filepath.Clean(dir))
	tempDir := strings.ToLower(filepath.Clean(os.TempDir()))
	inTempDir := tempDir != "" && tempDir != "." &&
		(lowerDir == tempDir || strings.HasPrefix(lowerDir, tempDir+string(os.PathSeparator)))
	if strings.Contains(exePath, "go-build") ||
		inTempDir ||
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

// seedCoreBinCandidates 列出内置资产（seed）可能所在的目录，按优先级排列。
//
// 兼容三种运行形态：安装版（<appDir>/seed）、仓库内构建（<appDir>/build/runtime-assets）、
// 以及从 <repo>/bin 直接运行（此时 seed 在 <repo>/build/runtime-assets，即 appDir 的父目录）。
// 候选是否"可用"由 GetSeedCoreBinDir 进一步校验（必须真的放着资产）——只建了空目录不算命中。
func seedCoreBinCandidates() []string {
	parent := filepath.Dir(appDir)
	return []string{
		filepath.Join(appDir, "seed", "core", "bin"),
		filepath.Join(appDir, "build", "runtime-assets", "core", "bin"),
		filepath.Join(parent, "seed", "core", "bin"),
		filepath.Join(parent, "build", "runtime-assets", "core", "bin"),
	}
}

// seedAssetNames 是 seed 目录里可能出现的资产文件名（用于判断"目录是不是真的可用"）。
var seedAssetNames = []string{"clash.exe", "wintun.dll", "geoip.metadb", "geosite.dat", "country.mmdb", "asn.dat"}

func isRegularFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// seedDirLooksUsable 判断 seed 目录里是否真的放着资产：
// hasCore 表示能找到 clash.exe（可直接自愈内核），hasAny 表示至少有一个已知资产。
func seedDirLooksUsable(dir string) (hasCore bool, hasAny bool) {
	hasCore = isRegularFile(filepath.Join(dir, "clash.exe"))
	for _, name := range seedAssetNames {
		if isRegularFile(filepath.Join(dir, name)) {
			hasAny = true
			break
		}
	}
	return hasCore, hasAny
}

// GetSeedCoreBinDir 返回内置资产（seed）所在目录。
//
// 收敛规则（F-A1-11）：只把"目录存在"当作弱条件——空的 seed/core/bin 目录不得遮蔽后面
// 真正有资产的候选。优先级：含 clash.exe 的候选 > 含任意资产的候选 > 存在但为空的候选。
func GetSeedCoreBinDir() string {
	candidates := seedCoreBinCandidates()

	fallback := ""
	for _, dir := range candidates {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		if fallback == "" {
			fallback = dir
		}
		if hasCore, _ := seedDirLooksUsable(dir); hasCore {
			return dir
		}
	}

	for _, dir := range candidates {
		if _, hasAny := seedDirLooksUsable(dir); hasAny {
			return dir
		}
	}

	if fallback != "" {
		return fallback
	}
	return candidates[0]
}

// GetSeedAssetManifestPath 返回 seed 清单路径。
//
// 约定只保留一套：清单放在 seed bin 目录的上一层，即 <seedRoot>/core/bin → <seedRoot>/core/asset-manifest.json
// （此前还有一条 Dir(Dir(seedDir)) 的推导，与实际布局始终对不上，已删除 —— F-A1-11）。
func GetSeedAssetManifestPath() string {
	dirs := append([]string{GetSeedCoreBinDir()}, seedCoreBinCandidates()...)
	for _, dir := range dirs {
		path := filepath.Join(filepath.Dir(dir), "asset-manifest.json")
		if isRegularFile(path) {
			return path
		}
	}
	return filepath.Join(filepath.Dir(GetSeedCoreBinDir()), "asset-manifest.json")
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

// IsPackagedInstall 判断是否为安装包模式（非开发环境）
func IsPackagedInstall() bool {
	if _, err := os.Stat(filepath.Join(appDir, "install-profile.json")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(appDir, ".installed")); err == nil {
		return true
	}
	return false
}
