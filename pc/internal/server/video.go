package server

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/mehmetraufoguz/phoneset/pc/internal/proto"
	"github.com/mehmetraufoguz/phoneset/pc/internal/secure"
	"github.com/mehmetraufoguz/phoneset/pc/internal/store"
	"github.com/mehmetraufoguz/phoneset/pc/internal/video"
)

// The phone's camera arrives on a connection of its own (docs/protocol.md, section 8), not on the one
// that carries the microphone and the headphones: a video frame that is slow to send can then never
// sit in front of a piece of audio. It is a second encrypted channel, opened by a phone that is already
// connected, proven with the same pairing token, and it exists only while the phone wants video.
//
// Everything about video lives in this file; the audio session knows only that a video connection
// may belong to it.

// videoCodecs are the codecs this PC accepts. It relays JPEG frames without decoding them; a codec
// that needs a decoder is added here when its payload is defined (docs/protocol.md, section 8.7).
var videoCodecs = []string{proto.CodecJPEG, proto.CodecH264}

// Bounds for a mode the phone asks for. The 16-bit sizes of the frame header are the real limit.
const (
	maxVideoSide = 16384
	maxVideoFPS  = 240
)

// videoConn is one video connection.
type videoConn struct {
	ch     *secure.Channel
	cancel context.CancelFunc
	done   chan struct{}

	// used by the goroutine that reads the connection only
	on     bool
	camera string
	mode   proto.VideoMode // the mode in effect, while on
	gen    uint64          // the Hub's number for this stream
	bad    uint64
}

func (s *Server) handleVideoWS(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if err != nil {
		return
	}
	defer ws.CloseNow()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	hs, hsCancel := context.WithTimeout(ctx, authTimeout)
	ch, err := secure.Accept(hs, ws)
	if err != nil {
		hsCancel()
		s.log.Debug("video handshake failed", "remote", r.RemoteAddr, "err", err)
		return
	}
	dev, err := s.authenticateKnown(hs, ch)
	hsCancel()
	if err != nil {
		s.log.Debug("video authentication failed", "remote", r.RemoteAddr, "err", err)
		ch.Close("unauthorised")
		return
	}

	if s.opt.NoVideo {
		_ = s.reject(ctx, ch, proto.ErrVideoOff, "Turned off on the PC (--no-video).")
		ch.Close("off")
		return
	}
	se := s.sessionOf(dev.ID)
	if se == nil {
		_ = s.reject(ctx, ch, proto.ErrNoSession, "Connect the phone to this PC first.")
		ch.Close("no session")
		return
	}

	vc := &videoConn{ch: ch, cancel: cancel, done: make(chan struct{})}
	if err := se.attachVideo(vc); err != nil {
		_ = s.reject(ctx, ch, proto.ErrNoSession, "The phone's connection to this PC has ended.")
		ch.Close("no session")
		return
	}
	defer se.detachVideo(vc)
	defer close(vc.done)
	defer s.stopVideo(vc)

	// Authentication is over: let frames through (the limit was small until now).
	ch.SetMaxMessage(proto.MaxVideoFrame + proto.VideoHeaderLen + 64)
	s.log.Debug("video connection open", "name", dev.Name)
	reason := s.runVideo(ctx, vc)
	s.log.Debug("video connection closed", "name", dev.Name, "reason", reason)
	ch.Close("bye")
}

// authenticateKnown accepts only an auth message: the camera is for a phone that is already paired.
func (s *Server) authenticateKnown(ctx context.Context, ch *secure.Channel) (store.Device, error) {
	typ, body, err := ch.Recv(ctx)
	if err != nil {
		return store.Device{}, err
	}
	var m proto.Auth
	if typ != proto.FrameControl || json.Unmarshal(body, &m) != nil || m.T != proto.TAuth {
		return store.Device{}, s.reject(ctx, ch, proto.ErrProtocol, "The video connection needs an auth message from a paired phone.")
	}
	return s.handleAuth(ctx, ch, m)
}

func (s *Server) sessionOf(deviceID string) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != nil && s.active.dev.ID == deviceID {
		return s.active
	}
	return nil
}

var errNoSession = errors.New("no session")

// attachVideo makes vc the session's video connection, replacing an older one.
func (se *session) attachVideo(vc *videoConn) error {
	se.mu.Lock()
	if se.closed {
		se.mu.Unlock()
		return errNoSession
	}
	old := se.vc
	se.vc = vc
	se.mu.Unlock()
	if old != nil {
		old.cancel()
		select {
		case <-old.done:
		case <-time.After(3 * time.Second):
		}
	}
	return nil
}

func (se *session) detachVideo(vc *videoConn) {
	se.mu.Lock()
	if se.vc == vc {
		se.vc = nil
	}
	se.mu.Unlock()
}

func (s *Server) stopVideo(vc *videoConn) {
	if vc.on {
		vc.on = false
		s.video.End(vc.gen)
		s.log.Info("camera off")
		s.changed()
	}
}

// runVideo serves one video connection until it ends, returning a short reason for the log.
func (s *Server) runVideo(ctx context.Context, vc *videoConn) string {
	go s.sendVideoStats(ctx, vc)
	for {
		typ, body, err := vc.ch.Recv(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return "closed"
			}
			return "connection lost"
		}
		switch typ {
		case proto.FrameVideo:
			s.onVideo(ctx, vc, body)
		case proto.FrameControl:
			if bye := s.videoControl(ctx, vc, body); bye {
				return "phone left"
			}
		}
	}
}

