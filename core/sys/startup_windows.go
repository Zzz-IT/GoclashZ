//go:build windows

package sys

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/go-ole/go-ole"
	"goclashz/core/utils"
	"golang.org/x/sys/windows"
)

const (
	tsActionExec     = 0 // TASK_ACTION_EXEC
	tsTriggerLogon   = 9 // TASK_TRIGGER_LOGON
	tsCreateOrUpdate = 6 // TASK_CREATE_OR_UPDATE
)

var clsidTaskScheduler = ole.NewGUID("{0F87369F-A4E5-4CFC-BD3E-73E6154572DD}")

const tsTaskName = "GoclashZ Startup"

type StartupMode string

const (
	StartupDisabled StartupMode = "disabled"
	StartupNormal   StartupMode = "normal"
)

type StartupTaskInfo struct {
	Exists          bool        `json:"exists"`
	Enabled         bool        `json:"enabled"`
	Mode            StartupMode `json:"mode"`
	Path            string      `json:"path"`
	Arguments       string      `json:"arguments"`
	RunLevel        int         `json:"runLevel"`
	LastError       string      `json:"lastError"`
	ExpectedPath    string      `json:"expectedPath"`
	ActualPath      string      `json:"actualPath"`
	ActualArgs      string      `json:"actualArgs"`
	ExpectedDataDir string      `json:"expectedDataDir"`
	ActualDataDir   string      `json:"actualDataDir"`
	IsHealthy       bool        `json:"isHealthy"`
}

// initCOM initializes COM and returns a cleanup function.
func initCOM() (func(), error) {
	err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	if err != nil {
		if oleErr, ok := err.(*ole.OleError); ok {
			code := oleErr.Code()
			// 1 (S_FALSE) 表示已作为相同模式初始化
			// 0x80010106 (RPC_E_CHANGED_MODE) 表示已作为不同模式初始化 (Wails 主线程)
			if code == 1 || code == 0x80010106 {
				return func() {}, nil
			}
		}
		errStr := err.Error()
		if strings.Contains(errStr, "函数不正确") || strings.Contains(errStr, "Incorrect function") {
			return func() {}, nil
		}
		return nil, fmt.Errorf("COM 初始化失败: %w", err)
	}
	return ole.CoUninitialize, nil
}

// newTaskScheduler creates a Task Scheduler IDispatch connected to the local service.
func newTaskScheduler() (*ole.IDispatch, error) {
	unk, err := ole.CreateInstance(clsidTaskScheduler, nil)
	if err != nil {
		return nil, fmt.Errorf("创建 TaskScheduler 实例失败: %w", err)
	}

	disp, err := unk.QueryInterface(ole.IID_IDispatch)
	unk.Release()
	if err != nil {
		return nil, fmt.Errorf("获取 IDispatch 接口失败: %w", err)
	}

	if _, err := disp.CallMethod("Connect"); err != nil {
		disp.Release()
		return nil, fmt.Errorf("连接 Task Scheduler 服务失败: %w", err)
	}

	return disp, nil
}

func variantInt(v any) int {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case int:
		return x
	case int32:
		return int(x)
	case int16:
		return int(x)
	case int64:
		return int(x)
	case uint32:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}

func variantString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func splitCommandLine(args string) []string {
	if strings.TrimSpace(args) == "" {
		return nil
	}

	cmdline := `GoclashZ.exe ` + args
	argv, err := windows.UTF16PtrFromString(cmdline)
	if err != nil {
		return nil
	}

	var argc int32
	ptr, err := windows.CommandLineToArgv(argv, &argc)
	if err != nil {
		return nil
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(ptr)))

	argsArr := (*[(1 << 29) - 1]*uint16)(unsafe.Pointer(ptr))[:argc:argc]
	out := make([]string, 0, argc-1)
	for i := int32(1); i < argc; i++ {
		out = append(out, windows.UTF16PtrToString(argsArr[i]))
	}
	return out
}

