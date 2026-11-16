//go:build windows

package camera

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Registering a DirectShow camera writes to HKLM, and its files go to Program Files, so the install and the removal
// run as a second copy of this program started with administrator rights (the UAC window, `ShellExecute` with the verb
// "runas"): `phoneset camera _elevated install|uninstall ...`. The first copy waits for it and reads what it says.
//
// Why a copy of this program and not a script: the scripts in windows\ do the same with PowerShell and were checked on
// a PC; this one needs no PowerShell, no execution policy and no quoting of paths across two shells.

// Exit codes of the elevated step.
const (
	exitOK      = 0
	exitFailed  = 1
	exitPartial = 2 // the camera is unregistered, but a file of it is still in use
)

// partialError: the camera is gone from the registry (programs no longer list it), but a file is still in use.
type partialError struct{ msg string }

func (e *partialError) Error() string { return e.msg }

// runElevatedStep runs `phoneset camera _elevated <step> ...` with administrator rights and returns what it reports.
func runElevatedStep(ctx context.Context, step string, args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "phoneset-camera-step-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	result := filepath.Join(dir, "result.txt")
	full := append([]string{"camera", "_elevated", step}, args...)
	full = append(full, "--result", result)

	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute) // the time to answer the UAC window and register
	defer cancel()
	code, err := runElevated(ctx, exe, full)
	if err != nil {
		var errno syscall.Errno
		if errors.As(err, &errno) && errno == windows.ERROR_CANCELLED {
			return errors.New("administrator rights were not given (the UAC window was closed or answered No), so the camera was not changed")
		}
		return fmt.Errorf("couldn't ask for administrator rights: %w", err)
	}
	if code == exitOK {
		return nil
	}
	msg := ""
	if b, err := os.ReadFile(result); err == nil {
		msg = strings.TrimSpace(string(b))
	}
	if msg == "" {
		msg = fmt.Sprintf("the step that runs with administrator rights ended with code %d and said nothing", code)
	}
	return errors.New(msg)
}

// shellExecuteInfo is SHELLEXECUTEINFOW (shellapi.h); x/sys/windows does not have it.
type shellExecuteInfo struct {
	cbSize         uint32
	fMask          uint32
	hwnd           windows.Handle
	lpVerb         *uint16
	lpFile         *uint16
	lpParameters   *uint16
	lpDirectory    *uint16
	nShow          int32
	hInstApp       windows.Handle
	lpIDList       uintptr
	lpClass        *uint16
	hkeyClass      windows.Handle
	dwHotKey       uint32
	hIconOrMonitor windows.Handle
	hProcess       windows.Handle
}

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
)

var procShellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// runElevated starts exe with the verb "runas" (the UAC window), hidden, waits for it and returns its exit code. A
// cancelled UAC window comes back as ERROR_CANCELLED.
func runElevated(ctx context.Context, exe string, args []string) (uint32, error) {
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0, err
	}
	params, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(args))
	if err != nil {
		return 0, err
	}
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(exe))
	info := shellExecuteInfo{
		fMask: seeMaskNoCloseProcess | seeMaskNoAsync, lpVerb: verb, lpFile: file, lpParameters: params,
		lpDirectory: dir, nShow: windows.SW_HIDE,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))
	r, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		if callErr == nil || callErr == syscall.Errno(0) {
			callErr = errors.New("ShellExecuteEx failed")
		}
		return 0, callErr
	}
	if info.hProcess == 0 {
		return 0, errors.New("the elevated process was not started")
	}
	defer windows.CloseHandle(info.hProcess)
	for {
		ev, err := windows.WaitForSingleObject(info.hProcess, 500)
		if err != nil {
			return 0, err
		}
		if ev == windows.WAIT_OBJECT_0 {
			break
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.hProcess, &code); err != nil {
		return 0, err
	}
	return code, nil
}

// ElevatedMain is `phoneset camera _elevated <step> [--from dir] [--version tag] --result file`: the part that runs
// with administrator rights. It returns the exit code of the process.
func ElevatedMain(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "phoneset camera _elevated needs a step (install or uninstall)")
		return exitFailed
	}
	step := args[0]
	fs := flag.NewFlagSet("elevated", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	from := fs.String("from", "", "")
	version := fs.String("version", "", "")
	result := fs.String("result", "", "")
	if err := fs.Parse(args[1:]); err != nil {
		return failWith(*result, exitFailed, err)
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return failWith(*result, exitFailed, errors.New("this step needs administrator rights and was started without them"))
	}
	var err error
	switch step {
	case "install":
		err = applyInstall(*from, *version)
	case "uninstall":
		err = applyUninstall()
	default:
		err = fmt.Errorf("unknown step %q", step)
	}
	if err == nil {
		return exitOK
	}
	code := exitFailed
	var p *partialError
	if errors.As(err, &p) {
		code = exitPartial
	}
	return failWith(*result, code, err)
}

func failWith(resultFile string, code int, err error) int {
	if resultFile != "" {
		_ = os.WriteFile(resultFile, []byte(err.Error()+"\n"), 0o600)
	}
	fmt.Fprintln(os.Stderr, err)
	return code
}

