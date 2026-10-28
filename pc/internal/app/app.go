// Package app ties the pieces of the agent together and describes what it is doing,
// so the terminal, the tray, the local control API and the CLI all read one source.
package app

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mehmetraufoguz/phoneset/pc/internal/audio"
	"github.com/mehmetraufoguz/phoneset/pc/internal/discovery"
	"github.com/mehmetraufoguz/phoneset/pc/internal/pairing"
	"github.com/mehmetraufoguz/phoneset/pc/internal/proto"
	"github.com/mehmetraufoguz/phoneset/pc/internal/server"
	"github.com/mehmetraufoguz/phoneset/pc/internal/store"
)

type PairingInfo struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
	Link      string    `json:"link"`
}

type PhoneRow struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Platform string     `json:"platform"`
	LastSeen *time.Time `json:"lastSeen,omitempty"`
}

// Snapshot is everything a UI needs to draw the current state.
type Snapshot struct {
	Version string            `json:"version"`
	Name    string            `json:"name"`
	PID     int               `json:"pid"`
	Port    int               `json:"port"`
	Addrs   []string          `json:"addrs"`
	Backend string            `json:"backend"`
	Phone   *server.PhoneInfo `json:"phone"`
	Mic     server.StreamInfo `json:"mic"`
	Speaker server.StreamInfo `json:"speaker"`
	Pairing *PairingInfo      `json:"pairing"`
	Phones  []PhoneRow        `json:"phones"`
}

type App struct {
	Version string
	Port    int
	Store   *store.Store
	Pairing *pairing.Manager
	Server  *server.Server
	Backend audio.Backend
	Log     *slog.Logger

	// Hooks for a front end (the terminal prints from them). Called after state is updated.
	OnPaired        func(store.Device)
	OnPairingClosed func(reason string)

	mu        sync.Mutex
	listeners []func(Snapshot)
}

func New(version string, port int, st *store.Store, pm *pairing.Manager, srv *server.Server,
	be audio.Backend, log *slog.Logger) *App {
	a := &App{Version: version, Port: port, Store: st, Pairing: pm, Server: srv, Backend: be, Log: log}
	srv.OnChange(a.notify)
	srv.OnPaired = func(d store.Device) {
		log.Info("paired", "phone", d.Name, "platform", d.Platform)
		a.notify()
		if a.OnPaired != nil {
			a.OnPaired(d)
		}
	}
	pm.OnWithdrawn = func(reason string) {
		a.notify()
		if a.OnPairingClosed != nil {
			a.OnPairingClosed(reason)
		}
	}
	return a
}

// Run advertises the service and serves phones until ctx ends.
func (a *App) Run(ctx context.Context) error {
	ident := a.Store.Identity()
	if stop, err := discovery.Advertise(ident.ID, ident.Name, a.Port); err != nil {
		a.Log.Warn("mDNS is unavailable; pair with the QR code or the PC's address", "err", err)
	} else {
		defer stop()
	}
	return a.Server.Serve(ctx)
}

func (a *App) Snapshot() Snapshot {
	act := a.Server.Activity()
	s := Snapshot{
		Version: a.Version,
		Name:    a.Store.Identity().Name,
		PID:     os.Getpid(),
		Port:    a.Port,
		Addrs:   discovery.Addrs(),
		Backend: a.Backend.Name(),
		Phone:   act.Phone,
		Mic:     act.Mic,
		Speaker: act.Speaker,
		Phones:  []PhoneRow{},
	}
	if offer, ok := a.Pairing.Current(); ok {
		s.Pairing = &PairingInfo{Code: offer.FormatCode(), ExpiresAt: offer.Expires, Link: a.link(offer.Secret)}
	}
	for _, d := range a.Store.Devices() {
		row := PhoneRow{ID: d.ID, Name: d.Name, Platform: d.Platform}
		if !d.LastSeen.IsZero() {
			t := d.LastSeen
			row.LastSeen = &t
		}
		s.Phones = append(s.Phones, row)
	}
	return s
}

// OpenPairing starts a fresh pairing offer, replacing any current one.
func (a *App) OpenPairing() (PairingInfo, error) {
	offer, err := a.Pairing.Open()
	if err != nil {
		return PairingInfo{}, err
	}
	a.notify()
	return PairingInfo{Code: offer.FormatCode(), ExpiresAt: offer.Expires, Link: a.link(offer.Secret)}, nil
}

func (a *App) Disconnect() bool { return a.Server.Disconnect() }

// Forget unpairs phones matching ref (id, name or "all"); a connected one is dropped.
func (a *App) Forget(ref string) ([]store.Device, error) {
	removed, err := a.Store.Remove(ref)
	if err != nil {
		return nil, err
	}
	if act := a.Server.Activity(); act.Phone != nil {
		for _, d := range removed {
			if d.ID == act.Phone.ID {
				a.Server.Disconnect()
			}
		}
	}
	a.notify()
	return removed, nil
}

// Subscribe calls fn with the current state now and after every change.
func (a *App) Subscribe(fn func(Snapshot)) {
	a.mu.Lock()
	a.listeners = append(a.listeners, fn)
	a.mu.Unlock()
	fn(a.Snapshot())
}

func (a *App) notify() {
	a.mu.Lock()
	fns := append([]func(Snapshot){}, a.listeners...)
	a.mu.Unlock()
	if len(fns) == 0 {
		return
	}
	snap := a.Snapshot()
	for _, fn := range fns {
		fn(snap)
	}
}

// link is what the QR code carries: phoneset://pair?v=1&id=..&n=..&h=ip,ip&p=port&k=secret
func (a *App) link(secret []byte) string {
	ident := a.Store.Identity()
	addrs := discovery.Addrs()
	if len(addrs) > 3 {
		addrs = addrs[:3]
	}
	return fmt.Sprintf("phoneset://pair?v=%d&id=%s&n=%s&h=%s&p=%d&k=%s",
		proto.Version, ident.ID, url.QueryEscape(ident.Name), strings.Join(addrs, ","), a.Port,
		base64.RawURLEncoding.EncodeToString(secret))
}
