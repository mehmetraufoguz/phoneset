//go:build windows

package system

import "golang.org/x/sys/windows"

// idYes is what MessageBox returns for the Yes button (IDYES in winuser.h).
const idYes = 6

// confirm shows a Yes/No message box with No as the default and keeps it on top (the program has no window).
func confirm(title, message, action string) bool {
	text, err1 := windows.UTF16PtrFromString(message + "\n\n" + action + "?")
	caption, err2 := windows.UTF16PtrFromString(title)
	if err1 != nil || err2 != nil {
		return false
	}
	ret, err := windows.MessageBox(0, text, caption, windows.MB_YESNO|windows.MB_ICONWARNING|windows.MB_DEFBUTTON2|windows.MB_TOPMOST|windows.MB_SETFOREGROUND)
	return err == nil && ret == idYes
}
