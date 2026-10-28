package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mehmetraufoguz/phoneset/pc/internal/audio"
	"github.com/mehmetraufoguz/phoneset/pc/internal/term"
)

// cmdTest lets you check the audio setup without a phone.
//
//	phoneset test          meter for what would be sent to the phone's headphones
//	phoneset test --mic    a quiet tone into the device that carries the phone's microphone
func cmdTest(args []string) error {
	fs := flag.NewFlagSet("test", flag.ExitOnError)
	mic := fs.Bool("mic", false, "")
	micDev := fs.String("mic-device", "", "")
	spkDev := fs.String("speaker-device", "", "")
	seconds := fs.Int("seconds", 8, "")
	_ = fs.Parse(args)

	be := audio.NewBackend()
	ctx := make(chan os.Signal, 1)
	signal.Notify(ctx, os.Interrupt, syscall.SIGTERM)
	deadline := time.After(time.Duration(*seconds) * time.Second)

	if *mic {
		return testMic(be, *micDev, deadline, ctx)
	}
	return testSpeaker(be, *spkDev, deadline, ctx)
}

func testSpeaker(be audio.Backend, selector string, deadline <-chan time.Time, stop <-chan os.Signal) error {
	dev, err := be.ResolveSpeaker(selector)
	if err != nil {
		return fmt.Errorf("no device to capture; name one with --speaker-device (see `phoneset audio`)")
	}
	var window, overall atomic.Int64
	failed := make(chan error, 1)
	src, err := be.OpenSpeaker(dev, func(pcm []byte) {
		peak := int64(0)
		for i := 0; i+1 < len(pcm); i += 2 {
			v := int64(int16(uint16(pcm[i]) | uint16(pcm[i+1])<<8))
			if v < 0 {
				v = -v
			}
			peak = max(peak, v)
		}
		for {
			cur := window.Load()
			if peak <= cur || window.CompareAndSwap(cur, peak) {
				break
			}
		}
		for {
			cur := overall.Load()
			if peak <= cur || overall.CompareAndSwap(cur, peak) {
				break
			}
		}
	}, func(err error) { failed <- err })
	if err != nil {
		return err
	}
	defer src.Close()

	fmt.Printf("Listening to %s. Play something on this PC (Ctrl+C to stop).\n\n", term.Bold(dev.Name))
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
listen:
	for {
		select {
		case <-tick.C:
			fmt.Printf("\r  %s", meter(window.Swap(0)))
		case err := <-failed:
			fmt.Println()
			return err
		case <-stop:
			break listen
		case <-deadline:
			break listen
		}
	}
	fmt.Println()
	if overall.Load() == 0 {
		fmt.Println("\n" + term.Yellow("Nothing was captured."))
		if hint := be.CaptureHint(); hint != "" {
			fmt.Println(hint)
		}
		return nil
	}
	fmt.Println("\n" + term.Green("Captured audio. The phone's headphones will get this."))
	return nil
}

func testMic(be audio.Backend, selector string, deadline <-chan time.Time, stop <-chan os.Signal) error {
	dev, err := be.ResolveMic(selector)
	if err != nil {
		return fmt.Errorf("no virtual cable found. %s", be.MicHint())
	}
	sink, err := be.OpenMic(dev)
	if err != nil {
		return err
	}
	defer sink.Close()

	fmt.Printf("Playing a quiet 440 Hz tone into %s.\n", term.Bold(dev.Name))
	fmt.Println("In an app, choose the other end of that cable as the microphone and watch its level (Ctrl+C to stop).")

	const frame = audio.PhoneFrameSamples
	buf := make([]byte, frame*2)
	phase := 0.0
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			for i := 0; i < frame; i++ {
				v := int16(math.Sin(phase) * 0.1 * 32767) // -20 dBFS
				phase += 2 * math.Pi * 440 / 48000
				buf[i*2], buf[i*2+1] = byte(v), byte(uint16(v)>>8)
			}
			sink.Write(buf)
		case <-stop:
			return nil
		case <-deadline:
			return nil
		}
	}
}

// meter draws a level bar from -60 dBFS to 0 dBFS.
func meter(peak int64) string {
	const width = 32
	db := -100.0
	if peak > 0 {
		db = 20 * math.Log10(float64(peak)/32768)
	}
	filled := int(math.Max(0, math.Min(width, (db+60)/60*width)))
	label := "silent"
	if peak > 0 {
		label = fmt.Sprintf("%5.1f dB", db)
	}
	return fmt.Sprintf("%s%s  %-8s", strings.Repeat("█", filled), strings.Repeat("░", width-filled), label)
}