func (s *Server) videoControl(ctx context.Context, vc *videoConn, body []byte) (bye bool) {
	var env proto.Envelope
	if json.Unmarshal(body, &env) != nil {
		return false
	}
	switch env.T {
	case proto.TVideo:
		var req proto.VideoState
		if json.Unmarshal(body, &req) != nil {
			return false
		}
		_ = sendJSON(ctx, vc.ch, s.applyVideo(vc, req))
	case proto.TBye:
		return true
	}
	return false
}

// checkMode says why a mode can't be accepted, or "".
func checkMode(req proto.VideoState) string {
	m := req.Mode
	if m == nil {
		return "The video request has no mode."
	}
	if !slices.Contains(videoCodecs, m.Codec) {
		return fmt.Sprintf("This PC can't receive %q video (it takes: %s).", m.Codec, strings.Join(videoCodecs, ", "))
	}
	if m.Width < 1 || m.Height < 1 || m.Width > maxVideoSide || m.Height > maxVideoSide || m.FPS < 1 || m.FPS > maxVideoFPS {
		return "That video size or frame rate is out of range."
	}
	if len(req.Modes) > 0 && !slices.Contains(req.Modes, *m) {
		return "The mode is not one of the modes the phone offered."
	}
	return ""
}

// applyVideo starts or stops the camera stream and reports the state that is in effect.
func (s *Server) applyVideo(vc *videoConn, req proto.VideoState) proto.VideoState {
	camera := req.Camera
	if camera != proto.CameraBack && camera != proto.CameraFront {
		camera = vc.camera // an unknown value keeps what we had
	}
	if camera == "" {
		camera = proto.CameraBack
	}
	if req.On {
		if why := checkMode(req); why != "" {
			s.stopVideo(vc)
			return proto.VideoState{T: proto.TVideo, On: false, Error: why}
		}
		mode := *req.Mode
		if !vc.on || vc.camera != camera || vc.mode != mode {
			s.log.Debug("video modes offered", "count", len(req.Modes))
			vc.gen = s.video.Begin(camera, mode.Codec, mode.FPS)
			if !vc.on {
				s.log.Info("camera on", "facing", camera, "codec", mode.Codec, "size", fmt.Sprintf("%dx%d", mode.Width, mode.Height), "fps", mode.FPS)
			}
			vc.on, vc.camera, vc.mode = true, camera, mode
			s.changed()
		}
	} else {
		s.stopVideo(vc)
	}
	state := proto.VideoState{T: proto.TVideo, On: vc.on}
	if vc.on {
		mode := vc.mode
		state.Camera, state.Mode = vc.camera, &mode
	}
	return state
}

// onVideo relays one camera frame to the readers and tells the phone it arrived (section 8). The
// picture is not decoded; a JPEG only has to look like one.
func (s *Server) onVideo(ctx context.Context, vc *videoConn, body []byte) {
	if !vc.on || len(body) < proto.VideoHeaderLen {
		return // the camera is off (a frame that was already on its way) or this is not a frame
	}
	seq := binary.BigEndian.Uint32(body[0:4])
	codec := body[8]
	w, h := int(binary.BigEndian.Uint16(body[10:12])), int(binary.BigEndian.Uint16(body[12:14]))
	payload := body[proto.VideoHeaderLen:]
	key := body[9]&1 == 1
	good := w > 0 && h > 0 &&
		(vc.mode.Codec == proto.CodecJPEG && codec == proto.CodecByteJPEG && isJPEG(payload) ||
			vc.mode.Codec == proto.CodecH264 && codec == proto.CodecByteH264 && isAnnexB(payload))
	if good {
		s.video.Publish(&video.Frame{
			Seq: seq, CapturedMs: binary.BigEndian.Uint32(body[4:8]), Width: w, Height: h,
			Codec: codec, Key: key, Data: payload,
		})
	} else if vc.bad++; vc.bad == 1 {
		s.log.Warn("ignored a camera frame that is not what the mode says", "codec", codec, "bytes", len(payload))
	}
	// Acknowledged either way, so the phone's window never stalls on a bad frame.
	_ = sendJSON(ctx, vc.ch, proto.VideoAck{T: proto.TVAck, Seq: seq})
}

func isJPEG(b []byte) bool {
	return len(b) >= 4 && len(b) <= proto.MaxVideoFrame && b[0] == 0xFF && b[1] == 0xD8
}

// isAnnexB reports whether b starts with an Annex B start code (00 00 01 or 00 00 00 01), which is as
// much as is checked: the PC does not decode h264.
func isAnnexB(b []byte) bool {
	if len(b) < 5 || len(b) > proto.MaxVideoFrame {
		return false
	}
	return b[0] == 0 && b[1] == 0 && (b[2] == 1 || b[2] == 0 && b[3] == 1)
}

func (s *Server) sendVideoStats(ctx context.Context, vc *videoConn) {
	tick := time.NewTicker(statsInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		st := s.video.Status()
		if !st.On {
			continue
		}
		msg := proto.VideoStats{T: proto.TVStats, Frames: st.Frames, Bytes: st.Bytes, Viewers: st.Viewers}
		if err := sendJSON(ctx, vc.ch, msg); err != nil {
			return
		}
	}
}
