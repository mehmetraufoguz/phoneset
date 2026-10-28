//go:build windows

package term

import (
	"os"

	"golang.org/x/sys/windows"
)

const cpUTF8 = 65001

// prepareConsole switches the console to UTF-8 and enables ANSI escape handling.
// It reports whether escapes will be honoured.
func prepareConsole() bool {
	_ = windows.SetConsoleOutputCP(cpUTF8)
	h := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false // redirected to a file or pipe
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
