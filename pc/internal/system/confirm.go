package system

// Confirm asks the person a yes or no question in a native dialog and returns true only when they pick the action
// button. The default button is Cancel; a closed dialog, a failure to show one, and a system without a dialog all
// answer false, so that a question that cannot be asked never counts as a yes. It blocks until answered.
func Confirm(title, message, action string) bool { return confirm(title, message, action) }
