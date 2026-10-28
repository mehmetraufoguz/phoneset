//go:build darwin && cgo

package audio

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework CoreAudio -framework AudioToolbox -framework CoreFoundation -framework Foundation
#include <stdlib.h>
#include "ca_darwin.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// CoreAudio backend.
//
//   - Phone microphone -> an output device, normally a virtual one such as
//     BlackHole 2ch. Apps then record from the same device as an input. macOS has
//     no way to make a microphone without such a driver.
//   - PC audio -> phone: by default the system-audio tap (macOS 14.2+), which needs
//     no driver but asks once for "System Audio Recording" permission. Older systems,
//     or --speaker-device, capture a virtual input device instead (for example
//     BlackHole 16ch behind a Multi-Output device).
//
// The audio threads live in ca_darwin.m. Go only polls a ring buffer, the same
// shape as the WASAPI backend.

const (
	systemAudioID   = "system-audio"
	systemAudioName = "System audio (no driver needed)"
	maxDevices      = 128
	errLen          = 256
)

type macBackend struct{}

func NewBackend() Backend { return macBackend{} }

func (macBackend) Name() string { return "CoreAudio" }

func (macBackend) MicHint() string {
	return "Install BlackHole 2ch (existential.audio/blackhole) so apps can pick your phone as a microphone."
}

func (macBackend) CaptureHint() string {
	return "macOS asks once for permission to record system audio, and delivers silence until you allow it: " +
		"System Settings > Privacy & Security > Screen & System Audio Recording, then enable the app " +
		"you run phoneset from (Terminal, iTerm...). Quit and reopen it, then try again while audio is playing."
}

type caDevice struct {
	id     uint32
	uid    string
	name   string
	in     int
	out    int
	defOut bool
}

func listCA() ([]caDevice, error) {
	buf := make([]C.ca_device, maxDevices)
	n := int(C.ca_list_devices(&buf[0], C.int(maxDevices)))
	if n < 0 {
		return nil, errors.New("couldn't list the audio devices")
	}
	devs := make([]caDevice, 0, n)
	for _, d := range buf[:n] {
		devs = append(devs, caDevice{
			id:     uint32(d.id),
			uid:    C.GoString(&d.uid[0]),
			name:   C.GoString(&d.name[0]),
			in:     int(d.in_channels),
			out:    int(d.out_channels),
			defOut: d.is_default_out != 0,
		})
	}
	return devs, nil
}

func tapSupported() bool { return C.ca_tap_supported() == 1 }

func (macBackend) Devices() ([]Device, error) {
	raw, err := listCA()
	if err != nil {
		return nil, err
	}
	var devs []Device
	for _, d := range raw {
		devs = append(devs, Device{
			ID: d.uid, Name: d.name, Default: d.defOut, Virtual: looksVirtual(d.name),
			Mic: d.out > 0, Speaker: d.in > 0,
		})
	}
	if tapSupported() {
		devs = append(devs, Device{ID: systemAudioID, Name: systemAudioName, Default: true, Speaker: true})
	}
	return devs, nil
}

func (b macBackend) ResolveMic(selector string) (Device, error) {
	devs, err := b.Devices()
	if err != nil {
		return Device{}, err
	}
	devs = filter(devs, func(d Device) bool { return d.Mic })
	if selector != "" {
		return match(devs, selector)
	}
	virtual := filter(devs, func(d Device) bool { return d.Virtual })
	// A 2-channel cable suits a microphone; larger ones are better kept for capture.
	for _, d := range virtual {
		if strings.Contains(strings.ToLower(d.Name), "2ch") {
			return d, nil
		}
	}
	return firstVirtual(virtual)
}

func (b macBackend) ResolveSpeaker(selector string) (Device, error) {
	devs, err := b.Devices()
	if err != nil {
		return Device{}, err
	}
	devs = filter(devs, func(d Device) bool { return d.Speaker })
	if selector != "" {
		return match(devs, selector)
	}
	if d, err := defaultDevice(devs); err == nil && d.ID == systemAudioID {
		return d, nil
	}
	// No tap (macOS before 14.2): capture a virtual input other than the microphone cable.
	mic, _ := b.ResolveMic("")
	return firstVirtual(filter(devs, func(d Device) bool { return d.Virtual && d.ID != mic.ID }))
}

func deviceID(uid string) (uint32, error) {
	raw, err := listCA()
	if err != nil {
		return 0, err
	}
	for _, d := range raw {
		if d.uid == uid {
			return d.id, nil
		}
	}
	return 0, ErrNoDevice
}

