package sys

import (
	"os"
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
	
	// 继承启动参数
	args := strings.Join(os.Args[1:], " ")

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
