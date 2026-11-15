//go:build darwin

package camera

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mehmetraufoguz/phoneset/pc/internal/update"
)

const (
	appName     = "phoneset Camera.app"
	appPath     = "/Applications/" + appName
	extensionID = "com.rapp.phoneset.camera.extension"
	// assetName is the camera app in a release: a zip of the signed, notarized app (macos/release.sh).
	assetName = "phoneset-camera-macos.zip"
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

// Install puts the app in /Applications (macOS only installs a camera from there) and asks for the extension.
// The app comes from next to the phoneset program if it is there; else the one that is installed stays, unless
// Force asks for the latest; else it is downloaded from the latest release.
func (macOS) Install(ctx context.Context, o InstallOptions) error {
	_, haveApp := os.Stat(appPath)
	local := filepath.Join(o.ExeDir, appName)
	if _, err := os.Stat(local); err == nil {
		o.say("installing " + local)
		if err := place(local); err != nil {
			return err
		}
	} else if haveApp != nil || o.Force {
		if err := download(ctx, o); err != nil {
			return err
		}
	}
	return launch("--install")
}

// download fetches the camera app of the latest release, checks it and puts it in /Applications.
func download(ctx context.Context, o InstallOptions) error {
	cl := update.NewClient()
	rel, err := cl.Latest(ctx)
	if err != nil {
		return fmt.Errorf("couldn't look for the camera app: %w", err)
	}
	if _, ok := rel.Assets[assetName]; !ok {
		return fmt.Errorf("release %s has no camera app for macOS (%s). Build it with macos/build.sh and put "+
			"“phoneset Camera.app” next to the phoneset program, or wait for a release that has it", rel.Tag, assetName)
	}
	tmp, err := os.MkdirTemp("", "phoneset-camera-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	zip, err := cl.Fetch(ctx, rel, assetName, tmp, o.say)
	if err != nil {
		return err
	}
	// ditto keeps what a signed app needs (permissions, extended attributes, the signature) that an unzip may drop.
	unpacked := filepath.Join(tmp, "app")
	if out, err := exec.CommandContext(ctx, "ditto", "-x", "-k", zip, unpacked).CombinedOutput(); err != nil {
		return fmt.Errorf("couldn't unpack the camera app: %v %s", err, out)
	}
	o.say("release " + rel.Tag)
	return place(filepath.Join(unpacked, appName))
}

// place replaces the app in /Applications with src. An old copy goes as a whole: a half-updated bundle fails its signature.
func place(src string) error {
	if running() {
		_ = exec.Command("pkill", "-f", appPath+"/Contents/MacOS/").Run()
		time.Sleep(700 * time.Millisecond)
	}
	if err := os.RemoveAll(appPath); err != nil {
		return fmt.Errorf("couldn't replace %s: %w", appPath, err)
	}
	if out, err := exec.Command("ditto", src, appPath).CombinedOutput(); err != nil {
		return fmt.Errorf("couldn't copy the camera to /Applications: %v %s", err, out)
	}
	return nil
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
