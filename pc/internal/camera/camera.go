// Package camera is the virtual camera of phoneset as the PC program sees it: "phoneset Camera", a camera other
// programs (Zoom, Meet, FaceTime, ...) can pick, showing the picture of the phone. The part that makes it is native to
// each operating system (on macOS a companion app with a camera extension, see docs/virtual-camera.md); this package
// is the one place the program controls it from: the command `phoneset camera`, the control API and the tray all
// go through it, whatever the system.
package camera

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Phase is how far the camera is on this PC.
type Phase string

const (
	// Unsupported: there is no camera of our own on this system (yet); Hint says what to use instead.
	Unsupported Phase = "unsupported"
	// NotInstalled: it is not on this PC yet.
	NotInstalled Phase = "not installed"
	// NeedsApproval: installed, but the system waits for the user to allow it.
	NeedsApproval Phase = "waiting for approval"
	// SwitchedOff: installed and allowed once, then switched off by the user.
	SwitchedOff Phase = "switched off"
	// Ready: programs can pick it.
	Ready Phase = "ready"
	// Removing: a removal was asked for through this program and the system has not finished it (it may wait for the
	// user to confirm). Only the Monitor says this; a backend never does.
	Removing Phase = "being removed"
)

// State is what the program shows of the camera.
type State struct {
	Phase Phase `json:"phase"`
	// Hint says what to do next, or what to use where there is no camera of our own.
	Hint string `json:"hint,omitempty"`
	// Running: the part that carries the phone's picture to the camera is running.
	Running bool `json:"running"`
	// Line and Flowing are what that part last said (only while it keeps reporting).
	Line    string `json:"line,omitempty"`
	Flowing bool   `json:"flowing"`
}

// Report is what the running part says about itself, once a second.
type Report struct {
	Line    string `json:"line"`
	Flowing bool   `json:"flowing"`
}

// InstallOptions say where the camera's files come from and how to report progress.
type InstallOptions struct {
	// ExeDir is the folder of the phoneset program: a camera found there is the one installed.
	ExeDir string
	// Force fetches the latest release's camera even when one is installed already (an update).
	Force bool
	// Say is told what is going on (downloading, checking); it may be nil.
	Say func(string)
}

func (o InstallOptions) say(s string) {
	if o.Say != nil {
		o.Say(s)
	}
}

// ErrUnsupported is returned by a system that has no camera of our own.
var ErrUnsupported = errors.New("there is no virtual camera of phoneset for this system yet")

// Backend is the camera of one operating system.
type Backend interface {
	// Probe looks at the system: is the camera there and switched on, is the part that carries the picture running.
	Probe() State
	// Install puts the camera in place (from next to the phoneset program, else from what is on the PC, else
	// downloaded from the latest release) and asks the system to turn it on. It returns once the request is made;
	// Probe shows the outcome (the system may wait for the user).
	Install(ctx context.Context, o InstallOptions) error
	Uninstall() error
	// Start starts the part that carries the picture, when a phone begins to send it.
	Start() error
}

// ConfigDirSetter is implemented by a backend that starts a part which has to find the running program: it is told
// the program's config folder (where control.json is), so that a program started with --config-dir is found too.
type ConfigDirSetter interface{ SetConfigDir(dir string) }

// How long a report counts as "now", and how long a look at the system is kept.
const (
	reportFresh = 4 * time.Second
	probeFresh  = 2 * time.Second
)

// Monitor keeps the state of the camera: what the system says (looked at not more than every two seconds) and what
// the running part last reported.
type Monitor struct {
	backend Backend

	mu       sync.Mutex
	probed   State
	probedAt time.Time
	report   Report
	reportAt time.Time
	changed  func()
	// removingSince is when a removal was asked for through Uninstall; zero when none is going on.
	removingSince time.Time
	// following: a goroutine is watching the system after an install or a removal was asked for.
	following bool
}

func NewMonitor(b Backend) *Monitor { return &Monitor{backend: b} }

// Backend is the system's camera.
func (m *Monitor) Backend() Backend { return m.backend }

// OnChange registers a function called when a report changes what is shown.
func (m *Monitor) OnChange(fn func()) {
	m.mu.Lock()
	m.changed = fn
	m.mu.Unlock()
}

// State is the camera as it is now.
func (m *Monitor) State() State { return m.state(false) }

// Refresh looks at the system again, whatever was seen a moment ago.
func (m *Monitor) Refresh() State { return m.state(true) }

func (m *Monitor) state(force bool) State {
	m.mu.Lock()
	if force || time.Since(m.probedAt) > probeFresh {
		m.mu.Unlock()
		s := m.backend.Probe() // runs a command: not under the lock
		m.mu.Lock()
		m.probed, m.probedAt = s, time.Now()
	}
	s := m.probed
	s = m.markRemoving(s)
	if time.Since(m.reportAt) < reportFresh {
		s.Running, s.Line, s.Flowing = true, m.report.Line, m.report.Flowing
	}
	m.mu.Unlock()
	return s
}

