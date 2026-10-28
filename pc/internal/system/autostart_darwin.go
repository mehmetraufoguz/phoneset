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
var Label = "dev.mehmetraufoguz.phoneset"

type macAutostart struct{}

// NewAutostart returns the macOS implementation: a per-user LaunchAgent that runs the
// service at login. launchd restarts it if it crashes, but not after a clean quit.
func NewAutostart() Autostart { return macAutostart{} }

func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist"), nil
}

func domain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

func (macAutostart) Enabled() (bool, error) {
	p, err := plistPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	return err == nil, nil
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
	return nil
}
