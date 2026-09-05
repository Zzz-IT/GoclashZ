//go:build windows

package sys

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"goclashz/core/utils"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	runRegistryKey = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName   = "GoclashZ"
	tsTaskName     = "GoclashZ Startup"
)

type StartupMode string

const (
	StartupDisabled StartupMode = "disabled"
	StartupNormal   StartupMode = "normal"
)

type StartupTaskInfo struct {
	Exists           bool        `json:"exists"`
	Enabled          bool        `json:"enabled"`
	// SchedulerEnabled 兼容旧前端和 controller 状态字段
	SchedulerEnabled bool        `json:"schedulerEnabled"`
	Mode             StartupMode `json:"mode"`
	Path             string      `json:"path"`
	Arguments        string      `json:"arguments"`
	RunLevel         int         `json:"runLevel"`
	LastError        string      `json:"lastError"`
	ExpectedPath     string      `json:"expectedPath"`
	ActualPath       string      `json:"actualPath"`
	ActualArgs       string      `json:"actualArgs"`
	ExpectedDataDir  string      `json:"expectedDataDir"`
	ActualDataDir    string      `json:"actualDataDir"`
	IsHealthy        bool        `json:"isHealthy"`
}

func parseCommandLine(cmdline string) (exePath string, args string, argList []string) {
	trimmed := strings.TrimSpace(cmdline)
	if trimmed == "" {
		return "", "", nil
	}

	argv, err := windows.UTF16PtrFromString(trimmed)
	if err != nil {
		return "", "", nil
	}

	var argc int32
	ptr, err := windows.CommandLineToArgv(argv, &argc)
	if err != nil {
		return "", "", nil
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(ptr)))

	if argc <= 0 {
		return "", "", nil
	}

	argsArr := (*[(1 << 29) - 1]*uint16)(unsafe.Pointer(ptr))[:argc:argc]
	exePath = windows.UTF16PtrToString(argsArr[0])
	argList = make([]string, 0, argc-1)
	for i := int32(1); i < argc; i++ {
		argList = append(argList, windows.UTF16PtrToString(argsArr[i]))
	}

	if strings.HasPrefix(trimmed, "\"") {
		closeIdx := strings.Index(trimmed[1:], "\"")
		if closeIdx != -1 {
			args = strings.TrimSpace(trimmed[closeIdx+2:])
		}
	} else {
		fields := strings.Fields(trimmed)
		if len(fields) > 1 {
			args = strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
		}
	}

	return exePath, args, argList
}

func extractArgValue(fields []string, key string) string {
	prefix := key + "="
	for i := 0; i < len(fields); i++ {
		if fields[i] == key && i+1 < len(fields) {
			return fields[i+1]
		}
		if strings.HasPrefix(fields[i], prefix) {
			return strings.TrimPrefix(fields[i], prefix)
		}
	}
	return ""
}

func hasLegacyTaskSchedulerTask() bool {
	cmd := exec.Command("schtasks", "/Query", "/TN", tsTaskName)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run() == nil
}

func deleteLegacyTaskSchedulerTask() {
	cmd := exec.Command("schtasks", "/Delete", "/TN", tsTaskName, "/F")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Run()
}

func checkAndMigrateLegacyTask(info StartupTaskInfo, expected string) (StartupTaskInfo, error) {
	if hasLegacyTaskSchedulerTask() {
		// 发现存在旧版任务计划程序自启，无感迁移至注册表 Run 项
		if err := CreateStartupTask(expected); err == nil {
			info.Exists = true
			info.Enabled = true
			info.SchedulerEnabled = true
			info.IsHealthy = true
			info.Mode = StartupNormal
			info.Path = expected
			info.ActualPath = expected
			info.ActualDataDir = info.ExpectedDataDir
			info.Arguments = fmt.Sprintf(`--startup --silent --data-dir "%s"`, info.ExpectedDataDir)
			info.ActualArgs = info.Arguments
			return info, nil
		}
	}

	info.Exists = false
	info.Enabled = false
	info.SchedulerEnabled = false
	info.IsHealthy = false
	info.LastError = "startup task not found"
	return info, nil
}

