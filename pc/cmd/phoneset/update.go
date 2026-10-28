package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mehmetraufoguz/phoneset/pc/internal/control"
	"github.com/mehmetraufoguz/phoneset/pc/internal/system"
	"github.com/mehmetraufoguz/phoneset/pc/internal/term"
	"github.com/mehmetraufoguz/phoneset/pc/internal/update"
)

// commit is the full commit hash of the build, written by the release workflow (-X main.commit=...).
var commit = ""

func currentBuild() update.Build { return update.CurrentBuild(version, commit) }

func buildLabel(v, c string) string {
	if len(c) > 7 {
		c = c[:7]
	}
	if c == "" {
		return v
	}
	return v + " (" + c + ")"
}

// cmdUpdate looks for a newer release and installs it. A background service is restarted on the new build.
func cmdUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	dir := fs.String("config-dir", "", "")
	checkOnly := fs.Bool("check", false, "")
	asJSON := fs.Bool("json", false, "")
	force := fs.Bool("force", false, "")
	_ = fs.Parse(args)

	cur := currentBuild()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cl := update.NewClient()
	res, err := cl.Check(ctx, cur)
	if err != nil {
		if *asJSON {
			out, _ := json.MarshalIndent(map[string]any{"current": cur, "status": update.Unknown, "error": err.Error()}, "", "  ")
			fmt.Println(string(out))
			return nil
		}
		return fmt.Errorf("couldn't check for updates: %w", err)
	}
	if *asJSON {
		out, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(out))
		if *checkOnly {
			return nil
		}
	} else {
		printUpdate(res)
	}

	switch {
	case *checkOnly:
		return nil
	case res.Status == update.Available:
	case *force && res.Latest != nil:
		fmt.Println(term.Dim("  --force: installing it anyway."))
	default:
		return nil
	}

	exe, err := installedExe()
	if err != nil {
		return err
	}
	if err := cl.Install(ctx, res.Latest, exe, func(s string) { fmt.Println(term.Dim("  " + s)) }); err != nil {
		return err
	}
	fmt.Println(term.Green("Updated to " + res.Latest.Tag + " (" + res.Latest.Short() + ")."))

	// Bring a background service over to the new build.
	d := configDir(*dir)
	c, err := control.Connect(d)
	if err != nil {
		return nil
	}
	if c.Info.Mode != "background" {
		fmt.Println("  phoneset is running in a terminal: stop it with Ctrl+C and start it again to use the new build.")
		return nil
	}
	fmt.Println("  Restarting the background service...")
	if err := stopAndWait(c, d); err != nil {
		return err
	}
	if _, err := startBackground(exe, d, c.Info.Args); err != nil {
		return err
	}
	fmt.Println("  Running again.")
	return nil
}

func printUpdate(r update.Result) {
	fmt.Printf("%s  %s\n", term.Bold("this build "), buildLabel(r.Current.Version, r.Current.Commit)+modifiedNote(r.Current))
	if r.Latest != nil {
		fmt.Printf("%s  %s (%s)\n", term.Bold("latest     "), r.Latest.Tag, r.Latest.Short())
	}
	switch r.Status {
	case update.UpToDate:
		fmt.Println(term.Green("You are up to date."))
	case update.Available:
		n := ""
		if r.BehindBy > 0 {
			n = fmt.Sprintf(" (%d commit(s) behind)", r.BehindBy)
		}
		fmt.Println(term.Yellow("A newer build is available" + n + "."))
	case update.Ahead:
		fmt.Println(term.Dim("This build is newer than the latest release (a development build). Nothing to update."))
	default:
		note := r.Note
		if note == "" {
			note = "couldn't compare"
		}
		fmt.Println(term.Dim("Can't tell whether there is an update: " + note + ". `phoneset update --force` installs the latest release anyway."))
	}
}

func modifiedNote(b update.Build) string {
	if b.Modified {
		return " " + term.Dim("with local changes")
	}
	return ""
}

// installedExe is the path of the running executable, as long as it is somewhere an update should replace.
func installedExe() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if strings.Contains(exe, "go-build") || strings.HasPrefix(exe, os.TempDir()) {
		return "", fmt.Errorf("this copy of phoneset is in a temporary folder (%s); update the installed one instead", exe)
	}
	return exe, nil
}

// cmdRelaunch is what a service that has just replaced its own executable starts before it quits:
// it waits for that service to be gone and brings phoneset back up in the background.
// Usage: phoneset relaunch -- <serve flags>
func cmdRelaunch(args []string) error {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir := configDir(flagValue(args, "--config-dir"))
	for i := 0; i < 100; i++ { // the old service has 20 s to finish
		if _, err := control.Connect(dir); err != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	_, err = startBackground(exe, dir, args)
	return err
}

// relaunchAfterQuit starts a detached `phoneset relaunch` that will bring the service back once this one has quit.
func relaunchAfterQuit(exe string, serveArgs []string) error {
	cmd := exec.Command(exe, append([]string{"relaunch", "--"}, serveArgs...)...)
	system.Detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
