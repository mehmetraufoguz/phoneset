//go:build !darwin

package camera

import (
	"fmt"
	"runtime"
)

// Default is the camera of a system that has none of our own yet.
func Default() Backend { return unsupported{hint: hint()} }

type unsupported struct{ hint string }

func hint() string {
	switch runtime.GOOS {
	case "windows":
		return "There is no camera of phoneset's own for Windows yet. For now use OBS Studio: add a Browser source with " +
			"the address `phoneset video` prints, then Start Virtual Camera."
	case "linux":
		return "A camera for Linux (v4l2loopback and ffmpeg) is planned and not built yet. For now use OBS Studio with " +
			"its Virtual Camera: add a Browser source with the address `phoneset video` prints."
	default:
		return "There is no virtual camera of phoneset for this system."
	}
}

func (u unsupported) Probe() State { return State{Phase: Unsupported, Hint: u.hint} }

func (u unsupported) Install(string) error { return fmt.Errorf("%w. %s", ErrUnsupported, u.hint) }

func (u unsupported) Uninstall() error { return fmt.Errorf("%w. %s", ErrUnsupported, u.hint) }

func (unsupported) Start() error { return nil }
