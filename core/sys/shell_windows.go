//go:build windows

package sys

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

func ShellOpen(path string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}

	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}

	shell32 := windows.NewLazySystemDLL("shell32.dll")
	proc := shell32.NewProc("ShellExecuteW")

	ret, _, callErr := proc.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		0,
		0,
		uintptr(windows.SW_SHOWNORMAL),
	)

	if ret <= 32 {
		return fmt.Errorf("ShellExecuteW failed: %v", callErr)
	}

	return nil
}
