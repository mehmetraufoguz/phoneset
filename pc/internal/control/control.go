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
	"fmt"
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
	"github.com/mehmetraufoguz/phoneset/pc/internal/proto"
	"github.com/mehmetraufoguz/phoneset/pc/internal/store"
	"github.com/mehmetraufoguz/phoneset/pc/internal/video"
)

// Info is written to control.json while an agent runs.
type Info struct {
	PID     int       `json:"pid"`
	Port    int       `json:"port"`
	Token   string    `json:"token"`
	Mode    string    `json:"mode"` // "terminal" or "background"
	Started time.Time `json:"started"`
	Args    []string  `json:"args,omitempty"` // the `serve` flags, so an update can start it the same way
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
	Video() *video.Hub
}

// Serve listens on a free loopback port and publishes it until ctx ends.
// stop is called for POST /api/stop. args are the flags the agent was started with.
func Serve(ctx context.Context, dir, mode string, args []string, h Handler, stop func()) (Info, error) {
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
		Mode: mode, Started: time.Now(), Args: args,
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
	// The camera of a phone (docs/protocol.md, section 9). Same listener, same token, same Host check.
	mux.HandleFunc("GET /video.mjpg", s.guard(s.mjpeg))
	mux.HandleFunc("GET /video.h264", s.guard(s.h264))
	mux.HandleFunc("GET /video.frames", s.guard(s.frames))
	mux.HandleFunc("GET /video.jpg", s.guard(s.snapshot))
	mux.HandleFunc("GET /video", s.guard(s.videoPage))
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

// ------------------------------------------------------------------ video

const mjpegBoundary = "phonesetframe"

// mjpeg streams the camera as multipart/x-mixed-replace: one image/jpeg part per frame, the bytes as
// the phone sent them. A reader that can't keep up is given the newest frame and skips the rest.
func (a *api) mjpeg(w http.ResponseWriter, r *http.Request) {
	sub := a.h.Video().Subscribe()
	defer sub.Close()
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+mjpegBoundary)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	rc := http.NewResponseController(w)
	// The opening delimiter goes out with the headers, and every frame is followed by the next delimiter
	// (as ffmpeg's mpjpeg does), so a reader that finds the end of a picture by the boundary has it the
	// moment the picture is complete instead of one frame later.
	_, _ = fmt.Fprintf(w, "--%s\r\n", mjpegBoundary)
	if rc.Flush() != nil {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case f := <-sub.C:
			if f.Codec != proto.CodecByteJPEG {
				continue // this stream carries another codec: there is no picture to show here
			}
			// A reader that stopped reading must not hold the connection forever.
			_ = rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := fmt.Fprintf(w, "Content-Type: image/jpeg\r\nContent-Length: %d\r\nX-Phoneset-Seq: %d\r\n\r\n",
				len(f.Data), f.Seq); err != nil {
				return
			}
			if _, err := w.Write(f.Data); err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "\r\n--%s\r\n", mjpegBoundary); err != nil {
				return
			}
			if rc.Flush() != nil {
				return
			}
		}
	}
}

// h264 streams the camera as a raw Annex B byte stream, which is what ffplay and ffmpeg read as "h264".
// It starts at a key frame, and when this reader missed a frame (the hub hands a slow reader only the
// newest) it waits for the next key frame again, so a decoder never sees a gap in the middle of a
// sequence of frames that depend on each other.
func (a *api) h264(w http.ResponseWriter, r *http.Request) {
	sub := a.h.Video().Subscribe()
	defer sub.Close()
	w.Header().Set("Content-Type", "video/h264")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	rc := http.NewResponseController(w)
	if rc.Flush() != nil {
		return
	}
	var gate keyGate
	// A phone that supports it is asked for a key frame now, so that this reader does not wait for the next one.
	a.h.Video().RequestKey()
	for {
		select {
		case <-r.Context().Done():
			return
		case f := <-sub.C:
			if f.Codec != proto.CodecByteH264 || !gate.pass(f, a.h.Video().RequestKey) {
				continue
			}
			_ = rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := w.Write(f.Data); err != nil {
				return
			}
			if rc.Flush() != nil {
				return
			}
		}
	}
}

func (a *api) snapshot(w http.ResponseWriter, _ *http.Request) {
	f := a.h.Video().Latest()
	if f == nil {
		http.Error(w, "no camera frame yet", http.StatusNotFound)
		return
	}
	if f.Codec != proto.CodecByteJPEG {
		http.Error(w, "the camera is sending h264, which is not a picture to save; read /video.h264", http.StatusUnsupportedMediaType)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(f.Data)
}

func (a *api) videoPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(strings.ReplaceAll(videoHTML, "__TOKEN__", a.info.Token)))
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

// VideoURL is where a browser, or OBS (as a browser source or a media source), can show the camera.
func (c *Client) VideoURL() string { return c.url("/video?t=" + c.Info.Token) }

// MJPEGURL is the raw stream: multipart/x-mixed-replace, for programs that read MJPEG.
func (c *Client) MJPEGURL() string { return c.url("/video.mjpg?t=" + c.Info.Token) }

// H264URL is the raw Annex B stream of an h264 camera, for ffplay, ffmpeg and the like.
func (c *Client) H264URL() string { return c.url("/video.h264?t=" + c.Info.Token) }

// FramesURL is the stream of records that carries each frame with its codec, size and orientation, for
// programs that decode the camera themselves (docs/protocol.md, section 9).
func (c *Client) FramesURL() string { return c.url("/video.frames?t=" + c.Info.Token) }

// PairPageURL is the pairing page, with the token so a browser may open it.
func (c *Client) PairPageURL() string { return c.url("/pair?t=" + c.Info.Token) }