// applyInstall copies the camera from `from` to Program Files and registers the DLL. A file that a program still has
// loaded (a browser keeps the DLL after it used the camera) cannot be overwritten or deleted, but it can be renamed:
// the old copy is moved aside and the new one takes its place, so an update does not wait for the browser to close.
func applyInstall(from, version string) error {
	if from == "" {
		return errors.New("no folder to install from")
	}
	dest, err := installDir()
	if err != nil {
		return err
	}
	for _, f := range []string{dllName, helperName} {
		if !fileExists(filepath.Join(from, f)) {
			return fmt.Errorf("%s is not in %s", f, from)
		}
	}
	quitHelper(dest)
	// An old registration is taken away first (its file may be somewhere else); a failure here does not matter, the
	// registration below replaces it.
	if old, ok := registeredDLL(); ok && fileExists(old) {
		_ = regsvr32(true, old)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("couldn't make %s: %w", dest, err)
	}
	sweep(dest)
	files := []string{dllName, helperName}
	if extras, _ := filepath.Glob(filepath.Join(from, "*.txt")); len(extras) > 0 { // the licence of the DLL
		for _, e := range extras {
			files = append(files, filepath.Base(e))
		}
	}
	for _, f := range files {
		if err := replaceFile(filepath.Join(from, f), filepath.Join(dest, f)); err != nil {
			return err
		}
	}
	dll := filepath.Join(dest, dllName)
	if err := regsvr32(false, dll); err != nil {
		return err
	}
	if reg, ok := registeredDLL(); !ok || !strings.EqualFold(reg, dll) {
		return fmt.Errorf("regsvr32 said it was done, but the registry does not point to %s (it says %q)", dll, reg)
	}
	_ = os.WriteFile(filepath.Join(dest, "version.txt"), []byte(version+"\n"), 0o644)
	return nil
}

// applyUninstall takes the registration away and removes the folder. A program that still has the DLL loaded keeps
// the folder from being deleted: the camera is unregistered then (programs stop listing it), and the error says
// which program holds the file.
func applyUninstall() error {
	dest, err := installDir()
	if err != nil {
		return err
	}
	quitHelper(dest)
	dll := filepath.Join(dest, dllName)
	reg, registered := registeredDLL()
	if registered {
		dll = reg // what is registered is what to unregister
	}
	// Only a camera that is registered is unregistered: after an uninstall that could not delete the file (a browser held
	// it) the registration is gone already, and the second run only has the folder left to remove.
	switch {
	case registered && fileExists(dll):
		if err := regsvr32(true, dll); err != nil {
			return err
		}
	case registered:
		return fmt.Errorf("the camera is registered to %s, but that file is gone, so it cannot be unregistered cleanly. "+
			"Run `phoneset camera install --force`, then uninstall again", dll)
	}
	if err := os.RemoveAll(dest); err != nil {
		msg := "The camera is unregistered, but " + dest + " could not be removed"
		if h := holdersOf(dllName); len(h) > 0 {
			msg += ": " + strings.Join(h, ", ") + " still has " + dllName + " loaded (a browser keeps a camera it used). " +
				"Close it, then run `phoneset camera uninstall` again to remove the file"
		} else {
			msg += ": " + err.Error()
		}
		return &partialError{msg: msg + "."}
	}
	_ = os.Remove(filepath.Dir(dest)) // the "phoneset" folder goes only when nothing else is in it
	return nil
}

// quitHelper asks a running helper to quit (it signals the event `--quit` knows and waits for the helper to end).
func quitHelper(dir string) {
	if !helperRunning() {
		return
	}
	exe := filepath.Join(dir, helperName)
	if !fileExists(exe) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, exe, "--quit").Run()
}

// regsvr32 registers or unregisters a DLL. regsvr32 is a Windows (not console) program; exec waits for it all the same.
func regsvr32(unregister bool, dll string) error {
	sys, err := windows.GetSystemDirectory()
	if err != nil || sys == "" {
		sys = `C:\Windows\System32`
	}
	args := []string{"/s"}
	if unregister {
		args = append(args, "/u")
	}
	args = append(args, dll)
	out, err := exec.Command(filepath.Join(sys, "regsvr32.exe"), args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("regsvr32 %s failed: %v %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// replaceFile puts a copy of src at dst. The old dst, if there is one, is renamed first (that works for a loaded DLL,
// where deleting does not) and deleted if it can be; what cannot is swept at the next install.
func replaceFile(src, dst string) error {
	tmp := dst + ".new"
	if err := copyFile(src, tmp); err != nil {
		return fmt.Errorf("couldn't copy %s: %w", filepath.Base(src), err)
	}
	var old string
	if fileExists(dst) {
		old = fmt.Sprintf("%s.old-%d", dst, time.Now().UnixNano())
		if err := os.Rename(dst, old); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("couldn't move the old %s out of the way: %w", filepath.Base(dst), err)
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("couldn't put %s in place: %w", filepath.Base(dst), err)
	}
	if old != "" {
		_ = os.Remove(old)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// sweep deletes what an earlier install could not: copies that were moved aside while a program had them loaded.
func sweep(dir string) {
	for _, pattern := range []string{"*.old-*", "*.new"} {
		if names, err := filepath.Glob(filepath.Join(dir, pattern)); err == nil {
			for _, n := range names {
				_ = os.Remove(n)
			}
		}
	}
}

// holdersOf names the programs that have a module of this name loaded (tasklist /m), best effort.
func holdersOf(module string) []string {
	out, err := exec.Command("tasklist.exe", "/m", module, "/fo", "csv", "/nh").Output()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, `"`) {
			continue // "INFO: No tasks are running which match the specified criteria."
		}
		if name := strings.Split(strings.TrimPrefix(line, `"`), `"`)[0]; name != "" {
			seen[name] = true
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
