package system

import "errors"

// ErrUnsupported is returned where start-at-login isn't implemented.
var ErrUnsupported = errors.New("starting at login is only supported on Windows and macOS")

// Autostart manages starting phoneset in the background when the user logs in.
type Autostart interface {
	Enabled() (bool, error)
	Enable(exe string) error
	Disable() error
}
