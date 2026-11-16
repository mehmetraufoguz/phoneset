//go:build !darwin && !windows

package system

// confirm: there is no dialog on this system, and a question that cannot be asked is not a yes.
func confirm(title, message, action string) bool { return false }
