//go:build !windows

package sys

import "fmt"

func ShellOpen(path string) error {
	return fmt.Errorf("ShellOpen unsupported on this platform")
}