func CheckStartupTask() (StartupTaskInfo, error) {
	exe, _ := os.Executable()
	expected, _ := filepath.Abs(exe)
	expectedDataDir := filepath.Clean(utils.GetDataDir())

	info := StartupTaskInfo{
		Mode:            StartupDisabled,
		ExpectedPath:    expected,
		ExpectedDataDir: expectedDataDir,
		RunLevel:        0,
	}

	key, err := registry.OpenKey(registry.CURRENT_USER, runRegistryKey, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return checkAndMigrateLegacyTask(info, expected)
		}
		info.LastError = fmt.Sprintf("打开注册表失败: %v", err)
		return info, err
	}
	defer key.Close()

	val, _, err := key.GetStringValue(runValueName)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return checkAndMigrateLegacyTask(info, expected)
		}
		info.LastError = fmt.Sprintf("读取注册表失败: %v", err)
		return info, err
	}

	info.Exists = true
	info.Enabled = true
	info.SchedulerEnabled = true
	info.Mode = StartupNormal

	actualExe, rawArgs, argList := parseCommandLine(val)
	actualAbs, _ := filepath.Abs(actualExe)

	info.Path = actualExe
	info.Arguments = rawArgs
	info.ActualPath = actualExe
	info.ActualArgs = rawArgs

	actualDataDir := ""
	if rawDir := strings.TrimSpace(extractArgValue(argList, "--data-dir")); rawDir != "" {
		actualDataDir = filepath.Clean(rawDir)
	}
	info.ActualDataDir = actualDataDir

	hasStartup := false
	hasSilent := false
	for _, arg := range argList {
		if arg == "--startup" {
			hasStartup = true
		}
		if arg == "--silent" {
			hasSilent = true
		}
	}

	pathMismatch := !strings.EqualFold(filepath.Clean(actualAbs), filepath.Clean(expected))
	dataDirMismatch := actualDataDir == "" || !strings.EqualFold(actualDataDir, expectedDataDir)
	argsMismatch := !hasStartup || !hasSilent || dataDirMismatch

	if pathMismatch || argsMismatch {
		info.Enabled = false
		info.LastError = "path mismatch or incomplete arguments"
		info.IsHealthy = false
	} else {
		info.IsHealthy = true
		info.LastError = ""
	}

	return info, nil
}

func CreateStartupTask(exePath string) error {
	absPath, err := filepath.Abs(exePath)
	if err != nil {
		return fmt.Errorf("无法获取绝对路径: %w", err)
	}
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("可执行文件不存在: %s", absPath)
	}

	key, _, err := registry.CreateKey(registry.CURRENT_USER, runRegistryKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册表自启动项失败: %w", err)
	}
	defer key.Close()

	cmd := fmt.Sprintf(`"%s" --startup --silent --data-dir "%s"`, absPath, utils.GetDataDir())
	if err := key.SetStringValue(runValueName, cmd); err != nil {
		return fmt.Errorf("写入注册表自启动项失败: %w", err)
	}

	// 尽力清理旧版任务计划程序残留（若无权限则忽略，不阻断主流程）
	go deleteLegacyTaskSchedulerTask()

	return nil
}

// DeleteStartupTask removes the GoclashZ startup entry from registry Run key.
// Returns nil if the value does not exist.
func DeleteStartupTask() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runRegistryKey, registry.SET_VALUE)
	if err != nil {
		if !errors.Is(err, registry.ErrNotExist) {
			return fmt.Errorf("打开注册表自启动项失败: %w", err)
		}
	} else {
		defer key.Close()
		if err := key.DeleteValue(runValueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return fmt.Errorf("删除注册表自启动项失败: %w", err)
		}
	}

	// 尽力清理旧版任务计划程序残留
	go deleteLegacyTaskSchedulerTask()

	return nil
}
