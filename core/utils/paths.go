package utils

import (
	"os"
	"path/filepath"
	"strings"
)

var (
	appDir  string
	dataDir string
)

func init() {
	initDirs()
}

func initDirs() {
	// 1. 初始化程序目录 (AppDir)
	exePath, err := os.Executable()
	if err != nil {
		appDir = "."
	} else {
		appDir = filepath.Dir(exePath)
	}

	// 兼容 Wails Dev 模式与 Go 临时目录
	if strings.Contains(exePath, "go-build") ||
		strings.Contains(os.TempDir(), appDir) ||
		strings.Contains(exePath, "wails-dev") {
		wd, err := os.Getwd()
		if err == nil {
			appDir = wd
		}
	}

	// 兼容 build/bin 本地直接运行测试
	if filepath.Base(appDir) == "bin" && filepath.Base(filepath.Dir(appDir)) == "build" {
		appDir = filepath.Dir(filepath.Dir(appDir))
	}

	// ---------------------------------------------------------
	// 2. 🎯 智能嗅探：决定数据目录 (DataDir)
	// ---------------------------------------------------------
	
	// 预期在安装目录下建立一个专属的 data 文件夹（自定义模式）
	customModeDataDir := filepath.Join(appDir, "data")

	// 动态检测：当前安装目录是否允许写入？
	if isDirWritable(appDir) {
		// ✅ 允许写入（如 D:\GoclashZ）：触发【自定义模式】
		dataDir = customModeDataDir
	} else {
		// ❌ 拒绝访问（如 C:\Program Files）：降级到【系统安全模式】(AppData)
		configDir, err := os.UserConfigDir()
		if err != nil {
			configDir = appDir // 极端兜底
		}
		dataDir = filepath.Join(configDir, "GoclashZ")
	}

	// 确保基础目录存在
	os.MkdirAll(dataDir, 0755)
	os.MkdirAll(filepath.Join(dataDir, "profiles"), 0755)
	os.MkdirAll(filepath.Join(dataDir, "core", "bin"), 0755) // 提前建好内核目录
}

// isDirWritable 测试目标目录是否可写 (通过静默创建和删除测试文件)
func isDirWritable(dir string) bool {
	testFile := filepath.Join(dir, ".write_test_goclashz")
	f, err := os.OpenFile(testFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return false // 没权限或受 UAC 保护
	}
	f.Close()
	_ = os.Remove(testFile)
	return true
}

// GetAppDir 返回程序所在目录 (只读)
func GetAppDir() string {
	return appDir
}

// GetCoreBinDir 返回 clash.exe 所在目录 (只读)
// 🎯 核心修复：内核存放目录转移到 DataDir
func GetCoreBinDir() string {
	return filepath.Join(dataDir, "core", "bin")
}

// GetDataDir 返回全局用户数据目录 (动态决定)
func GetDataDir() string {
	return dataDir
}

// GetProfilesDir 返回存放订阅配置文件的目录
func GetProfilesDir() string {
	return filepath.Join(dataDir, "profiles")
}
