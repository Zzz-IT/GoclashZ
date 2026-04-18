package sys

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// 获取系统真实绝对路径，杜绝 os.Getwd() 的隐患
func getSysExeDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exePath)
}

// CheckAdmin 检查当前程序是否以管理员权限运行
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

// CheckWintun 检查核心目录下是否存在 wintun.dll
func CheckWintun() bool {
	dllPath := filepath.Join(getSysExeDir(), "core", "bin", "wintun.dll")
	_, err := os.Stat(dllPath)
	return err == nil || !os.IsNotExist(err)
}
