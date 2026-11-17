//go:build windows

package sys

import (
	jsonv2 "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"goclashz/core/logger"
	"golang.org/x/sys/windows"
)

// CheckAdmin 检查当前进程是否拥有 Windows 管理员权限
func CheckAdmin() bool {
	var token windows.Token
	if err := windows.OpenProcessToken(
		windows.CurrentProcess(),
		windows.TOKEN_QUERY,
		&token,
	); err != nil {
		return false
	}
	defer token.Close()

	return token.IsElevated()
}

// IsAdmin 是 CheckAdmin 的别名
func IsAdmin() bool {
	return CheckAdmin()
}

// beforeElevatedExit 是「提权（UAC）重启前」的清理回调。
//
// RequestAdmin / RequestAdminWithArgs 走的是 os.Exit(0)，defer 一律不执行；
// 如果不先停内核、摘掉系统代理，普通用户进程消失后注册表里的
// ProxyServer 仍然指向 127.0.0.1，用户会直接断网。
//
// 方向约束：sys 不能 import appcore（会形成环），因此用函数变量反向注入。
var beforeElevatedExit func()

// SetBeforeElevatedExit 注册提权退出前的清理回调（由 appcore 在 NewController 里注册）。
// 传 nil 可清除。回调内部 panic 会被吞掉并记录，绝不影响提权流程。
func SetBeforeElevatedExit(fn func()) {
	beforeElevatedExit = fn
}

// runBeforeElevatedExit 执行清理回调；未注册时是空操作（例如启动期 --repair-permissions
// 分支上的提权，此时还没有 Controller 实例）。
func runBeforeElevatedExit() {
	fn := beforeElevatedExit
	if fn == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("提权退出前清理回调 panic: %v", r)
		}
	}()
	fn()
}

// RequestAdmin 呼出 UAC 窗口并以管理员身份重新启动当前程序
func RequestAdmin() error {
	if CheckAdmin() {
		return nil // 已经是管理员，无需再次提权
	}

	verb := "runas"
	exe, _ := os.Executable()
	cwd, _ := os.Getwd()

	// 🚀 修复：使用 syscall.EscapeArg 安全转义所有参数，防止注入
	var safeArgs []string
	for _, arg := range os.Args[1:] {
		safeArgs = append(safeArgs, syscall.EscapeArg(arg))
	}
	args := strings.Join(safeArgs, " ")

	verbPtr, _ := syscall.UTF16PtrFromString(verb)
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	cwdPtr, _ := syscall.UTF16PtrFromString(cwd)
	argPtr, _ := syscall.UTF16PtrFromString(args)

	var showCmd int32 = 1 // SW_NORMAL

	// 🚀 核心：使用 ShellExecute 的 runas 动作触发 UAC
	err := windows.ShellExecute(0, verbPtr, exePtr, argPtr, cwdPtr, showCmd)
	if err != nil {
		return err
	}

	// 提权成功后，退出当前普通用户权限的进程。
	// os.Exit 不会执行 defer，代理/内核必须在退出前手动收拾。
	runBeforeElevatedExit()
	os.Exit(0)
	return nil
}

// RequestAdminWithArgs 以指定的参数和管理员身份重新启动当前程序
func RequestAdminWithArgs(extraArgs string) error {
	if CheckAdmin() {
		return nil // 已经是管理员，无需再次提权
	}

	verb := "runas"
	exe, _ := os.Executable()
	cwd, _ := os.Getwd()

	verbPtr, _ := syscall.UTF16PtrFromString(verb)
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	cwdPtr, _ := syscall.UTF16PtrFromString(cwd)
	argPtr, _ := syscall.UTF16PtrFromString(extraArgs)

	var showCmd int32 = 1 // SW_NORMAL

	err := windows.ShellExecute(0, verbPtr, exePtr, argPtr, cwdPtr, showCmd)
	if err != nil {
		return err
	}

	// 同 RequestAdmin：os.Exit 前先跑一遍清理。
	runBeforeElevatedExit()
	os.Exit(0)
	return nil
}

// ShellExecuteInfo 为 Windows ShellExecuteExW API 的结构体定义
type ShellExecuteInfo struct {
	CbSize       uint32
	FMask        uint32
	Hwnd         windows.Handle
	LpVerb       *uint16
	LpFile       *uint16
	LpParameters *uint16
	LpDirectory  *uint16
	NShow        int32
	HInstApp     windows.Handle
	LpIDList     uintptr
	LpClass      *uint16
	HkeyClass    windows.Handle
	HotKey       uint32
	Union        uintptr // hIcon or hMonitor
	HProcess     windows.Handle
}

var (
	shell32            = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteEx = shell32.NewProc("ShellExecuteExW")
)

