// Package server accepts phone connections: the WebSocket upgrade, the
// encrypted handshake, pairing or authentication, and then a streaming session.
package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/mehmetraufoguz/phoneset/pc/internal/audio"
	"github.com/mehmetraufoguz/phoneset/pc/internal/pairing"
	"github.com/mehmetraufoguz/phoneset/pc/internal/proto"
	"github.com/mehmetraufoguz/phoneset/pc/internal/secure"
	"github.com/mehmetraufoguz/phoneset/pc/internal/store"
)

const authTimeout = 10 * time.Second

type Options struct {
	Port            int
	MicSelector     string
	SpeakerSelector string
	NoMic           bool
	NoSpeaker       bool
}

type Server struct {
	opt   Options
	store *store.Store
	pair  *pairing.Manager
	audio audio.Backend
	log   *slog.Logger

	// OnPaired is called after a phone completed pairing.
	OnPaired func(store.Device)

	mu     sync.Mutex
	active *session
}

func New(opt Options, st *store.Store, pm *pairing.Manager, be audio.Backend, log *slog.Logger) *Server {
	return &Server{opt: opt, store: st, pair: pm, audio: be, log: log}
}

// Serve blocks until ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.opt.Port))
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc(proto.InfoPath, s.handleInfo)
	mux.HandleFunc(proto.WSPath, s.handleWS)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		if s.active != nil {
			s.active.cancel()
		}
		s.mu.Unlock()
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	id := s.store.Identity()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(proto.Info{App: "phoneset", V: proto.Version, ID: id.ID, Name: id.Name})
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Native apps send no Origin; everything is authenticated cryptographically.
		OriginPatterns: []string{"*"},
	})
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
		s.log.Debug("handshake failed", "remote", r.RemoteAddr, "err", err)
		return
	}
	dev, err := s.authenticate(hs, ch)
	hsCancel()
	if err != nil {
		s.log.Debug("authentication failed", "remote", r.RemoteAddr, "err", err)
		ch.Close("unauthorised")
		return
	}

	se := newSession(s, ch, dev, cancel)
	if err := s.attach(se); err != nil {
		_ = sendJSON(ctx, ch, proto.Error{T: proto.TError, Code: proto.ErrBusy,
			Message: "Another phone is already connected to this PC."})
		ch.Close("busy")
		return
	}
	defer s.detach(se)

	s.log.Info("phone connected", "name", dev.Name, "from", remoteIP(r.RemoteAddr))
	reason := se.run(ctx)
	s.log.Info("phone disconnected", "name", dev.Name, "reason", reason)
	ch.Close("bye")
}

// authenticate expects a pair or auth message and answers it.
func (s *Server) authenticate(ctx context.Context, ch *secure.Channel) (store.Device, error) {
	typ, body, err := ch.Recv(ctx)
	if err != nil {
		return store.Device{}, err
	}
	if typ != proto.FrameControl {
		return store.Device{}, s.reject(ctx, ch, proto.ErrProtocol, "Expected a pair or auth message.")
	}
	var env proto.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return store.Device{}, s.reject(ctx, ch, proto.ErrProtocol, "Malformed message.")
	}
	switch env.T {
	case proto.TPair:
		var m proto.Pair
		if err := json.Unmarshal(body, &m); err != nil {
			return store.Device{}, s.reject(ctx, ch, proto.ErrProtocol, "Malformed pair message.")
		}
		return s.handlePair(ctx, ch, m)
	case proto.TAuth:
		var m proto.Auth
		if err := json.Unmarshal(body, &m); err != nil {
			return store.Device{}, s.reject(ctx, ch, proto.ErrProtocol, "Malformed auth message.")
		}
		return s.handleAuth(ctx, ch, m)
	}
	return store.Device{}, s.reject(ctx, ch, proto.ErrProtocol, "Expected a pair or auth message.")
}