// removalPatience is how long a removal that the system has not finished still counts as going on.
const removalPatience = followPatience

// markRemoving shows "being removed" while a removal asked for through Uninstall is not finished: the system lists
// the camera in the states of an install while it waits for the user (macOS: "waiting for user"), and the hint of an
// install ("allow it, switch it on") is the opposite of what the person has to do. The caller holds the lock.
func (m *Monitor) markRemoving(s State) State {
	if m.removingSince.IsZero() {
		return s
	}
	if s.Phase == NotInstalled || s.Phase == Unsupported || time.Since(m.removingSince) > removalPatience {
		m.removingSince = time.Time{}
		return s
	}
	s.Phase = Removing
	s.Hint = removingHint()
	return s
}

func removingHint() string {
	if runtime.GOOS == "darwin" {
		return "macOS is waiting for you to confirm the removal: a password or Touch ID prompt, or System Settings, " +
			"General, Login Items & Extensions, Camera Extensions."
	}
	return "The system is removing it."
}

// Uninstall asks the backend to remove the camera and, until the system says it is gone, shows it as being removed.
func (m *Monitor) Uninstall() error {
	if err := m.backend.Uninstall(); err != nil {
		return err
	}
	m.mu.Lock()
	m.removingSince = time.Now()
	m.probedAt = time.Time{} // look at the system again
	m.mu.Unlock()
	m.notifyChanged()
	m.follow(func(s State) bool { return s.Phase == NotInstalled || s.Phase == Unsupported })
	return nil
}

// Install asks the backend to put the camera in place and, until the system says it is ready, keeps the display up to
// date (the system may wait for the person, and finishes in its own time).
func (m *Monitor) Install(ctx context.Context, o InstallOptions) error {
	if err := m.backend.Install(ctx, o); err != nil {
		return err
	}
	m.mu.Lock()
	m.probedAt = time.Time{}
	m.mu.Unlock()
	m.notifyChanged()
	m.follow(func(s State) bool { return s.Phase == Ready || s.Phase == Unsupported })
	return nil
}

func (m *Monitor) notifyChanged() {
	m.mu.Lock()
	fn := m.changed
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// followPatience is how long the display is kept up to date after an install or a removal was asked for.
const followPatience = 15 * time.Minute

// follow looks at the system every two seconds until done says so (or the patience ends) and tells whoever listens
// whenever what is shown changes. Without it a tray would keep "being removed" for good: nothing else tells it that
// the system has finished. One watcher at a time.
func (m *Monitor) follow(done func(State) bool) {
	m.mu.Lock()
	if m.following {
		m.mu.Unlock()
		return
	}
	m.following = true
	m.mu.Unlock()
	go func() {
		defer func() {
			m.mu.Lock()
			m.following = false
			m.mu.Unlock()
			m.notifyChanged()
		}()
		var last State
		deadline := time.Now().Add(followPatience)
		for time.Now().Before(deadline) {
			time.Sleep(2 * time.Second)
			s := m.Refresh()
			if s.Phase != last.Phase || s.Hint != last.Hint {
				m.notifyChanged()
			}
			last = s
			if done(s) {
				return
			}
		}
	}()
}

// Report records what the running part said.
func (m *Monitor) Report(r Report) {
	m.mu.Lock()
	changed := r != m.report || time.Since(m.reportAt) >= reportFresh
	m.report, m.reportAt = r, time.Now()
	fn := m.changed
	m.mu.Unlock()
	if changed && fn != nil {
		fn()
	}
}

// Start starts the part that carries the picture, if the camera is ready and that part is not running.
func (m *Monitor) Start() error {
	if s := m.State(); s.Phase != Ready || s.Running {
		return nil
	}
	return m.backend.Start()
}

// PhaseFromExtensions reads the output of `systemextensionsctl list` (macOS) and says how far the extension with
// this identifier is: it looks at every line that names it and takes the furthest state. Versions that were replaced
// ("terminated waiting to uninstall on reboot") do not count.
func PhaseFromExtensions(list, identifier string) Phase {
	phase := NotInstalled
	for _, line := range strings.Split(list, "\n") {
		if !strings.Contains(line, identifier) {
			continue
		}
		switch {
		case strings.Contains(line, "[activated enabled]"):
			return Ready
		case strings.Contains(line, "[activated waiting for user]"):
			phase = NeedsApproval
		case strings.Contains(line, "[activated disabled]"):
			if phase == NotInstalled {
				phase = SwitchedOff
			}
		}
	}
	return phase
}
