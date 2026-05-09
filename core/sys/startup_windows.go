//go:build windows

package sys

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-ole/go-ole"
)

const (
	tsActionExec     = 0 // TASK_ACTION_EXEC
	tsTriggerLogon   = 9 // TASK_TRIGGER_LOGON
	tsCreateOrUpdate = 6 // TASK_CREATE_OR_UPDATE
)

var clsidTaskScheduler = ole.NewGUID("{0F87369F-A4E5-4CFC-BD3E-73E6154572DD}")

const tsTaskName = "GoclashZ Startup"

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

// CheckStartupTask returns true if the GoclashZ startup task exists.
func CheckStartupTask() bool {
	cleanup, err := initCOM()
	if err != nil {
		return false
	}
	defer cleanup()

	sched, err := newTaskScheduler()
	if err != nil {
		return false
	}
	defer sched.Release()

	rootV, err := sched.CallMethod("GetFolder", `\`)
	if err != nil {
		return false
	}
	root := rootV.ToIDispatch()
	defer root.Release()

	taskV, err := root.CallMethod("GetTask", tsTaskName)
	if err != nil {
		return false
	}
	taskV.ToIDispatch().Release()
	return true
}

// CreateStartupTask registers a Task Scheduler task that launches exePath at user logon
// with limited (non-elevated) privileges.
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

	// NewTask(0) → blank ITaskDefinition
	defV, err := sched.CallMethod("NewTask", 0)
	if err != nil {
		return fmt.Errorf("创建任务定义失败: %w", err)
	}
	def := defV.ToIDispatch()
	defer def.Release()

	// --- Settings ---
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

	// --- Action: Exec ---
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
	action.PutProperty("WorkingDirectory", workDir)
	action.Release()

	// --- Trigger: Logon ---
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
	trigger.Release()

	// --- Principal: current user, non-elevated ---
	principalV, err := def.GetProperty("Principal")
	if err != nil {
		return fmt.Errorf("获取 Principal 失败: %w", err)
	}
	principal := principalV.ToIDispatch()
	principal.PutProperty("LogonType", 3) // TASK_LOGON_TOKEN
	principal.PutProperty("RunLevel", 0)  // TASK_RUNLEVEL_LUA
	principal.Release()

	// --- Display metadata ---
	def.PutProperty("DisplayName", tsTaskName)
	def.PutProperty("Description", "开机自启 GoclashZ 代理客户端")

	// --- Register ---
	rootV, err := sched.CallMethod("GetFolder", `\`)
	if err != nil {
		return fmt.Errorf("获取根文件夹失败: %w", err)
	}
	root := rootV.ToIDispatch()
	defer root.Release()

	_, err = root.CallMethod("RegisterTaskDefinition",
		tsTaskName,
		def,
		tsCreateOrUpdate,
		"",  // userId: current user
		nil, // password
		3,   // logonType: TASK_LOGON_TOKEN
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
