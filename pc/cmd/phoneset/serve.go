package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mehmetraufoguz/phoneset/pc/internal/app"
	"github.com/mehmetraufoguz/phoneset/pc/internal/audio"
	"github.com/mehmetraufoguz/phoneset/pc/internal/control"
	"github.com/mehmetraufoguz/phoneset/pc/internal/discovery"
	"github.com/mehmetraufoguz/phoneset/pc/internal/pairing"
	"github.com/mehmetraufoguz/phoneset/pc/internal/proto"
	"github.com/mehmetraufoguz/phoneset/pc/internal/server"
	"github.com/mehmetraufoguz/phoneset/pc/internal/store"
	"github.com/mehmetraufoguz/phoneset/pc/internal/system"
	"github.com/mehmetraufoguz/phoneset/pc/internal/term"
	"github.com/mehmetraufoguz/phoneset/pc/internal/tray"
)

const logName = "phoneset.log"

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.Usage = func() { fmt.Print(usage) }
	port := fs.Int("port", proto.DefaultPort, "")
	name := fs.String("name", "", "")
	micDev := fs.String("mic-device", "", "")
	spkDev := fs.String("speaker-device", "", "")
	noMic := fs.Bool("no-mic", false, "")
	noSpk := fs.Bool("no-speaker", false, "")
	forcePair := fs.Bool("pair", false, "")
	dir := fs.String("config-dir", "", "")
	verbose := fs.Bool("verbose", false, "")
	background := fs.Bool("background", false, "")
	showTray := fs.Bool("tray", false, "")
	noTray := fs.Bool("no-tray", false, "")
	_ = fs.Parse(args)

	if *background {
		system.HideConsole()
	}
	st, err := openStore(*dir)
	if err != nil {
		return err
	}
	if *name != "" {
		if err := st.SetName(*name); err != nil {
			return err
		}
	}
	ident := st.Identity()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	var log *slog.Logger
	logPath := filepath.Join(st.Dir(), logName)
	if *background {
		f, err := openLog(logPath)
		if err != nil {
			return err
		}
		defer f.Close()
		log = slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: level}))
	} else {
		log = slog.New(term.NewHandler(os.Stdout, level))
	}

	// One agent per config directory. A second start is harmless when the system
	// launches it (login item), and an error when a person asks for it.
	if c, err := control.Connect(st.Dir()); err == nil {
		if *background {
			log.Info("phoneset is already running", "pid", c.Info.PID)
			return nil
		}
		return fmt.Errorf("phoneset is already running (PID %d). Run `phoneset status`, or `phoneset stop` first", c.Info.PID)
	}

	backend := audio.NewBackend()
	pm := pairing.NewManager()
	srv := server.New(server.Options{
		Port: *port, MicSelector: *micDev, SpeakerSelector: *spkDev, NoMic: *noMic, NoSpeaker: *noSpk,
	}, st, pm, backend, log)
	application := app.New(version, *port, st, pm, srv, backend, log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mode := "terminal"
	if *background {
		mode = "background"
	}
	ctl, err := control.Serve(ctx, st.Dir(), mode, application, stop)
	if err != nil {
		return err
	}
	defer control.Remove(st.Dir())
	log.Info("phoneset started", "version", version, "mode", mode, "pid", ctl.PID)

	if !*background {
		ui := &terminalUI{app: application, log: log}
		application.OnPaired = func(d store.Device) {
			fmt.Println(term.Green("  ✓ " + d.Name + " is paired. It can connect any time phoneset is running."))
		}
		application.OnPairingClosed = func(reason string) {
			log.Warn("pairing closed ("+reason+")", "hint", "type p to start again")
		}
		ui.banner(ident, *micDev, *spkDev, *noMic, *noSpk)
		if *forcePair || len(st.Devices()) == 0 {
			if err := ui.showPairing(); err != nil {
				return err
			}
		} else {
			fmt.Printf("  %d paired phone(s). Open the app and tap your PC to connect. Type %s to pair another phone.\n\n",
				len(st.Devices()), term.Bold("p"))
		}
		go ui.commands(ctx, stop)
	}

	work := func() error {
		err := application.Run(ctx)
		if !*background && ctx.Err() != nil {
			fmt.Println("\nbye")
		}
		return err
	}

	wantTray := *showTray || (*background && !*noTray)
	if !wantTray {
		return work()
	}
	if !tray.Available() {
		log.Warn("this build has no tray icon; running without it")
		return work()
	}
	exe, _ := os.Executable()
	pageURL := fmt.Sprintf("http://127.0.0.1:%d/pair?t=%s", ctl.Port, ctl.Token)
	return tray.Run(ctx, application, tray.Actions{
		OpenPairing: func() { _ = system.OpenURL(pageURL) },
		Disconnect:  func() { application.Disconnect() },
		Forget:      func(id string) { _, _ = application.Forget(id) },
		Quit:        stop,
		OpenLog:     func() { _ = system.OpenFile(logPath) },
		Autostart:   system.NewAutostart(),
		Exe:         exe,
	}, work)
}

