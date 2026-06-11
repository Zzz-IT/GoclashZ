//go:build windows

package sys

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-ole/go-ole"
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
	StartupElevated StartupMode = "elevated"
)

type StartupTaskInfo struct {
	Exists    bool        `json:"exists"`
	Enabled   bool        `json:"enabled"`
	Mode      StartupMode `json:"mode"`
	Path      string      `json:"path"`
	Arguments string      `json:"arguments"`
	RunLevel  int         `json:"runLevel"`
	LastError string      `json:"lastError"`
}

// initCOM initializes COM and returns a cleanup function.
func initCOM() (func(), error) {
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
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

// CheckStartupTask returns true if the GoclashZ startup task exists and is enabled.
// It also returns detailed task info.
func CheckStartupTask() (StartupTaskInfo, error) {
	info := StartupTaskInfo{Mode: StartupDisabled}

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
		return info, nil
	}
	info.Exists = true
	task := taskV.ToIDispatch()
	defer task.Release()

	// Check if enabled
	enabledV, err := task.GetProperty("Enabled")
	if err == nil {
		info.Enabled = enabledV.Value().(bool)
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
				switch v := runLevelV.Value().(type) {
				case int32:
					info.RunLevel = int(v)
				case int16:
					info.RunLevel = int(v)
				case int:
					info.RunLevel = v
				}
			}
			prin.Release()
		}

		// Get Actions
		actionsV, err := def.GetProperty("Actions")
		if err == nil {
			actions := actionsV.ToIDispatch()
			actionCountV, err := actions.GetProperty("Count")
			if err == nil && actionCountV.Value().(int32) > 0 {
				actionV, err := actions.GetProperty("Item", 1) // 1-indexed in COM collections
				if err == nil {
					action := actionV.ToIDispatch()
					pathV, err := action.GetProperty("Path")
					if err == nil && pathV.Value() != nil {
						info.Path = pathV.Value().(string)
					}
					argsV, err := action.GetProperty("Arguments")
					if err == nil && argsV.Value() != nil {
						info.Arguments = argsV.Value().(string)
					}
					action.Release()
				}
			}
			actions.Release()
		}

		if info.RunLevel == 1 {
			info.Mode = StartupElevated
		} else {
			info.Mode = StartupNormal
		}
		
		// Validation
		exe, _ := os.Executable()
		if info.Path != exe || !strings.Contains(info.Arguments, "--startup") {
			info.Enabled = false
			info.LastError = "path mismatch or missing --startup"
		}
		
		if !info.Enabled {
			info.Mode = StartupDisabled
		}
	}

	return info, nil
}

func CreateStartupTask(exePath string) error {
	return createStartupTaskInternal(exePath, false)
}

func CreateElevatedStartupTask(exePath string) error {
	return createStartupTaskInternal(exePath, true)
}

func createStartupTaskInternal(exePath string, elevated bool) error {
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
	
	if elevated {
		settings.PutProperty("MultipleInstances", 1) // IgnoreNew (1)
		settings.PutProperty("StartWhenAvailable", true)
		settings.PutProperty("RestartCount", 3)
		settings.PutProperty("RestartInterval", "PT1M")
	}
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
	args := "--startup --silent"
	if elevated {
		args += " --elevated"
	}
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
	if elevated {
		principal.PutProperty("LogonType", 3) // TASK_LOGON_INTERACTIVE_TOKEN
		principal.PutProperty("RunLevel", 1)  // TASK_RUNLEVEL_HIGHEST (Elevated)
	} else {
		principal.PutProperty("LogonType", 3) // TASK_LOGON_TOKEN
		principal.PutProperty("RunLevel", 0)  // TASK_RUNLEVEL_LUA
	}
	principal.Release()

	def.PutProperty("DisplayName", tsTaskName)
	if elevated {
		def.PutProperty("Description", "开机自启 GoclashZ 代理客户端 (管理员权限)")
	} else {
		def.PutProperty("Description", "开机自启 GoclashZ 代理客户端")
	}

	rootV, err := sched.CallMethod("GetFolder", `\`)
	if err != nil {
		return fmt.Errorf("获取根文件夹失败: %w", err)
	}
	root := rootV.ToIDispatch()
	defer root.Release()

	logonType := int32(3) // TASK_LOGON_TOKEN
	if elevated {
		logonType = 3 // TASK_LOGON_INTERACTIVE_TOKEN -> it's the same enum value, 3.
	}

	_, err = root.CallMethod("RegisterTaskDefinition",
		tsTaskName,
		def,
		tsCreateOrUpdate,
		"",  // userId: current user
		nil, // password
		logonType,
	)
	if err != nil {
		if elevated {
			return fmt.Errorf("注册管理员计划任务失败: %w", err)
		}
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
