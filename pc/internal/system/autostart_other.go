//go:build !windows && !darwin

package system

type noAutostart struct{}

func NewAutostart() Autostart { return noAutostart{} }

func (noAutostart) Enabled() (bool, error) { return false, ErrUnsupported }
func (noAutostart) Enable(string) error    { return ErrUnsupported }
func (noAutostart) Disable() error         { return ErrUnsupported }
