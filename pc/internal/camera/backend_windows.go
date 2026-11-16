//go:build windows

package camera

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/mehmetraufoguz/phoneset/pc/internal/update"
)

const (
	// assetName is the camera in a release: a zip of softcam.dll, phoneset-camera.exe and the DLL's licence
	// (windows/release.ps1).
	assetName  = "phoneset-camera-windows.zip"
	dllName    = "softcam.dll"
	helperName = "phoneset-camera.exe"
	// clsid is the camera's class id; it is the one windows/build.ps1 puts in the DLL.
	clsid = "{43750394-889D-43A4-807F-5FB98F8FCA5D}"
	// helperMutex is held by the running helper (one per user session), helperQuitEvent is what `--quit` signals.
	helperMutex = `Local\phoneset-camera-helper`
)

// Default is the Windows camera: a DirectShow filter ("phoneset Camera", a camera that Chrome, Edge, OBS, Zoom and
// the like list) registered under HKLM, and a helper process that carries the phone's picture to it
// (docs/virtual-camera.md, phase 3). Registering writes to HKLM, so putting it in place and taking it away asks for
// administrator rights (the UAC window).
func Default() Backend { return &windowsCamera{} }

type windowsCamera struct {
	// configDir is where the running program keeps control.json, which the helper reads to find it ("" is the
	// default folder, which is also the helper's default).
	configDir string
}

// SetConfigDir tells the backend which folder the running program uses, so that the helper it starts finds it.
func (w *windowsCamera) SetConfigDir(dir string) { w.configDir = dir }

func notAmd64() State {
	return State{Phase: Unsupported, Hint: "The camera of phoneset for Windows is a 64-bit (x64) component and this PC runs " +
		runtime.GOARCH + ". For now use OBS Studio: add a Browser source with the address `phoneset video` prints, " +
		"then Start Virtual Camera."}
}

func (w *windowsCamera) Probe() State {
	if runtime.GOARCH != "amd64" {
		return notAmd64()
	}
	s := State{Phase: NotInstalled, Running: helperRunning()}
	dll, registered := registeredDLL()
	switch {
	case !registered:
		s.Hint = "Run `phoneset camera install`: Windows asks for administrator rights once, to register the camera."
	case !fileExists(dll):
		s.Hint = "The camera is registered, but its file is gone (" + dll + "). Run `phoneset camera install --force`."
	default:
		s.Phase = Ready
	}
	return s
}

// Install puts the camera in place. The files come from next to the phoneset program if they are there (a build
// from windows\build.ps1 copied beside it); else an installed camera stays, unless Force asks for the latest; else the
// latest release's zip is downloaded, checked against SHA256SUMS and unpacked. Then the program starts itself again
// with administrator rights (the UAC window) to copy the files to Program Files and register the DLL.
func (w *windowsCamera) Install(ctx context.Context, o InstallOptions) error {
	if runtime.GOARCH != "amd64" {
		return fmt.Errorf("%w. %s", ErrUnsupported, notAmd64().Hint)
	}
	src, version, cleanup, err := w.source(ctx, o)
	if err != nil {
		return err
	}
	defer cleanup()
	if src == "" {
		return nil // installed already and nothing newer was asked for
	}
	o.say("asking for administrator rights to register the camera")
	return runElevatedStep(ctx, "install", "--from", src, "--version", version)
}

// source finds the camera's files: the folder to install from, the release's name, and what to delete afterwards. An
// empty folder means there is nothing to do.
func (w *windowsCamera) source(ctx context.Context, o InstallOptions) (dir, version string, cleanup func(), err error) {
	none := func() {}
	if fileExists(filepath.Join(o.ExeDir, dllName)) && fileExists(filepath.Join(o.ExeDir, helperName)) {
		o.say("installing the camera found next to the program: " + o.ExeDir)
		return o.ExeDir, "local build", none, nil
	}
	if w.Probe().Phase == Ready && !o.Force {
		return "", "", none, nil
	}
	cl := update.NewClient()
	rel, err := cl.Latest(ctx)
	if err != nil {
		return "", "", none, fmt.Errorf("couldn't look for the camera: %w", err)
	}
	if _, ok := rel.Assets[assetName]; !ok {
		return "", "", none, fmt.Errorf("release %s has no camera for Windows (%s). Build it with windows\\build.ps1 and "+
			"put softcam.dll and phoneset-camera.exe next to the phoneset program, or wait for a release that has it",
			rel.Tag, assetName)
	}
	tmp, err := os.MkdirTemp("", "phoneset-camera-")
	if err != nil {
		return "", "", none, err
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }
	zipPath, err := cl.Fetch(ctx, rel, assetName, tmp, o.say)
	if err != nil {
		cleanup()
		return "", "", none, err
	}
	pkg := filepath.Join(tmp, "pkg")
	if err := extractZip(zipPath, pkg); err != nil {
		cleanup()
		return "", "", none, err
	}
	for _, f := range []string{dllName, helperName} {
		if !fileExists(filepath.Join(pkg, f)) {
			cleanup()
			return "", "", none, fmt.Errorf("the camera zip of release %s has no %s", rel.Tag, f)
		}
	}
	o.say("release " + rel.Tag)
	return pkg, rel.Tag, cleanup, nil
}

func (w *windowsCamera) Uninstall() error {
	if _, registered := registeredDLL(); !registered {
		if dir, err := installDir(); err != nil || !fileExists(filepath.Join(dir, dllName)) {
			return errors.New("the camera is not installed")
		}
	}
	return runElevatedStep(context.Background(), "uninstall")
}

// Start starts the helper that carries the picture, when a phone begins to send it. It is one process per user
// session; a second one would end at once, but it is not started at all while one runs.
func (w *windowsCamera) Start() error {
	if helperRunning() {
		return nil
	}
	dll, registered := registeredDLL()
	if !registered {
		return nil // not installed: nothing to start
	}
	exe := filepath.Join(filepath.Dir(dll), helperName)
	if !fileExists(exe) {
		return nil
	}
	var args []string
	if w.configDir != "" {
		args = []string{"--config-dir", w.configDir}
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("couldn't start %s: %w", exe, err)
	}
	return cmd.Process.Release()
}

// registeredDLL is the file the camera's class id points to in the registry (the 64-bit view), and whether it is registered.
func registeredDLL() (string, bool) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Classes\CLSID\`+clsid+`\InprocServer32`,
		registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", false
	}
	defer k.Close()
	v, _, err := k.GetStringValue("")
	if err != nil || strings.TrimSpace(v) == "" {
		return "", false
	}
	return v, true
}

// helperRunning: the helper holds a named mutex for as long as it runs.
func helperRunning() bool {
	name, err := windows.UTF16PtrFromString(helperMutex)
	if err != nil {
		return false
	}
	h, err := windows.OpenMutex(windows.SYNCHRONIZE, false, name)
	if err != nil {
		return false
	}
	_ = windows.CloseHandle(h)
	return true
}

// installDir is "C:\Program Files\phoneset\camera": a place every process may read, where a program belongs.
func installDir() (string, error) {
	pf, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return "", err
	}
	return filepath.Join(pf, "phoneset", "camera"), nil
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
