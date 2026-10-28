package server

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mehmetraufoguz/phoneset/pc/internal/audio"
	"github.com/mehmetraufoguz/phoneset/pc/internal/proto"
	"github.com/mehmetraufoguz/phoneset/pc/internal/secure"
	"github.com/mehmetraufoguz/phoneset/pc/internal/store"
)

const (
	speakerQueueFrames = 8 // 160 ms; beyond that we drop rather than add latency
	statsInterval      = 2 * time.Second
)

type session struct {
	srv    *Server
	ch     *secure.Channel
	dev    store.Device
	cancel context.CancelFunc
	done   chan struct{}

	spkQ       chan []byte
	spkSent    atomic.Uint64
	spkDropped atomic.Uint64

	mu     sync.Mutex // guards the fields below
	sink   audio.Sink
	micDev audio.Device
	src    audio.Source
	spkDev audio.Device
}

func newSession(srv *Server, ch *secure.Channel, dev store.Device, cancel context.CancelFunc) *session {
	return &session{
		srv: srv, ch: ch, dev: dev, cancel: cancel,
		done: make(chan struct{}),
		spkQ: make(chan []byte, speakerQueueFrames),
	}
}

// run serves the phone until it leaves, returning a short reason for the log.
func (se *session) run(ctx context.Context) (reason string) {
	defer close(se.done)
	defer se.stopAll()

	if err := sendJSON(ctx, se.ch, se.ready()); err != nil {
		return "send failed"
	}
	go se.sendSpeaker(ctx)
	go se.sendStats(ctx)

	for {
		typ, body, err := se.ch.Recv(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return "closed"
			}
			return "connection lost"
		}
		switch typ {
		case proto.FrameMic:
			if len(body) > proto.AudioHeaderLen {
				se.mu.Lock()
				sink := se.sink
				se.mu.Unlock()
				if sink != nil {
					sink.Write(body[proto.AudioHeaderLen:])
				}
			}
		case proto.FrameControl:
			if bye := se.control(ctx, body); bye {
				return "phone left"
			}
		}
	}
}

func (se *session) control(ctx context.Context, body []byte) (bye bool) {
	var env proto.Envelope
	if json.Unmarshal(body, &env) != nil {
		return false
	}
	switch env.T {
	case proto.TStream:
		var req proto.Stream
		if json.Unmarshal(body, &req) == nil {
			_ = sendJSON(ctx, se.ch, se.applyStream(req))
		}
	case proto.TPing:
		var p proto.Ping
		if json.Unmarshal(body, &p) == nil {
			p.T = proto.TPong
			_ = sendJSON(ctx, se.ch, p)
		}
	case proto.TBye:
		return true
	}
	return false
}

func (se *session) ready() proto.Ready {
	r := proto.Ready{
		T: proto.TReady,
		Audio: proto.AudioFormat{
			Rate: proto.SampleRate, FrameMs: proto.FrameMs,
			MicChannels: proto.MicChannels, SpeakerChannels: proto.SpeakerChannels,
		},
	}
	opt := se.srv.opt
	switch dev, err := se.srv.audio.ResolveMic(opt.MicSelector); {
	case opt.NoMic:
		r.Mic.Reason = "Turned off on the PC (--no-mic)."
	case err != nil:
		r.Mic.Reason = micMissing(opt.MicSelector, se.srv.audio.MicHint())
	default:
		r.Mic = proto.Endpoint{Available: true, Device: dev.Name}
	}
	switch dev, err := se.srv.audio.ResolveSpeaker(opt.SpeakerSelector); {
	case opt.NoSpeaker:
		r.Speaker.Reason = "Turned off on the PC (--no-speaker)."
	case err != nil:
		r.Speaker.Reason = "No playback device found."
	default:
		r.Speaker = proto.Endpoint{Available: true, Device: dev.Name}
	}
	return r
}

func micMissing(selector, hint string) string {
	if selector != "" {
		return fmt.Sprintf("No playback device matching %q.", selector)
	}
	return "No virtual cable found on the PC. " + hint
}

// applyStream starts or stops streams and reports the effective state.
func (se *session) applyStream(req proto.Stream) proto.Stream {
	se.mu.Lock()
	defer se.mu.Unlock()

	var micErr, spkErr string
	if req.Mic != nil {
		if *req.Mic {
			if err := se.startMicLocked(); err != nil {
				micErr = err.Error()
			}
		} else {
			se.stopMicLocked()
		}
	}
	if req.Speaker != nil {
		if *req.Speaker {
			if err := se.startSpeakerLocked(); err != nil {
				spkErr = err.Error()
			}
		} else {
			se.stopSpeakerLocked()
		}
	}
	mic, spk := se.sink != nil, se.src != nil
	return proto.Stream{T: proto.TStream, Mic: &mic, Speaker: &spk, MicError: micErr, SpeakerErr: spkErr}
}

