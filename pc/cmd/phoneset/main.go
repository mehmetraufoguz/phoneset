// Command phoneset turns a phone into this PC's microphone and headphones over Wi-Fi.
package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mehmetraufoguz/phoneset/pc/internal/audio"
	"github.com/mehmetraufoguz/phoneset/pc/internal/discovery"
	"github.com/mehmetraufoguz/phoneset/pc/internal/pairing"
	"github.com/mehmetraufoguz/phoneset/pc/internal/proto"
	"github.com/mehmetraufoguz/phoneset/pc/internal/server"
	"github.com/mehmetraufoguz/phoneset/pc/internal/store"
	"github.com/mehmetraufoguz/phoneset/pc/internal/term"
)

var version = "dev"

const usage = `phoneset - use your phone as this PC's microphone and headphones over Wi-Fi

Usage:
  phoneset [serve] [flags]   start the service and show the pairing QR code
  phoneset audio             list audio devices and how phoneset would use them
  phoneset test              check the headphone source (level meter) or --mic (tone)
  phoneset phones            list paired phones
  phoneset forget <name|id|all>
  phoneset version

Flags for serve:
  --port N              TCP port (default 47831)
  --name NAME           name shown on the phone (default: this PC's name)
  --mic-device TEXT     device the phone's microphone is played into, a virtual
                        cable (default: auto-detect VB-CABLE or BlackHole)
  --speaker-device TEXT device whose audio is sent to the phone (default: the system
                        default playback on Windows; system audio on macOS)
  --no-mic, --no-speaker
  --pair                open pairing even when phones are already paired
  --config-dir DIR      where pairings are stored
  --verbose
`

func main() {
	term.Init()
	cmd, args := "serve", os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = cmdServe(args)
	case "audio":
		err = cmdAudio()
	case "test":
		err = cmdTest(os.Args[2:])
	case "phones":
		err = cmdPhones(args)
	case "forget":
		err = cmdForget(args)
	case "version":
		fmt.Println("phoneset", version)
	case "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, term.Red("error:"), err)
		os.Exit(1)
	}
}

func openStore(dir string) (*store.Store, error) {
	if dir == "" {
		var err error
		if dir, err = store.DefaultDir(); err != nil {
			return nil, err
		}
	}
	return store.Open(dir)
}

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
	_ = fs.Parse(args)

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
	log := slog.New(term.NewHandler(os.Stdout, level))

	backend := audio.NewBackend()
	pm := pairing.NewManager()
	srv := server.New(server.Options{
		Port: *port, MicSelector: *micDev, SpeakerSelector: *spkDev, NoMic: *noMic, NoSpeaker: *noSpk,
	}, st, pm, backend, log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := &app{st: st, pm: pm, backend: backend, port: *port, opt: srv, log: log}
	pm.OnWithdrawn = func(reason string) {
		log.Warn("pairing closed ("+reason+")", "hint", "type p to start again")
	}
	srv.OnPaired = func(d store.Device) {
		log.Info("paired", "phone", d.Name, "platform", d.Platform)
		fmt.Println(term.Green("  ✓ " + d.Name + " is paired. It can connect any time phoneset is running."))
	}

	app.banner(ident, *micDev, *spkDev, *noMic, *noSpk)

	if stopAd, err := discovery.Advertise(ident.ID, ident.Name, *port); err != nil {
		log.Warn("mDNS is unavailable; pair with the QR code or the PC's address", "err", err)
	} else {
		defer stopAd()
	}

	if *forcePair || len(st.Devices()) == 0 {
		if err := app.showPairing(); err != nil {
			return err
		}
	} else {
		fmt.Printf("  %d paired phone(s). Open the app and tap your PC to connect. Type %s to pair another phone.\n\n",
			len(st.Devices()), term.Bold("p"))
	}

	go app.commands(ctx, stop)

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ctx) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		fmt.Println("\nbye")
		<-errc
		return nil
	}
}

type app struct {
	st      *store.Store
	pm      *pairing.Manager
	backend audio.Backend
	port    int
	opt     *server.Server
	log     *slog.Logger
}

func (a *app) banner(ident store.Identity, micSel, spkSel string, noMic, noSpk bool) {
	fmt.Printf("%s %s  ·  %s\n", term.Bold("phoneset"), version, ident.Name)
	fmt.Printf("  audio backend      %s\n", a.backend.Name())

	switch dev, err := a.backend.ResolveMic(micSel); {
	case noMic:
		fmt.Printf("  microphone         %s\n", term.Dim("off (--no-mic)"))
	case err != nil:
		fmt.Printf("  microphone         %s\n", term.Yellow("no virtual cable found"))
		fmt.Println(term.Dim("                     " + a.backend.MicHint()))
		fmt.Println(term.Dim("                     Or name a device with --mic-device."))
	default:
		fmt.Printf("  microphone         phone → %s\n", dev.Name)
	}
	switch dev, err := a.backend.ResolveSpeaker(spkSel); {
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
		fmt.Printf("  %-18s %s:%d\n", label, ad, a.port)
	}
	fmt.Println()
}

