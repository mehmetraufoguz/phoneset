// Package control is the local control channel of a running agent: a small HTTP API on
// the loopback interface that the CLI (`phoneset status`, `stop`, `pair`), the tray and the
// pairing page use. It lives only on 127.0.0.1 and every request needs the token that the
// agent writes to control.json (readable by this user only).
package control

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"

	"github.com/mehmetraufoguz/phoneset/pc/internal/app"
	"github.com/mehmetraufoguz/phoneset/pc/internal/icons"
	"github.com/mehmetraufoguz/phoneset/pc/internal/store"
)

// Info is written to control.json while an agent runs.
type Info struct {
	PID     int       `json:"pid"`
	Port    int       `json:"port"`
	Token   string    `json:"token"`
	Mode    string    `json:"mode"` // "terminal" or "background"
	Started time.Time `json:"started"`
}

func infoPath(dir string) string { return filepath.Join(dir, "control.json") }

// Remove deletes the control file; the agent calls it on the way out.
func Remove(dir string) { _ = os.Remove(infoPath(dir)) }

// Handler is what the agent exposes.
type Handler interface {
	Snapshot() app.Snapshot
	OpenPairing() (app.PairingInfo, error)
	Disconnect() bool
	Forget(ref string) ([]store.Device, error)
}

// Serve listens on a free loopback port and publishes it until ctx ends.
// stop is called for POST /api/stop.
func Serve(ctx context.Context, dir, mode string, h Handler, stop func()) (Info, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Info{}, err
	}
	tok := make([]byte, 16)
	if _, err := rand.Read(tok); err != nil {
		return Info{}, err
	}
	info := Info{
		PID: os.Getpid(), Port: ln.Addr().(*net.TCPAddr).Port, Token: hex.EncodeToString(tok),
		Mode: mode, Started: time.Now(),
	}
	b, _ := json.Marshal(info)
	if err := os.WriteFile(infoPath(dir), b, 0o600); err != nil {
		return Info{}, err
	}

	s := &api{h: h, info: info, stop: stop}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.guard(s.status))
	mux.HandleFunc("POST /api/pair", s.guard(s.pair))
	mux.HandleFunc("POST /api/disconnect", s.guard(s.disconnect))
	mux.HandleFunc("POST /api/forget", s.guard(s.forget))
	mux.HandleFunc("POST /api/stop", s.guard(s.stopHandler))
	mux.HandleFunc("GET /pair", s.guard(s.pairPage))
	mux.HandleFunc("GET /qr.png", s.guard(s.qr))
	mux.HandleFunc("GET /logo.png", s.guard(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(icons.LogoPNG)
	}))
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		_ = os.Remove(infoPath(dir))
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	go func() { _ = srv.Serve(ln) }()
	return info, nil
}

type api struct {
	h    Handler
	info Info
	stop func()
}

// guard checks the token and that the request really targets the loopback address,
// which keeps a web page from reaching the agent through DNS rebinding.
func (a *api) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.Host)
		if host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			token = r.URL.Query().Get("t")
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(a.info.Token)) != 1 {
			http.Error(w, "unauthorised", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (a *api) status(w http.ResponseWriter, _ *http.Request) { writeJSON(w, a.h.Snapshot()) }

func (a *api) pair(w http.ResponseWriter, _ *http.Request) {
	p, err := a.h.OpenPairing()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, p)
}

func (a *api) disconnect(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]bool{"disconnected": a.h.Disconnect()})
}

func (a *api) forget(w http.ResponseWriter, r *http.Request) {
	removed, err := a.h.Forget(r.URL.Query().Get("ref"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]int{"forgotten": len(removed)})
}

func (a *api) stopHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]bool{"stopping": true})
	go a.stop()
}

func (a *api) qr(w http.ResponseWriter, _ *http.Request) {
	p := a.h.Snapshot().Pairing
	if p == nil {
		http.NotFound(w, nil)
		return
	}
	png, err := qrcode.Encode(p.Link, qrcode.Medium, 360)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

func (a *api) pairPage(w http.ResponseWriter, _ *http.Request) {
	// Opening the page starts a fresh offer, so the code and QR are always current.
	if _, err := a.h.OpenPairing(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(strings.ReplaceAll(pairHTML, "__TOKEN__", a.info.Token)))
}

// ----------------------------------------------------------------- client

var ErrNotRunning = errors.New("phoneset isn't running")

type Client struct {
	Info Info
	http *http.Client
}

// Connect finds a running agent for this config directory.
func Connect(dir string) (*Client, error) {
	raw, err := os.ReadFile(infoPath(dir))
	if err != nil {
		return nil, ErrNotRunning
	}
	var info Info
	if json.Unmarshal(raw, &info) != nil {
		return nil, ErrNotRunning
	}
	c := &Client{Info: info, http: &http.Client{Timeout: 3 * time.Second}}
	if _, err := c.Status(); err != nil {
		_ = os.Remove(infoPath(dir)) // left behind by an agent that died
		return nil, ErrNotRunning
	}
	return c, nil
}

func (c *Client) url(path string) string {
	return "http://127.0.0.1:" + strconv.Itoa(c.Info.Port) + path
}

func (c *Client) do(method, path string, out any) error {
	req, err := http.NewRequest(method, c.url(path), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Info.Token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) Status() (s app.Snapshot, err error)  { err = c.do("GET", "/api/status", &s); return }
func (c *Client) Pair() (p app.PairingInfo, err error) { err = c.do("POST", "/api/pair", &p); return }
func (c *Client) Stop() error                          { return c.do("POST", "/api/stop", nil) }
func (c *Client) Disconnect() error                    { return c.do("POST", "/api/disconnect", nil) }
func (c *Client) Forget(ref string) error {
	return c.do("POST", "/api/forget?ref="+strings.ReplaceAll(ref, " ", "%20"), nil)
}

// PairPageURL is the pairing page, with the token so a browser may open it.
func (c *Client) PairPageURL() string { return c.url("/pair?t=" + c.Info.Token) }