func extractArgValue(args string, key string) string {
	fields := splitCommandLine(args)
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

func CheckStartupTask() (StartupTaskInfo, error) {
	exe, _ := os.Executable()
	expected, _ := filepath.Abs(exe)
	info := StartupTaskInfo{Mode: StartupDisabled, ExpectedPath: expected}

	cleanup, err := initCOM()
	if err != nil {
		info.LastError = "COM init failed"
		return info, err
	}
	defer cleanup()

	sched, err := newTaskScheduler()
	if err != nil {
		info.LastError = "TaskScheduler connect failed"
		return info, err
	}
	defer sched.Release()

	rootV, err := sched.CallMethod("GetFolder", `\`)
	if err != nil {
		info.LastError = "GetFolder failed"
		return info, err
	}
	root := rootV.ToIDispatch()
	defer root.Release()

	taskV, err := root.CallMethod("GetTask", tsTaskName)
	if err != nil {
		// Task doesn't exist
		info.Exists = false
		info.Enabled = false
		info.IsHealthy = false
		info.LastError = "startup task not found"
		return info, nil
	}
	info.Exists = true
	task := taskV.ToIDispatch()
	defer task.Release()

	// Check if enabled
	enabledV, err := task.GetProperty("Enabled")
	if err == nil {
		if b, ok := enabledV.Value().(bool); ok {
			info.Enabled = b
		}
	}

	// Check definition
	defV, err := task.GetProperty("Definition")
	if err == nil {
		def := defV.ToIDispatch()
		defer def.Release()

		// Get Principal for RunLevel
		prinV, err := def.GetProperty("Principal")
		if err == nil {
			prin := prinV.ToIDispatch()
			runLevelV, err := prin.GetProperty("RunLevel")
			if err == nil {
				info.RunLevel = variantInt(runLevelV.Value())
			}
			prin.Release()
		}

		// Get Actions
		actionsV, err := def.GetProperty("Actions")
		if err == nil {
			actions := actionsV.ToIDispatch()
			actionCountV, err := actions.GetProperty("Count")
			if err == nil && variantInt(actionCountV.Value()) > 0 {
				actionV, err := actions.GetProperty("Item", 1) // 1-indexed in COM collections
				if err == nil {
					action := actionV.ToIDispatch()
					pathV, err := action.GetProperty("Path")
					if err == nil {
						info.Path = variantString(pathV.Value())
					}
					argsV, err := action.GetProperty("Arguments")
					if err == nil {
						info.Arguments = variantString(argsV.Value())
					}
					action.Release()
				}
			}
			actions.Release()
		}

		info.Mode = StartupNormal

		// Validation
		actualPath := strings.Trim(info.Path, "\"")
		actualPath = strings.TrimSpace(actualPath)
		actual, _ := filepath.Abs(actualPath)

		info.ActualPath = actualPath
		info.ActualArgs = info.Arguments

		expectedDataDir := filepath.Clean(utils.GetDataDir())
		rawDataDir := strings.TrimSpace(extractArgValue(info.Arguments, "--data-dir"))
		actualDataDir := ""
		if rawDataDir != "" {
			actualDataDir = filepath.Clean(rawDataDir)
		}

		hasStartup := strings.Contains(info.Arguments, "--startup")
		hasSilent := strings.Contains(info.Arguments, "--silent")
		hasDataDir := actualDataDir != ""
		dataDirMismatch := !hasDataDir || !strings.EqualFold(actualDataDir, expectedDataDir)

		info.ExpectedDataDir = expectedDataDir
		info.ActualDataDir = actualDataDir

		pathMismatch := !strings.EqualFold(filepath.Clean(actual), filepath.Clean(expected))
		argsMismatch := !hasStartup || !hasSilent || dataDirMismatch

		// 旧的 elevated 任务 (RunLevel=1) 视为不健康，需要修复为普通任务
		if info.RunLevel == 1 {
			info.LastError = "旧的管理员自启任务已不再支持，请重新设置开机自启"
			info.Enabled = false
		} else if pathMismatch || argsMismatch {
			info.Enabled = false
			info.LastError = "path mismatch or incomplete arguments"
		}

		info.Mode = StartupNormal
	}

	info.IsHealthy = info.Exists && info.Enabled && info.LastError == ""

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
	workDir := filepath.Dir(absPath)

	cleanup, err := initCOM()
	if err != nil {
		return err
	}
	defer cleanup()

	sched, err := newTaskScheduler()
	if err != nil {
		return err
	}
	defer sched.Release()

	defV, err := sched.CallMethod("NewTask", 0)
	if err != nil {
		return fmt.Errorf("创建任务定义失败: %w", err)
	}
	def := defV.ToIDispatch()
	defer def.Release()

	settingsV, err := def.GetProperty("Settings")
	if err != nil {
		return fmt.Errorf("获取 Settings 失败: %w", err)
	}
	settings := settingsV.ToIDispatch()
	settings.PutProperty("DisallowStartIfOnBatteries", false)
	settings.PutProperty("StopIfGoingOnBatteries", false)
	settings.PutProperty("AllowStartIfOnBatteries", true)
	settings.PutProperty("ExecutionTimeLimit", "PT0S")
	settings.Release()

	actionsV, err := def.GetProperty("Actions")
	if err != nil {
		return fmt.Errorf("获取 Actions 失败: %w", err)
	}
	actions := actionsV.ToIDispatch()
	actionV, err := actions.CallMethod("Create", tsActionExec)
	actions.Release()
	if err != nil {
		return fmt.Errorf("创建 Action 失败: %w", err)
	}
	action := actionV.ToIDispatch()
	action.PutProperty("Path", absPath)
	args := fmt.Sprintf("--startup --silent --data-dir \"%s\"", utils.GetDataDir())
	action.PutProperty("Arguments", args)
	action.PutProperty("WorkingDirectory", workDir)
	action.Release()

	triggersV, err := def.GetProperty("Triggers")
	if err != nil {
		return fmt.Errorf("获取 Triggers 失败: %w", err)
	}
	triggers := triggersV.ToIDispatch()
	triggerV, err := triggers.CallMethod("Create", tsTriggerLogon)
	triggers.Release()
	if err != nil {
		return fmt.Errorf("创建 Trigger 失败: %w", err)
	}
	trigger := triggerV.ToIDispatch()
	trigger.PutProperty("Enabled", true)
	// 统一延迟 15 秒以避开系统启动高峰，防止 explorer.exe 未加载完成导致托盘图标空白
	trigger.PutProperty("Delay", "PT15S")
	trigger.Release()

	principalV, err := def.GetProperty("Principal")
	if err != nil {
		return fmt.Errorf("获取 Principal 失败: %w", err)
	}
	principal := principalV.ToIDispatch()
	principal.PutProperty("LogonType", 3) // TASK_LOGON_TOKEN
	principal.PutProperty("RunLevel", 0)  // TASK_RUNLEVEL_LUA (normal user)
	principal.Release()

	def.PutProperty("DisplayName", tsTaskName)
	def.PutProperty("Description", "开机自启 GoclashZ 代理客户端")

	rootV, err := sched.CallMethod("GetFolder", `\`)
	if err != nil {
		return fmt.Errorf("获取根文件夹失败: %w", err)
	}
	root := rootV.ToIDispatch()
	defer root.Release()

	logonType := int32(3) // TASK_LOGON_TOKEN

	_, err = root.CallMethod("RegisterTaskDefinition",
		tsTaskName,
		def,
		tsCreateOrUpdate,
		"",  // userId: current user
		nil, // password
		logonType,
	)
	if err != nil {
		return fmt.Errorf("注册计划任务失败: %w", err)
	}

	return nil
}

// DeleteStartupTask removes the GoclashZ startup task.
// Returns nil if the task does not exist.
func DeleteStartupTask() error {
	cleanup, err := initCOM()
	if err != nil {
		return err
	}
	defer cleanup()

	sched, err := newTaskScheduler()
	if err != nil {
		return err
	}
	defer sched.Release()

	rootV, err := sched.CallMethod("GetFolder", `\`)
	if err != nil {
		return fmt.Errorf("获取根文件夹失败: %w", err)
	}
	root := rootV.ToIDispatch()
	defer root.Release()

	// Ignore errors (task may not exist)
	_, _ = root.CallMethod("DeleteTask", tsTaskName, 0)
	return nil
}