const (
	SEE_MASK_NOCLOSEPROCESS = 0x00000040
)

// elevatedChildWaitTimeout 是等待管理员子进程的最长时间。
//
// 默认 10 分钟：正常提权任务（装/删/重启服务、修权限、UWP 豁免）都是秒级，
// 这个上限只用来兜住「SCM 卡在 STOP_PENDING」「子进程被系统对话框挂住」这类
// 会让 UI 永久转圈的情况。超时后强杀子进程并返回可读错误，不再无限等待。
var elevatedChildWaitTimeout = 10 * time.Minute

// waitTimeoutEvent 是 WaitForSingleObject 的超时返回值（winbase.h: WAIT_TIMEOUT）。
// x/sys/windows 只导出了 WAIT_OBJECT_0 / WAIT_ABANDONED / WAIT_FAILED，这里补一个。
const waitTimeoutEvent uint32 = 0x00000102

// RunElevatedWithArgsWait 以管理员身份运行指定参数的自身进程，并等待其执行完毕，不会退出当前进程
func RunElevatedWithArgsWait(args ...string) error {
	if CheckAdmin() {
		return nil // 已经是管理员，无需提权
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	escaped := make([]string, 0, len(args))
	for _, arg := range args {
		escaped = append(escaped, syscall.EscapeArg(arg))
	}
	param := strings.Join(escaped, " ")

	taskID := fmt.Sprintf("%d_%d", time.Now().UnixNano(), os.Getpid())
	os.Setenv("GOCLASHZ_ADMIN_TASK_ID", taskID)
	// 提权子进程会继承此环境变量

	verbPtr, _ := syscall.UTF16PtrFromString("runas")
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	argPtr, _ := syscall.UTF16PtrFromString(param)

	var sei ShellExecuteInfo
	sei.CbSize = uint32(unsafe.Sizeof(sei))
	sei.FMask = SEE_MASK_NOCLOSEPROCESS
	sei.LpVerb = verbPtr
	sei.LpFile = exePtr
	sei.LpParameters = argPtr
	sei.NShow = 0 // SW_HIDE

	r1, _, err := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&sei)))
	if r1 == 0 {
		if err != nil && err != windows.ERROR_SUCCESS {
			return fmt.Errorf("请求管理员权限失败或被取消: %w", err)
		}
		return fmt.Errorf("请求管理员权限失败或被取消")
	}
	if sei.HProcess == 0 {
		return fmt.Errorf("管理员子进程句柄为空")
	}
	defer windows.CloseHandle(sei.HProcess)

	waitMS := uint32(elevatedChildWaitTimeout / time.Millisecond)
	waitEvent, waitErr := windows.WaitForSingleObject(sei.HProcess, waitMS)
	if waitErr != nil {
		return fmt.Errorf("等待管理员子进程失败: %w", waitErr)
	}
	if waitEvent == waitTimeoutEvent {
		// 超时：子进程被服务管理器/系统对话框挂住时，UI 不能跟着无限等。
		_ = windows.TerminateProcess(sei.HProcess, 1)
		return fmt.Errorf("管理员子进程超过 %s 未完成，已强制终止；请重试，或改用「以管理员身份重启」后手动执行",
			elevatedChildWaitTimeout)
	}

	var exitCode uint32
	if err := windows.GetExitCodeProcess(sei.HProcess, &exitCode); err != nil {
		return fmt.Errorf("获取管理员子进程退出码失败: %w", err)
	}

	if exitCode != 0 {
		if msg := readAdminTaskError(); msg != "" {
			return fmt.Errorf("管理员子进程执行失败: %s", msg)
		}
		return fmt.Errorf("管理员子进程执行失败，退出码: %d", exitCode)
	}

	return nil
}

type AdminTaskResult struct {
	OK        bool   `json:"ok"`
	Operation string `json:"operation"`
	Error     string `json:"error,omitempty"`
}

func getAdminTaskResultPath() string {
	id := os.Getenv("GOCLASHZ_ADMIN_TASK_ID")
	if id == "" {
		id = "default"
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("goclashz_admin_task_%s.json", id))
}

// WriteAdminTaskResult 写入管理员提权任务的结果
func WriteAdminTaskResult(operation string, err error) {
	res := AdminTaskResult{
		OK:        err == nil,
		Operation: operation,
	}
	if err != nil {
		res.Error = err.Error()
	}
	data, _ := jsonv2.Marshal(res)
	_ = os.WriteFile(getAdminTaskResultPath(), data, 0666)
}

func readAdminTaskError() string {
	path := getAdminTaskResultPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	_ = os.Remove(path) // 读完删除
	var res AdminTaskResult
	if err := jsonv2.Unmarshal(data, &res); err == nil && !res.OK {
		return res.Error
	}
	return ""
}
