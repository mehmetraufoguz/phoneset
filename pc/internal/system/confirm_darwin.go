//go:build darwin

package system

import (
	"os/exec"
	"strings"
)

// quote is an AppleScript string literal.
func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// confirm shows a dialog with osascript: it exits with 0 when the action button is pressed and with an error for Cancel
// (-128). It runs in System Events so that the dialog comes to the front even though phoneset has no window.
func confirm(title, message, action string) bool {
	script := `tell application "System Events"
activate
display dialog ` + quote(message) + ` with title ` + quote(title) + ` buttons {"Cancel", ` + quote(action) + `} default button "Cancel" cancel button "Cancel" with icon caution
end tell`
	return exec.Command("osascript", "-e", script).Run() == nil
}
