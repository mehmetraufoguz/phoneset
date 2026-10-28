package main

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/mehmetraufoguz/phoneset/pc/internal/audio"
	"github.com/mehmetraufoguz/phoneset/pc/internal/control"
	"github.com/mehmetraufoguz/phoneset/pc/internal/store"
	"github.com/mehmetraufoguz/phoneset/pc/internal/term"
)

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
	// A running agent keeps its own copy of the list, so it has to do the forgetting.
	if c, err := control.Connect(configDir(*dir)); err == nil {
		if err := c.Forget(fs.Arg(0)); err != nil {
			return fmt.Errorf("no paired phone matches %q", fs.Arg(0))
		}
		fmt.Println("  forgot", fs.Arg(0))
		return nil
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