// openLog opens the log for appending, starting a fresh file once it passes 2 MB.
func openLog(path string) (*os.File, error) {
	if fi, err := os.Stat(path); err == nil && fi.Size() > 2<<20 {
		_ = os.Rename(path, path+".1")
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}

// terminalUI is the interactive side of running in a terminal.
type terminalUI struct {
	app *app.App
	log *slog.Logger
}

func (u *terminalUI) banner(ident store.Identity, micSel, spkSel string, noMic, noSpk bool) {
	be := u.app.Backend
	fmt.Printf("%s %s  ·  %s\n", term.Bold("phoneset"), version, ident.Name)
	fmt.Printf("  audio backend      %s\n", be.Name())

	switch dev, err := be.ResolveMic(micSel); {
	case noMic:
		fmt.Printf("  microphone         %s\n", term.Dim("off (--no-mic)"))
	case err != nil:
		fmt.Printf("  microphone         %s\n", term.Yellow("no virtual cable found"))
		fmt.Println(term.Dim("                     " + be.MicHint()))
		fmt.Println(term.Dim("                     Or name a device with --mic-device."))
	default:
		fmt.Printf("  microphone         phone → %s\n", dev.Name)
	}
	switch dev, err := be.ResolveSpeaker(spkSel); {
	case noSpk:
		fmt.Printf("  headphones         %s\n", term.Dim("off (--no-speaker)"))
	case err != nil:
		fmt.Printf("  headphones         %s\n", term.Yellow("nothing to capture; name a device with --speaker-device"))
	default:
		fmt.Printf("  headphones         %s → phone\n", dev.Name)
	}
	addrs := discovery.Addrs()
	if len(addrs) == 0 {
		fmt.Printf("  address            %s\n", term.Yellow("no Wi-Fi/LAN address found - connect this PC to your network"))
	}
	for i, ad := range addrs {
		label := ""
		if i == 0 {
			label = "address"
		}
		fmt.Printf("  %-18s %s:%d\n", label, ad, u.app.Port)
	}
	fmt.Println()
}

func (u *terminalUI) showPairing() error {
	p, err := u.app.OpenPairing()
	if err != nil {
		return err
	}
	printPairing(p)
	return nil
}

// printPairing draws the QR code and the code for a pairing offer.
func printPairing(p app.PairingInfo) {
	qr, err := term.QR(p.Link)
	if err != nil {
		fmt.Println("  couldn't draw the QR code:", err)
	}
	fmt.Println(term.Bold("Pair a phone"))
	fmt.Println("  Open phoneset on the phone, tap +, then scan this QR code")
	fmt.Println("  or type the code. Phone and PC must be on the same Wi-Fi.")
	fmt.Println()
	fmt.Print(qr)
	fmt.Println()
	fmt.Printf("  Code  %s    %s\n\n", term.Bold(term.Cyan(p.Code)),
		term.Dim(fmt.Sprintf("valid for %d minutes", int(pairing.DefaultTTL.Minutes()))))
}

// commands reads one-letter commands from the terminal.
func (u *terminalUI) commands(ctx context.Context, quit func()) {
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		switch strings.ToLower(strings.TrimSpace(sc.Text())) {
		case "p", "pair":
			if err := u.showPairing(); err != nil {
				u.log.Error("couldn't start pairing", "err", err)
			}
		case "l", "list":
			printPhones(u.app.Store)
		case "a", "audio":
			_ = printAudio(u.app.Backend)
		case "d", "disconnect":
			if !u.app.Disconnect() {
				fmt.Println("  no phone is connected")
			}
		case "q", "quit", "exit":
			quit()
			return
		case "h", "?", "help":
			fmt.Println("  p  pair a phone    l  list paired phones    d  disconnect the phone    a  audio devices    q  quit")
		}
		if ctx.Err() != nil {
			return
		}
	}
}