func (a *app) showPairing() error {
	offer, err := a.pm.Open()
	if err != nil {
		return err
	}
	ident := a.st.Identity()
	addrs := discovery.Addrs()
	link := pairURL(ident, addrs, a.port, offer.Secret)
	qr, err := term.QR(link)
	if err != nil {
		return err
	}
	a.log.Debug("pairing link", "url", link)
	fmt.Println(term.Bold("Pair a phone"))
	fmt.Println("  Open phoneset on the phone, tap \"Pair a PC\", then scan this QR code")
	fmt.Println("  or type the code. Phone and PC must be on the same Wi-Fi.")
	fmt.Println()
	fmt.Print(qr)
	fmt.Println()
	fmt.Printf("  Code  %s    %s\n\n", term.Bold(term.Cyan(offer.FormatCode())),
		term.Dim(fmt.Sprintf("valid for %d minutes", int(pairing.DefaultTTL.Minutes()))))
	return nil
}

// commands reads one-letter commands from the terminal.
func (a *app) commands(ctx context.Context, quit func()) {
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		switch strings.ToLower(strings.TrimSpace(sc.Text())) {
		case "p", "pair":
			if err := a.showPairing(); err != nil {
				a.log.Error("couldn't start pairing", "err", err)
			}
		case "l", "list":
			printPhones(a.st)
		case "a", "audio":
			_ = printAudio(a.backend)
		case "q", "quit", "exit":
			quit()
			return
		case "h", "?", "help":
			fmt.Println("  p  pair a phone    l  list paired phones    a  audio devices    q  quit")
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func pairURL(ident store.Identity, addrs []string, port int, secret []byte) string {
	if len(addrs) > 3 {
		addrs = addrs[:3]
	}
	return fmt.Sprintf("phoneset://pair?v=%d&id=%s&n=%s&h=%s&p=%d&k=%s",
		proto.Version, ident.ID, url.QueryEscape(ident.Name), strings.Join(addrs, ","), port,
		base64.RawURLEncoding.EncodeToString(secret))
}

func cmdAudio() error { return printAudio(audio.NewBackend()) }

func printAudio(be audio.Backend) error {
	devs, err := be.Devices()
	if err != nil {
		return err
	}
	fmt.Println(term.Bold("Audio devices") + term.Dim("  ("+be.Name()+")"))
	if len(devs) == 0 {
		fmt.Println("  none found")
	}

	micDev, micErr := be.ResolveMic("")
	spkDev, spkErr := be.ResolveSpeaker("")
	section := func(title string, has func(audio.Device) bool, auto audio.Device, autoErr error) {
		fmt.Println("\n  " + title)
		for _, d := range devs {
			if !has(d) {
				continue
			}
			var tags []string
			if autoErr == nil && d.ID == auto.ID {
				tags = append(tags, "chosen automatically")
			}
			if d.Virtual {
				tags = append(tags, "virtual")
			}
			mark := " "
			if autoErr == nil && d.ID == auto.ID {
				mark = "*"
			}
			fmt.Printf("    %s %s  %s\n", mark, d.Name, term.Dim(strings.Join(tags, ", ")))
		}
	}
	section("The phone's microphone is played into (apps record from the other end):",
		func(d audio.Device) bool { return d.Mic }, micDev, micErr)
	section("Sent to the phone's headphones:",
		func(d audio.Device) bool { return d.Speaker }, spkDev, spkErr)
	fmt.Println(term.Dim("\n  Use --mic-device / --speaker-device with part of a name to override."))
	return nil
}

func cmdPhones(args []string) error {
	fs := flag.NewFlagSet("phones", flag.ExitOnError)
	dir := fs.String("config-dir", "", "")
	_ = fs.Parse(args)
	st, err := openStore(*dir)
	if err != nil {
		return err
	}
	printPhones(st)
	return nil
}

func printPhones(st *store.Store) {
	devs := st.Devices()
	if len(devs) == 0 {
		fmt.Println("  no paired phones")
		return
	}
	for _, d := range devs {
		seen := "never connected"
		if !d.LastSeen.IsZero() {
			seen = "last seen " + d.LastSeen.Local().Format(time.DateTime)
		}
		fmt.Printf("  %s  %s  %s\n", d.Name, term.Dim(d.Platform), term.Dim(seen))
	}
}

func cmdForget(args []string) error {
	fs := flag.NewFlagSet("forget", flag.ExitOnError)
	dir := fs.String("config-dir", "", "")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: phoneset forget <name|id|all>")
	}
	st, err := openStore(*dir)
	if err != nil {
		return err
	}
	removed, err := st.Remove(fs.Arg(0))
	if err != nil {
		return err
	}
	for _, d := range removed {
		fmt.Println("  forgot", d.Name)
	}
	return nil
}
