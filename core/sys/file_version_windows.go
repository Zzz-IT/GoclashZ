//go:build windows

package sys

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

func GetFileVersion(path string) (string, error) {
	var handle windows.Handle
	size, err := windows.GetFileVersionInfoSize(path, &handle)
	if err != nil || size == 0 {
		return "", err
	}

	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return "", err
	}

	var fixed *windows.VS_FIXEDFILEINFO
	var fixedLen uint32

	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\`, unsafe.Pointer(&fixed), &fixedLen); err != nil {
		return "", err
	}

	major := fixed.FileVersionMS >> 16
	minor := fixed.FileVersionMS & 0xffff
	build := fixed.FileVersionLS >> 16
	rev := fixed.FileVersionLS & 0xffff

	return fmt.Sprintf("%d.%d.%d.%d", major, minor, build, rev), nil
}
