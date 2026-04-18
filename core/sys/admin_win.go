package sys

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// CheckAdmin 检查当前是否拥有管理员权限
func CheckAdmin() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}
	return member
}

// RequestAdmin 自动弹出 UAC 窗口，以管理员身份重新运行本程序
func RequestAdmin() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	verb := windows.StringToUTF16Ptr("runas")
	exe := windows.StringToUTF16Ptr(exePath)
	cwd := windows.StringToUTF16Ptr(filepath.Dir(exePath))
	
	// 将当前参数传递给新进程（如果有）
	args := windows.StringToUTF16Ptr(strings.Join(os.Args[1:], " "))

	err = windows.ShellExecute(0, verb, exe, args, cwd, syscall.SW_NORMAL)
	if err != nil {
		return fmt.Errorf("请求管理员权限失败: %v", err)
	}

	// 提权成功后，退出当前普通权限的进程
	os.Exit(0)
	return nil
}
