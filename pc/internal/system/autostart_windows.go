//go:build windows

package system

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

const (
	runKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue = "phoneset"
)

type winAutostart struct{}

// NewAutostart returns the Windows implementation: a value under the per-user Run key,
// so no administrator rights are needed.
func NewAutostart() Autostart { return winAutostart{} }

func (winAutostart) Enabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false, err
	}
	defer k.Close()
	if _, _, err := k.GetStringValue(runValue); err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (winAutostart) Enable(exe string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	// `daemon` starts the background service without a console window and exits.
	return k.SetStringValue(runValue, `"`+exe+`" daemon --quiet`)
}

func (winAutostart) Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(runValue); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}
