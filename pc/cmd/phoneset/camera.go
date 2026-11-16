package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mehmetraufoguz/phoneset/pc/internal/camera"
	"github.com/mehmetraufoguz/phoneset/pc/internal/control"
	"github.com/mehmetraufoguz/phoneset/pc/internal/term"
)

// cmdCamera controls the virtual camera "phoneset Camera": its state, and putting it in place or taking it away.
func cmdCamera(args []string) error {
	sub := "status"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	if sub == "_elevated" { // the part of install and uninstall that runs with administrator rights (Windows)
		os.Exit(camera.ElevatedMain(args))
	}
	fs := flag.NewFlagSet("camera", flag.ExitOnError)
	dir := fs.String("config-dir", "", "")
	force := fs.Bool("force", false, "")
	_ = fs.Parse(args)

	mon := camera.NewMonitor(camera.Default())
	switch sub {
	case "status":
		st := mon.Refresh()
		if c, err := control.Connect(configDir(*dir)); err == nil { // the running program knows what the camera says
			if s, err := c.Camera(); err == nil {
				st = s
			}
		}
		fmt.Printf("%s %s\n", term.Bold("phoneset Camera"), term.Dim("(virtual camera)"))
		fmt.Printf("  state            %s\n", cameraText(st))
		if st.Phase == camera.Ready {
			fmt.Printf("  picture          %s\n", pictureText(st))
		}
		return nil
	case "install":
		return cameraRequest(mon, true, *force)
	case "uninstall", "remove":
		return cameraRequest(mon, false, false)
	default:
		return fmt.Errorf("unknown camera command %q (status, install or uninstall)", sub)
	}
}

// cameraRequest asks the system to install or remove the camera and follows it for a moment.
func cameraRequest(mon *camera.Monitor, install, force bool) error {
	b := mon.Backend()
	if install {
		exe, _ := os.Executable()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		say := func(s string) { fmt.Println(term.Dim("  " + s)) }
		if err := b.Install(ctx, camera.InstallOptions{ExeDir: filepath.Dir(exe), Force: force, Say: say}); err != nil {
			return err
		}
		fmt.Println("Asking the system to install the camera…")
	} else {
		if err := mon.Uninstall(); err != nil {
			return err
		}
		fmt.Println("Asking the system to remove the camera…")
	}
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(1500 * time.Millisecond)
		st := mon.Refresh()
		switch {
		case install && st.Phase == camera.Ready:
			fmt.Println(term.Green("Done."), pickText())
			return nil
		case install && st.Phase == camera.NeedsApproval:
			fmt.Println(term.Yellow("Waiting for you:"), st.Hint)
			fmt.Println(term.Dim("  Then `phoneset camera status` shows when it is ready."))
			return nil
		case !install && st.Phase == camera.NotInstalled:
			fmt.Println(term.Green("The camera is removed."))
			return nil
		}
	}
	fmt.Println(term.Yellow("Not done yet:"), cameraText(mon.Refresh()))
	return nil
}

// pickText tells where the camera shows up, by system.
func pickText() string {
	if runtime.GOOS == "windows" {
		return "Pick “phoneset Camera” as the camera in Chrome, Edge, OBS, Zoom or Teams (restart a browser that was open)."
	}
	return "Pick “phoneset Camera” as the camera in Zoom, Meet, FaceTime or QuickTime Player."
}

func cameraText(s camera.State) string {
	switch s.Phase {
	case camera.Ready:
		return term.Green("ready")
	case camera.Unsupported:
		return term.Yellow("not available") + " " + term.Dim(s.Hint)
	default:
		text := term.Yellow(string(s.Phase))
		if s.Hint != "" {
			text += " " + term.Dim(s.Hint)
		}
		return text
	}
}

func pictureText(s camera.State) string {
	switch {
	case !s.Running:
		return term.Dim("the carrier of the picture is not running (it starts when a phone turns its camera on)")
	case s.Line != "":
		return s.Line
	default:
		return term.Dim("running")
	}
}
