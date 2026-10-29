//go:build darwin

package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Label identifies the LaunchAgent. Tests and tooling can override it.
var Label = "com.rapp.phoneset"

// legacyLabel is what 0.1.0 used (a variable so tooling can point it elsewhere). A LaunchAgent under it still counts as "start at login", and
// enabling or disabling replaces it, so nobody ends up with two.
var legacyLabel = "dev.mehmetraufoguz.phoneset"

type macAutostart struct{}

// NewAutostart returns the macOS implementation: a per-user LaunchAgent that runs the
// service at login. launchd restarts it if it crashes, but not after a clean quit.
func NewAutostart() Autostart { return macAutostart{} }

func plistPathFor(label string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"), nil
}

func plistPath() (string, error) { return plistPathFor(Label) }

// removeLegacy unloads and deletes a LaunchAgent made under the old label, if there is one.
func removeLegacy() error {
	if legacyLabel == Label {
		return nil
	}
	p, err := plistPathFor(legacyLabel)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err != nil {
		return nil
	}
	_ = exec.Command("launchctl", "bootout", domain()+"/"+legacyLabel).Run()
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func domain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

func (macAutostart) Enabled() (bool, error) {
	p, err := plistPath()
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(p); err == nil {
		return true, nil
	}
	if legacyLabel != Label {
		if old, err := plistPathFor(legacyLabel); err == nil {
			if _, err := os.Stat(old); err == nil {
				return true, nil
			}
		}
	}
	return false, nil
}

// PlistFor renders the LaunchAgent for an executable and a log path.
func PlistFor(exe, logPath string) string {
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>serve</string>
		<string>--background</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, esc(Label), esc(exe), esc(logPath), esc(logPath))
}

func (macAutostart) Enable(exe string) error {
	p, err := plistPath()
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	logPath := filepath.Join(home, "Library", "Logs", "phoneset-launchd.log")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(PlistFor(exe, logPath)), 0o644); err != nil {
		return err
	}
	if err := removeLegacy(); err != nil {
		return err
	}
	// Replace a loaded copy, then load ours. bootout fails when nothing is loaded; that is fine.
	_ = exec.Command("launchctl", "bootout", domain()+"/"+Label).Run()
	if out, err := exec.Command("launchctl", "bootstrap", domain(), p).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (macAutostart) Disable() error {
	p, err := plistPath()
	if err != nil {
		return err
	}
	_ = exec.Command("launchctl", "bootout", domain()+"/"+Label).Run()
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return removeLegacy()
}
