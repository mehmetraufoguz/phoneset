// Package audio is the PC's audio boundary. The rest of the program only knows
// the Backend interface; a platform file supplies NewBackend.
package audio

import (
	"errors"
	"strings"
)

// ErrNoDevice means no endpoint matched the selector.
var ErrNoDevice = errors.New("audio device not found")

type Device struct {
	ID      string
	Name    string
	Default bool
	// Virtual marks endpoints that look like a virtual cable and can therefore
	// serve as the PC-side "microphone".
	Virtual bool
}

// Sink plays the phone's microphone into the PC. Write takes 48 kHz mono s16le.
type Sink interface {
	Write(pcm []byte)
	Stats() PlayoutStats
	Close() error
}

// Source captures what the PC plays. Close stops it.
type Source interface {
	Close() error
}

// FrameFunc receives exactly 20 ms of 48 kHz stereo s16le per call.
type FrameFunc func(pcm []byte)

// Backend abstracts the platform audio stack.
type Backend interface {
	Name() string
	// Playback lists render endpoints (used for both roles).
	Playback() ([]Device, error)
	// ResolveMic picks the render endpoint that carries the virtual microphone.
	// An empty selector means "find a known virtual cable".
	ResolveMic(selector string) (Device, error)
	// ResolveSpeaker picks the render endpoint whose output is sent to the phone.
	// An empty selector means the system default.
	ResolveSpeaker(selector string) (Device, error)
	OpenMic(dev Device) (Sink, error)
	OpenSpeaker(dev Device, onFrame FrameFunc, onErr func(error)) (Source, error)
}

// virtualCableHints are substrings of well-known virtual cable endpoints.
var virtualCableHints = []string{"cable input", "hi-fi cable", "voicemeeter input", "virtual cable"}

func looksVirtual(name string) bool {
	n := strings.ToLower(name)
	for _, h := range virtualCableHints {
		if strings.Contains(n, h) {
			return true
		}
	}
	return false
}

// match finds a device by case-insensitive substring of its name or exact ID.
func match(devs []Device, selector string) (Device, error) {
	sel := strings.ToLower(strings.TrimSpace(selector))
	for _, d := range devs {
		if d.ID == selector {
			return d, nil
		}
	}
	for _, d := range devs {
		if strings.Contains(strings.ToLower(d.Name), sel) {
			return d, nil
		}
	}
	return Device{}, ErrNoDevice
}

func firstVirtual(devs []Device) (Device, error) {
	for _, d := range devs {
		if d.Virtual {
			return d, nil
		}
	}
	return Device{}, ErrNoDevice
}

func defaultDevice(devs []Device) (Device, error) {
	for _, d := range devs {
		if d.Default {
			return d, nil
		}
	}
	return Device{}, ErrNoDevice
}