var errSameDevice = errors.New(
	"The microphone cable and the headphone source are the same playback device, which would feed your voice back to the phone. " +
		"Pick another device with --speaker-device.")

func (se *session) startMicLocked() error {
	if se.sink != nil {
		return nil
	}
	if se.srv.opt.NoMic {
		return errors.New("Turned off on the PC (--no-mic).")
	}
	dev, err := se.srv.audio.ResolveMic(se.srv.opt.MicSelector)
	if err != nil {
		return errors.New(micMissing(se.srv.opt.MicSelector, se.srv.audio.MicHint()))
	}
	if se.src != nil && se.spkDev.ID == dev.ID {
		return errSameDevice
	}
	sink, err := se.srv.audio.OpenMic(dev)
	if err != nil {
		return fmt.Errorf("Couldn't open %s: %w", dev.Name, err)
	}
	se.sink, se.micDev = sink, dev
	se.srv.log.Info("microphone on", "into", dev.Name)
	return nil
}

func (se *session) stopMicLocked() {
	if se.sink == nil {
		return
	}
	_ = se.sink.Close()
	se.sink = nil
	se.srv.log.Info("microphone off")
}

func (se *session) startSpeakerLocked() error {
	if se.src != nil {
		return nil
	}
	if se.srv.opt.NoSpeaker {
		return errors.New("Turned off on the PC (--no-speaker).")
	}
	dev, err := se.srv.audio.ResolveSpeaker(se.srv.opt.SpeakerSelector)
	if err != nil {
		return errors.New("No playback device found to capture.")
	}
	if se.sink != nil && se.micDev.ID == dev.ID {
		return errSameDevice
	}
	if se.sink == nil {
		// Not running yet, but the same collision would appear when the mic starts.
		if mic, err := se.srv.audio.ResolveMic(se.srv.opt.MicSelector); err == nil && mic.ID == dev.ID && !se.srv.opt.NoMic {
			se.srv.log.Warn("headphone source is also the microphone cable; the microphone will be refused while it runs",
				"device", dev.Name)
		}
	}
	src, err := se.srv.audio.OpenSpeaker(dev, se.queueFrame, se.speakerFailed)
	if err != nil {
		return fmt.Errorf("Couldn't capture %s: %w", dev.Name, err)
	}
	se.src, se.spkDev = src, dev
	se.srv.log.Info("headphones on", "from", dev.Name)
	return nil
}

func (se *session) stopSpeakerLocked() {
	if se.src == nil {
		return
	}
	_ = se.src.Close()
	se.src = nil
	se.srv.log.Info("headphones off")
}

func (se *session) stopAll() {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.stopMicLocked()
	se.stopSpeakerLocked()
}

// speakerFailed is called from the capture goroutine when the device went away.
func (se *session) speakerFailed(err error) {
	se.srv.log.Warn("headphone capture stopped", "err", err)
	go func() {
		se.mu.Lock()
		se.stopSpeakerLocked()
		se.mu.Unlock()
		off := false
		_ = sendJSON(context.Background(), se.ch, proto.Stream{
			T: proto.TStream, Speaker: &off, SpeakerErr: "The playback device stopped: " + err.Error(),
		})
	}()
}

// queueFrame is called from the capture goroutine; it must never block.
func (se *session) queueFrame(frame []byte) {
	cp := append([]byte(nil), frame...)
	for {
		select {
		case se.spkQ <- cp:
			return
		default:
			select {
			case <-se.spkQ:
				se.spkDropped.Add(1)
			default:
			}
		}
	}
}

func (se *session) sendSpeaker(ctx context.Context) {
	var seq uint32
	for {
		select {
		case <-ctx.Done():
			return
		case frame := <-se.spkQ:
			var hdr [proto.AudioHeaderLen]byte
			binary.BigEndian.PutUint32(hdr[:], seq)
			seq++
			if err := se.ch.Send(ctx, proto.FrameSpeaker, hdr[:], frame); err != nil {
				se.cancel()
				return
			}
			se.spkSent.Add(1)
		}
	}
}

func (se *session) sendStats(ctx context.Context) {
	tick := time.NewTicker(statsInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		st := proto.Stats{
			T:       proto.TStats,
			Speaker: proto.SpeakerStats{Sent: se.spkSent.Load(), Dropped: se.spkDropped.Load()},
		}
		se.mu.Lock()
		if se.sink != nil {
			ps := se.sink.Stats()
			st.Mic = proto.MicStats{BufferMs: ps.BufferedMs, Underruns: ps.Underruns, Drops: ps.Drops}
		}
		se.mu.Unlock()
		if err := sendJSON(ctx, se.ch, st); err != nil {
			return
		}
	}
}
