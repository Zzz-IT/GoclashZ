//go:build !windows

package utils

import "os/exec"

func HideCommandWindow(cmd *exec.Cmd, extraFlags uint32) {
}
