// Package system holds the few operating-system specifics of running in the background:
// opening a page in the browser, detaching from the terminal and starting at login.
package system

import (
	"os/exec"
	"runtime"
)

// OpenURL opens a page in the default browser.
func OpenURL(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// OpenFile opens a file or folder with the system's default app.
func OpenFile(path string) error { return OpenURL(path) }
