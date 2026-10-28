//go:build !windows

package system

import (
	"os/exec"
	"syscall"
)

// Detach makes cmd outlive the terminal that started it.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// HideConsole is only needed on Windows.
func HideConsole() {}
