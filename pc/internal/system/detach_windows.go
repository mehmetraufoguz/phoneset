//go:build windows

package system

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// Detach makes cmd outlive the console that started it, and gives it no console at all.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}
}

// HideConsole closes the console window this process was started with. Used when Windows
// launches the background service at login, which would otherwise leave a window open.
func HideConsole() {
	_, _, _ = syscall.NewLazyDLL("kernel32.dll").NewProc("FreeConsole").Call()
}