// open runs one of the ca_open_* functions and starts the stream it returns.
func open(do func(err *C.char, n C.int) *C.ca_stream) (*C.ca_stream, error) {
	var msg [errLen]C.char
	s := do(&msg[0], errLen)
	if s == nil {
		return nil, errors.New(C.GoString(&msg[0]))
	}
	if C.ca_start(s, &msg[0], errLen) != 0 {
		C.ca_close(s)
		return nil, errors.New(C.GoString(&msg[0]))
	}
	return s, nil
}

func formatOf(s *C.ca_stream) RawFormat {
	return RawFormat{Rate: int(C.ca_rate(s)), Channels: int(C.ca_channels(s)), Bits: 32, Float: true}
}

// ---------------------------------------------------------------- microphone

type macSink struct {
	stream  *C.ca_stream
	format  RawFormat
	playout *Playout
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
}

func (macBackend) OpenMic(dev Device) (Sink, error) {
	id, err := deviceID(dev.ID)
	if err != nil {
		return nil, err
	}
	stream, err := open(func(e *C.char, n C.int) *C.ca_stream { return C.ca_open_output(C.uint32_t(id), e, n) })
	if err != nil {
		return nil, err
	}
	s := &macSink{
		stream:  stream,
		format:  formatOf(stream),
		playout: NewPlayout(phoneRate, 60, 150),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	go s.run()
	return s, nil
}

func (s *macSink) Write(pcm []byte)    { s.playout.Write(pcm) }
func (s *macSink) Stats() PlayoutStats { return s.playout.Stats() }

func (s *macSink) Close() error {
	s.once.Do(func() { close(s.stop) })
	<-s.done
	C.ca_close(s.stream)
	return nil
}

func (s *macSink) run() {
	defer close(s.done)
	var (
		feeder = NewFeeder(s.playout, s.format)
		target = s.format.Rate * renderTargetMs / 1000
		buf    []byte
	)
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-tick.C:
		}
		if C.ca_failed(s.stream) != 0 {
			return
		}
		queued := int(C.ca_queued(s.stream))
		if queued >= target {
			continue
		}
		want := target - queued
		if size := want * s.format.BlockAlign(); cap(buf) < size {
			buf = make([]byte, size)
		}
		buf = buf[:want*s.format.BlockAlign()]
		feeder.Fill(buf, want)
		C.ca_write(s.stream, (*C.float)(unsafe.Pointer(&buf[0])), C.size_t(want))
	}
}

// ------------------------------------------------------------------- speaker

type macSource struct {
	stream *C.ca_stream
	format RawFormat
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
}

func (macBackend) OpenSpeaker(dev Device, onFrame FrameFunc, onErr func(error)) (Source, error) {
	var stream *C.ca_stream
	var err error
	if dev.ID == systemAudioID {
		stream, err = open(func(e *C.char, n C.int) *C.ca_stream { return C.ca_open_system_tap(e, n) })
	} else {
		var id uint32
		if id, err = deviceID(dev.ID); err != nil {
			return nil, err
		}
		stream, err = open(func(e *C.char, n C.int) *C.ca_stream { return C.ca_open_input(C.uint32_t(id), e, n) })
	}
	if err != nil {
		return nil, err
	}
	s := &macSource{
		stream: stream,
		format: formatOf(stream),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	go s.run(SilenceGate(onFrame), onErr)
	return s, nil
}

func (s *macSource) Close() error {
	s.once.Do(func() { close(s.stop) })
	<-s.done
	C.ca_close(s.stream)
	return nil
}

func (s *macSource) run(onFrame FrameFunc, onErr func(error)) {
	defer close(s.done)
	const chunkFrames = 2048
	var (
		capture = NewCapturer(s.format, onFrame)
		floats  = make([]float32, chunkFrames*s.format.Channels)
	)
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-tick.C:
		}
		if C.ca_failed(s.stream) != 0 {
			if onErr != nil {
				onErr(fmt.Errorf("the audio device went away"))
			}
			return
		}
		for {
			n := int(C.ca_read(s.stream, (*C.float)(unsafe.Pointer(&floats[0])), C.size_t(chunkFrames)))
			if n == 0 {
				break
			}
			capture.Push(unsafe.Slice((*byte)(unsafe.Pointer(&floats[0])), n*s.format.BlockAlign()))
		}
	}
}
