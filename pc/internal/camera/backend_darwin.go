//go:build darwin

package camera

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	appName     = "phoneset Camera.app"
	appPath     = "/Applications/" + appName
	extensionID = "com.rapp.phoneset.camera.extension"
)

// Default is the macOS camera: the app "phoneset Camera" in /Applications, with a camera extension inside it.
// The app is also what carries the picture (it reads the program's frames and writes them to the extension).
func Default() Backend { return macOS{} }

type macOS struct{}

func (macOS) Probe() State {
	s := State{Phase: NotInstalled, Running: running()}
	if out, err := exec.Command("systemextensionsctl", "list").Output(); err == nil {
		s.Phase = PhaseFromExtensions(string(out), extensionID)
	}
	switch s.Phase {
	case NotInstalled:
		s.Hint = "Run `phoneset camera install`."
	case NeedsApproval:
		s.Hint = "Allow it in System Settings: General, Login Items & Extensions, Camera Extensions, switch on phoneset Camera."
	case SwitchedOff:
		s.Hint = "It is switched off in System Settings: General, Login Items & Extensions, Camera Extensions."
	}
	return s
}

func (macOS) Install(exeDir string) error {
	if _, err := os.Stat(appPath); err != nil {
		src := filepath.Join(exeDir, appName)
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("%s was found neither next to phoneset nor in /Applications", appName)
		}
		// macOS only installs a camera from an app in /Applications.
		if out, err := exec.Command("ditto", src, appPath).CombinedOutput(); err != nil {
			return fmt.Errorf("couldn't copy the camera to /Applications: %v %s", err, out)
		}
	}
	return launch("--install")
}

func (macOS) Uninstall() error {
	if _, err := os.Stat(appPath); err != nil {
		return fmt.Errorf("%s is not in /Applications", appName)
	}
	return launch("--uninstall")
}

func (macOS) Start() error {
	if running() {
		return nil
	}
	if _, err := os.Stat(appPath); err != nil {
		return nil // not installed: nothing to start
	}
	return exec.Command("open", "-g", appPath, "--args", "--background").Run()
}

func running() bool {
	return exec.Command("pgrep", "-f", appPath+"/Contents/MacOS/").Run() == nil
}

// launch starts the app with a request. A running app would not get new arguments from `open`, so it is ended first.
func launch(arg string) error {
	if running() {
		_ = exec.Command("pkill", "-f", appPath+"/Contents/MacOS/").Run()
		time.Sleep(700 * time.Millisecond)
	}
	return exec.Command("open", "-g", appPath, "--args", arg).Run()
}
