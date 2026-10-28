//go:build !windows && !(darwin && cgo)

package audio

import (
	"encoding/binary"
	"math"
	"os"
	"sync"
	"time"
)

// The development backend lets the whole program run where there is no real
// backend (Linux, or macOS built without cgo): the "speaker" is a 440 Hz tone and
// the "microphone" is played into a WAV file named by PHONESET_DEV_MIC_OUT (or
// discarded). Real audio exists for Windows (WASAPI) and macOS (CoreAudio).

type devBackend struct{}

func NewBackend() Backend { return devBackend{} }

func (devBackend) Name() string { return "dev (synthetic audio)" }

var devPlayback = []Device{
	{ID: "dev-speakers", Name: "Dev Speakers", Default: true, Mic: true, Speaker: true},
	{ID: "dev-cable", Name: "Dev Virtual Cable Input", Virtual: true, Mic: true, Speaker: true},
}

func (devBackend) MicHint() string     { return "" }
func (devBackend) CaptureHint() string { return "" }

func (devBackend) Devices() ([]Device, error) { return devPlayback, nil }

func (devBackend) ResolveMic(selector string) (Device, error) {
	if selector == "" {
		return firstVirtual(devPlayback)
	}
	return match(devPlayback, selector)
}

func (devBackend) ResolveSpeaker(selector string) (Device, error) {
	if selector == "" {
		return defaultDevice(devPlayback)
	}
	return match(devPlayback, selector)
}

type devSink struct {
	playout *Playout
	stop    chan struct{}
	done    chan struct{}
	file    *os.File
	written uint32
}

func (devBackend) OpenMic(Device) (Sink, error) {
	s := &devSink{
		playout: NewPlayout(48000, 60, 150),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	if path := os.Getenv("PHONESET_DEV_MIC_OUT"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		s.file = f
		_, _ = f.Write(make([]byte, 44)) // header patched on Close
	}
	go s.run()
	return s, nil
}

func (s *devSink) run() {
	defer close(s.done)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	samples := make([]int16, 480)
	out := make([]byte, 960)
	for {
		select {
		case <-s.stop:
			return
		case <-tick.C:
			s.playout.Read(samples)
			if s.file != nil {
				for i, v := range samples {
					binary.LittleEndian.PutUint16(out[i*2:], uint16(v))
				}
				n, _ := s.file.Write(out)
				s.written += uint32(n)
			}
		}
	}
}

func (s *devSink) Write(pcm []byte)    { s.playout.Write(pcm) }
func (s *devSink) Stats() PlayoutStats { return s.playout.Stats() }

func (s *devSink) Close() error {
	close(s.stop)
	<-s.done
	if s.file == nil {
		return nil
	}
	var h [44]byte
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], 36+s.written)
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 1)
	binary.LittleEndian.PutUint32(h[24:], 48000)
	binary.LittleEndian.PutUint32(h[28:], 96000)
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], s.written)
	_, _ = s.file.WriteAt(h[:], 0)
	return s.file.Close()
}

type devSource struct {
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

func (devBackend) OpenSpeaker(_ Device, onFrame FrameFunc, _ func(error)) (Source, error) {
	s := &devSource{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		const frames = 960
		buf := make([]byte, frames*4)
		phase := 0.0
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-tick.C:
				for i := 0; i < frames; i++ {
					v := int16(math.Sin(phase) * 0.2 * 32767)
					phase += 2 * math.Pi * 440 / 48000
					binary.LittleEndian.PutUint16(buf[i*4:], uint16(v))
					binary.LittleEndian.PutUint16(buf[i*4+2:], uint16(v))
				}
				onFrame(buf)
			}
		}
	}()
	return s, nil
}

func (s *devSource) Close() error {
	s.closeOnce.Do(func() { close(s.stop) })
	<-s.done
	return nil
}
