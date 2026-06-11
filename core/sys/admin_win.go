//go:build windows

package sys

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

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

// RunElevatedWithArgsWait 以管理员身份运行指定参数的自身进程，并等待其执行完毕，不会退出当前进程
func RunElevatedWithArgsWait(extraArgs string) error {
	if CheckAdmin() {
		return nil // 已经是管理员，无需提权
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	// 使用 PowerShell Start-Process -Wait 实现提权并等待
	psCmd := fmt.Sprintf("Start-Process -FilePath '%s' -ArgumentList '%s' -Verb RunAs -Wait -WindowStyle Hidden", exe, extraArgs)
	cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", psCmd)
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("提权执行失败: %v", err)
	}

	return nil
}
