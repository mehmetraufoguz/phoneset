// Package video keeps the camera frames a phone sends and hands them, unchanged, to local readers
// (the MJPEG endpoint of the control API). It never decodes a picture.
package video

import (
	"sync"
	"time"
)

// Frame is one coded picture. It is immutable once published, so readers share it.
type Frame struct {
	Seq        uint32
	CapturedMs uint32 // the phone's steady clock, wraps; only differences mean anything
	Width      int
	Height     int
	Codec      byte   // proto.CodecByteJPEG or proto.CodecByteH264
	Key        bool   // can be decoded on its own
	Rotation   int    // quarter turns clockwise that make the decoded picture upright (0 to 3)
	Mirror     bool   // mirror it horizontally after that
	Data       []byte // a JPEG, or one Annex B access unit for h264
	Received   time.Time
}

// Status describes the video stream right now.
type Status struct {
	On       bool    `json:"on"`
	Camera   string  `json:"camera,omitempty"`
	Codec    string  `json:"codec,omitempty"`
	AskedFPS int     `json:"askedFps,omitempty"` // the rate the phone was asked to capture at
	Width    int     `json:"width,omitempty"`
	Height   int     `json:"height,omitempty"`
	FPS      float64 `json:"fps"`
	KBps     float64 `json:"kBps"`
	Frames   uint64  `json:"frames"`
	Bytes    uint64  `json:"bytes"`
	Viewers  int     `json:"viewers"`
}

const rateWindow = 32 // frames the rate is measured over

// Hub fans the latest frame out to any number of readers. A reader that is slow loses the frames it
// could not take (it always gets the newest one), so nothing a reader does can slow the phone down.
type Hub struct {
	mu     sync.Mutex
	on     bool
	gen    uint64 // which Begin the stream belongs to, so a late End of an older one does nothing
	camera string
	codec  string
	asked  int
	latest *Frame
	subs   map[*Sub]struct{}
	frames uint64
	bytes  uint64

	// asks the phone for a key frame, set while a phone that supports it is sending (see RequestKey)
	keyReq     func()
	lastKeyReq time.Time

	// arrival times and sizes of the last rateWindow frames, for the rates
	ring  [rateWindow]arrival
	count int
}

type arrival struct {
	at   time.Time
	size int
}

func NewHub() *Hub { return &Hub{subs: map[*Sub]struct{}{}} }

// Sub is one reader. Receive from C; call Close when done.
type Sub struct {
	C   <-chan *Frame
	ch  chan *Frame
	hub *Hub
}

// Close detaches the reader.
func (s *Sub) Close() {
	s.hub.mu.Lock()
	delete(s.hub.subs, s)
	s.hub.mu.Unlock()
}

// Subscribe attaches a reader. If a frame is available it is delivered first, so a late reader
// has a picture straight away.
func (h *Hub) Subscribe() *Sub {
	ch := make(chan *Frame, 1)
	s := &Sub{C: ch, ch: ch, hub: h}
	h.mu.Lock()
	if h.latest != nil {
		ch <- h.latest
	}
	h.subs[s] = struct{}{}
	h.mu.Unlock()
	return s
}

// Begin marks the start of a video stream from a phone and clears the counters. The number it returns
// is the stream's: only End with that number ends it.
func (h *Hub) Begin(camera, codec string, askedFPS int) uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.on {
		h.frames, h.bytes, h.count = 0, 0, 0
		h.latest = nil
	}
	h.on, h.camera, h.codec, h.asked = true, camera, codec, askedFPS
	h.gen++
	return h.gen
}

// End marks the end of the stream that Begin numbered gen; if a newer stream has begun since, it does
// nothing. The last picture is forgotten so a reader never shows a stale one.
func (h *Hub) End(gen uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if gen != h.gen {
		return
	}
	h.on, h.camera, h.codec, h.asked, h.latest = false, "", "", 0, nil
	h.keyReq = nil
}

// SetKeyRequester registers how to ask the phone of the stream that Begin numbered gen for a key frame. A newer
// stream that has begun since replaces it (and ignores a late call for an older one).
func (h *Hub) SetKeyRequester(gen uint64, f func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if gen == h.gen {
		h.keyReq = f
	}
}

// RequestKey asks the phone for a key frame, if it can be asked (it listed the feature) and no request was
// made in the last 300 ms. A reader calls it when it joins and when it has lost its place.
func (h *Hub) RequestKey() {
	h.mu.Lock()
	f := h.keyReq
	if f == nil || time.Since(h.lastKeyReq) < 300*time.Millisecond {
		h.mu.Unlock()
		return
	}
	h.lastKeyReq = time.Now()
	h.mu.Unlock()
	f()
}

// Publish hands a frame to every reader without ever blocking.
func (h *Hub) Publish(f *Frame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.on {
		return
	}
	f.Received = time.Now()
	h.latest = f
	h.frames++
	h.bytes += uint64(len(f.Data))
	h.ring[h.count%rateWindow] = arrival{at: f.Received, size: len(f.Data)}
	h.count++
	for s := range h.subs {
		select {
		case s.ch <- f:
		default:
			// Only Publish sends (under h.mu), so after taking the old frame out there is room.
			select {
			case <-s.ch:
			default:
			}
			s.ch <- f
		}
	}
}

// Latest returns the newest frame, or nil.
func (h *Hub) Latest() *Frame {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.latest
}

// Active reports whether a phone is sending video.
func (h *Hub) Active() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.on
}

// Status reports the stream and its recent rates.
func (h *Hub) Status() Status {
	h.mu.Lock()
	defer h.mu.Unlock()
	st := Status{On: h.on, Camera: h.camera, Codec: h.codec, AskedFPS: h.asked, Frames: h.frames, Bytes: h.bytes, Viewers: len(h.subs)}
	if h.latest != nil {
		st.Width, st.Height = h.latest.Width, h.latest.Height
	}
	n := h.count
	if n > rateWindow {
		n = rateWindow
	}
	if h.on && n >= 2 && time.Since(h.ring[(h.count-1)%rateWindow].at) < time.Second {
		oldest := h.ring[(h.count-n)%rateWindow]
		newest := h.ring[(h.count-1)%rateWindow]
		if span := newest.at.Sub(oldest.at).Seconds(); span > 0 {
			bytes := 0
			for i := h.count - n + 1; i < h.count; i++ { // the frames that arrived after the oldest one
				bytes += h.ring[i%rateWindow].size
			}
			st.FPS = float64(n-1) / span
			st.KBps = float64(bytes) / span / 1000
		}
	}
	return st
}
