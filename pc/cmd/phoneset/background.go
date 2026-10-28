package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mehmetraufoguz/phoneset/pc/internal/app"
	"github.com/mehmetraufoguz/phoneset/pc/internal/control"
	"github.com/mehmetraufoguz/phoneset/pc/internal/system"
	"github.com/mehmetraufoguz/phoneset/pc/internal/term"
)

var errNotRunning = fmt.Errorf("phoneset isn't running. Start it with `phoneset daemon` (background) or `phoneset` (terminal)")

// cmdDaemon starts the service in the background and returns.
func cmdDaemon(args []string) error {
	quiet := hasFlag(args, "--quiet")
	childArgs := dropFlags(args, "--quiet", "--no-open")
	dir := configDir(flagValue(args, "--config-dir"))

	if c, err := control.Connect(dir); err == nil {
		if !quiet {
			fmt.Printf("phoneset is already running in the background (PID %d)\n", c.Info.PID)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	c, err := startBackground(exe, dir, childArgs)
	if err != nil {
		return err
	}
	if quiet {
		return nil
	}
	fmt.Printf("phoneset is running in the background (PID %d).\n", c.Info.PID)
	fmt.Println(term.Dim("  `phoneset status` shows what it is doing, `phoneset stop` ends it."))
	if snap, err := c.Status(); err == nil && len(snap.Phones) == 0 && !hasFlag(args, "--no-open") {
		fmt.Println("  No phone is paired yet: opening the pairing page in your browser.")
		_ = system.OpenURL(c.PairPageURL())
	}
	return nil
}

// startBackground launches `exe serve --background` detached and waits until its control API answers.
func startBackground(exe, dir string, serveArgs []string) (*control.Client, error) {
	cmd := exec.Command(exe, append([]string{"serve", "--background"}, serveArgs...)...)
	system.Detach(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	_ = cmd.Process.Release()

	for i := 0; i < 80; i++ {
		time.Sleep(150 * time.Millisecond)
		if c, err := control.Connect(dir); err == nil {
			return c, nil
		}
	}
	return nil, fmt.Errorf("the background service didn't start; see %s", filepath.Join(dir, logName))
}

// stopAndWait asks the running service to stop and waits until it has.
func stopAndWait(c *control.Client, dir string) error {
	if err := c.Stop(); err != nil {
		return err
	}
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if _, err := control.Connect(dir); err != nil {
			return nil
		}
	}
	return fmt.Errorf("asked phoneset to stop, but it is still running (PID %d)", c.Info.PID)
}

func cmdStop(args []string) error {
	dir := configDir(flagValue(args, "--config-dir"))
	c, err := control.Connect(dir)
	if err != nil {
		fmt.Println("phoneset isn't running.")
		return nil
	}
	if err := stopAndWait(c, dir); err != nil {
		return err
	}
	fmt.Println("phoneset stopped.")
	return nil
}

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	dir := fs.String("config-dir", "", "")
	asJSON := fs.Bool("json", false, "")
	_ = fs.Parse(args)

	c, err := control.Connect(configDir(*dir))
	if err != nil {
		if *asJSON {
			fmt.Println(`{"running":false}`)
			return nil
		}
		fmt.Println(term.Yellow("phoneset isn't running."))
		printAutostart()
		return nil
	}
	snap, err := c.Status()
	if err != nil {
		return err
	}
	if *asJSON {
		out, _ := json.MarshalIndent(map[string]any{
			"running": true, "mode": c.Info.Mode, "started": c.Info.Started, "status": snap,
		}, "", "  ")
		fmt.Println(string(out))
		return nil
	}
	printStatus(c, snap)
	return nil
}

func printStatus(c *control.Client, s app.Snapshot) {
	where := "in a terminal"
	if c.Info.Mode == "background" {
		where = "in the background"
	}
	fmt.Printf("%s %s %s  %s\n", term.Bold("phoneset"), buildLabel(s.Version, s.Commit), term.Green("running "+where),
		term.Dim(fmt.Sprintf("(PID %d, up %s)", s.PID, since(c.Info.Started))))
	fmt.Printf("  name             %s\n", s.Name)
	for i, a := range s.Addrs {
		label := ""
		if i == 0 {
			label = "address"
		}
		fmt.Printf("  %-16s %s:%d\n", label, a, s.Port)
	}
	if s.Phone == nil {
		fmt.Printf("  phone            %s\n", term.Dim("waiting for a phone"))
	} else {
		fmt.Printf("  phone            %s %s\n", term.Bold(s.Phone.Name),
			term.Green("connected")+term.Dim(fmt.Sprintf(" %s ago from %s", since(s.Phone.Since), s.Phone.From)))
		fmt.Printf("    microphone     %s\n", streamText(s.Mic.On, "→ "+s.Mic.Device))
		fmt.Printf("    headphones     %s\n", streamText(s.Speaker.On, "← "+s.Speaker.Device))
	}
	var names []string
	for _, p := range s.Phones {
		names = append(names, p.Name)
	}
	if len(names) == 0 {
		names = []string{"none"}
	}
	fmt.Printf("  paired phones    %s\n", strings.Join(names, ", "))
	if s.Pairing != nil {
		left := time.Until(s.Pairing.ExpiresAt).Round(time.Second)
		fmt.Printf("  pairing          open, code %s %s\n", term.Bold(s.Pairing.Code), term.Dim("("+left.String()+" left)"))
	}
	printAutostart()
}

func streamText(on bool, detail string) string {
	if !on {
		return term.Dim("off")
	}
	return "on " + term.Dim(detail)
}

func since(t time.Time) string {
	d := time.Since(t).Round(time.Second)
	if d < time.Minute {
		return d.String()
	}
	return d.Round(time.Minute).String()
}

func cmdPair(args []string) error {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	dir := fs.String("config-dir", "", "")
	browser := fs.Bool("browser", false, "")
	_ = fs.Parse(args)

	c, err := control.Connect(configDir(*dir))
	if err != nil {
		return errNotRunning
	}
	if *browser {
		return system.OpenURL(c.PairPageURL())
	}
	p, err := c.Pair()
	if err != nil {
		return err
	}
	printPairing(p)
	return nil
}

func cmdLogs(args []string) error {
	path := filepath.Join(configDir(flagValue(args, "--config-dir")), logName)
	fmt.Println(term.Dim(path))
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("  no log yet: it is written when phoneset runs in the background.")
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > 40 {
		lines = lines[len(lines)-40:]
	}
	fmt.Println(strings.Join(lines, "\n"))
	return nil
}

// cmdAutostart turns "start in the background at login" on or off.
func cmdAutostart(args []string) error {
	action := "status"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action = args[0]
	}
	as := system.NewAutostart()
	switch action {
	case "enable":
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		if strings.Contains(exe, "go-build") || strings.HasPrefix(exe, os.TempDir()) {
			return fmt.Errorf("this copy of phoneset is in a temporary folder (%s); install it somewhere permanent first", exe)
		}
		if err := as.Enable(exe); err != nil {
			return err
		}
		fmt.Println(term.Green("phoneset will start in the background every time you log in."))
		fmt.Println(term.Dim("  Start it now with `phoneset daemon`; turn this off with `phoneset autostart disable`."))
	case "disable":
		if err := as.Disable(); err != nil {
			return err
		}
		fmt.Println("phoneset no longer starts at login.")
	case "status":
		printAutostart()
	default:
		return fmt.Errorf("usage: phoneset autostart enable|disable|status")
	}
	return nil
}

func printAutostart() {
	on, err := system.NewAutostart().Enabled()
	switch {
	case err != nil:
		fmt.Printf("  start at login   %s\n", term.Dim("not available here"))
	case on:
		fmt.Printf("  start at login   %s\n", term.Green("on"))
	default:
		fmt.Printf("  start at login   %s\n", term.Dim("off (phoneset autostart enable)"))
	}
}

// ---------------------------------------------------------------- argument helpers

func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}

func dropFlags(args []string, names ...string) []string {
	var out []string
outer:
	for _, a := range args {
		for _, n := range names {
			if a == n {
				continue outer
			}
		}
		out = append(out, a)
	}
	return out
}

// flagValue reads "--name value" or "--name=value" without a full parse.
func flagValue(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, name+"=") {
			return strings.TrimPrefix(a, name+"=")
		}
	}
	return ""
}