func (s *Server) handlePair(ctx context.Context, ch *secure.Channel, m proto.Pair) (store.Device, error) {
	id, name := strings.TrimSpace(m.DeviceID), strings.TrimSpace(m.Name)
	if id == "" || len(id) > 64 {
		return store.Device{}, s.reject(ctx, ch, proto.ErrProtocol, "Missing device id.")
	}
	if name == "" {
		name = "Phone"
	}
	if len(name) > 64 {
		name = name[:64]
	}
	proof, err := base64.StdEncoding.DecodeString(m.Proof)
	if err != nil {
		return store.Device{}, s.reject(ctx, ch, proto.ErrBadProof, "Malformed proof.")
	}

	key, err := s.pair.Verify(m.Method, id, ch.Transcript(), proof)
	switch {
	case errors.Is(err, pairing.ErrNoOffer):
		return store.Device{}, s.reject(ctx, ch, proto.ErrNoPairing,
			"Pairing isn't open on the PC. Type p in the phoneset window to start it.")
	case errors.Is(err, pairing.ErrRateLimited):
		return store.Device{}, s.reject(ctx, ch, proto.ErrRateLimited,
			"Too many wrong codes. Start pairing again on the PC.")
	case err != nil:
		return store.Device{}, s.reject(ctx, ch, proto.ErrBadProof, "That code didn't match.")
	}

	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return store.Device{}, err
	}
	now := time.Now()
	dev := store.Device{
		ID: id, Name: name, Platform: m.Platform,
		Token: base64.StdEncoding.EncodeToString(token), PairedAt: now, LastSeen: now,
	}
	if err := s.store.Put(dev); err != nil {
		return store.Device{}, err
	}

	ident := s.store.Identity()
	err = sendJSON(ctx, ch, proto.Paired{
		T: proto.TPaired, PCID: ident.ID, PCName: ident.Name, Token: dev.Token,
		Proof: b64(secure.Proof(key, "phoneset/pair/s", ch.Transcript(), []byte(id), token)),
	})
	if err != nil {
		return store.Device{}, err
	}
	if s.OnPaired != nil {
		s.OnPaired(dev)
	}
	return dev, nil
}

func (s *Server) handleAuth(ctx context.Context, ch *secure.Channel, m proto.Auth) (store.Device, error) {
	dev, ok := s.store.Device(m.DeviceID)
	if !ok {
		return store.Device{}, s.reject(ctx, ch, proto.ErrUnknownDevice,
			"This PC doesn't know this phone any more. Pair again.")
	}
	token, err := base64.StdEncoding.DecodeString(dev.Token)
	if err != nil {
		return store.Device{}, err
	}
	proof, err := base64.StdEncoding.DecodeString(m.Proof)
	if err != nil || !secure.Equal(proof, secure.Proof(token, "phoneset/auth/c", ch.Transcript(), []byte(dev.ID))) {
		return store.Device{}, s.reject(ctx, ch, proto.ErrBadProof, "Authentication failed.")
	}

	ident := s.store.Identity()
	err = sendJSON(ctx, ch, proto.Authed{
		T: proto.TAuthed, PCID: ident.ID, PCName: ident.Name,
		Proof: b64(secure.Proof(token, "phoneset/auth/s", ch.Transcript(), []byte(dev.ID))),
	})
	if err != nil {
		return store.Device{}, err
	}
	s.store.Touch(dev.ID)
	return dev, nil
}

func (s *Server) reject(ctx context.Context, ch *secure.Channel, code, message string) error {
	_ = sendJSON(ctx, ch, proto.Error{T: proto.TError, Code: code, Message: message})
	return errors.New(code)
}

var errBusy = errors.New("busy")

// attach makes se the active session. The same phone reconnecting replaces its
// stale connection; a different phone is refused.
func (s *Server) attach(se *session) error {
	for {
		s.mu.Lock()
		old := s.active
		if old == nil {
			s.active = se
			s.mu.Unlock()
			return nil
		}
		if old.dev.ID != se.dev.ID {
			s.mu.Unlock()
			return errBusy
		}
		s.mu.Unlock()
		old.cancel()
		select {
		case <-old.done:
		case <-time.After(3 * time.Second):
			return errBusy
		}
	}
}

func (s *Server) detach(se *session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == se {
		s.active = nil
	}
}

func sendJSON(ctx context.Context, ch *secure.Channel, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return ch.Send(ctx, proto.FrameControl, b)
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func remoteIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
