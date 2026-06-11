//go:build windows

package sys

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CheckAdmin 检查当前进程是否拥有 Windows 管理员权限
// 原理：普通用户无法打开 PHYSICALDRIVE0 物理磁盘句柄
func CheckAdmin() bool {

	_, err := os.Open("\\\\.\\PHYSICALDRIVE0")
	if err != nil {
		return false
	}
	return true
}

// IsAdmin 是 CheckAdmin 的别名
func IsAdmin() bool {
	return CheckAdmin()
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

	// 提权成功后，退出当前普通用户权限的进程
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

	_, err = windows.WaitForSingleObject(sei.HProcess, windows.INFINITE)
	if err != nil {
		return fmt.Errorf("等待管理员子进程失败: %w", err)
	}

	var exitCode uint32
	if err := windows.GetExitCodeProcess(sei.HProcess, &exitCode); err != nil {
		return fmt.Errorf("获取管理员子进程退出码失败: %w", err)
	}

	if exitCode != 0 {
		return fmt.Errorf("管理员子进程执行失败，退出码: %d", exitCode)
	}

	return nil
}
