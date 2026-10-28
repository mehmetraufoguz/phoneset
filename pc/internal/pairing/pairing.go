// Package pairing owns the short-lived pairing offer: a 6-digit code for typing
// and a 128-bit secret carried by the QR code. An offer is single use, expires,
// and is withdrawn after too many wrong attempts.
package pairing

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/mehmetraufoguz/phoneset/pc/internal/proto"
	"github.com/mehmetraufoguz/phoneset/pc/internal/secure"
)

const (
	DefaultTTL      = 5 * time.Minute
	DefaultMaxFails = 5
)

var (
	ErrNoOffer     = errors.New("no pairing in progress")
	ErrRateLimited = errors.New("too many wrong attempts")
	ErrBadProof    = errors.New("wrong code")
)

type Offer struct {
	Code    string
	Secret  []byte
	Expires time.Time
}

// FormatCode renders 123456 as "123 456".
func (o Offer) FormatCode() string { return o.Code[:3] + " " + o.Code[3:] }

type Manager struct {
	ttl      time.Duration
	maxFails int
	// OnWithdrawn is called (without the lock held) when an offer stops being
	// usable for a reason other than a successful pairing.
	OnWithdrawn func(reason string)

	mu    sync.Mutex
	offer *Offer
	fails int
	timer *time.Timer
}

func NewManager() *Manager {
	return &Manager{ttl: DefaultTTL, maxFails: DefaultMaxFails}
}

// Open replaces any current offer with a fresh one.
func (m *Manager) Open() (Offer, error) {
	code, err := randomCode()
	if err != nil {
		return Offer{}, err
	}
	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		return Offer{}, err
	}
	o := Offer{Code: code, Secret: secret, Expires: time.Now().Add(m.ttl)}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopTimerLocked()
	m.offer = &o
	m.fails = 0
	m.timer = time.AfterFunc(m.ttl, func() { m.withdraw(&o, "expired") })
	return o, nil
}

func (m *Manager) Current() (Offer, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.offer == nil {
		return Offer{}, false
	}
	return *m.offer, true
}

func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.offer = nil
	m.stopTimerLocked()
}

// Verify checks the phone's proof for this connection. On success the offer is
// consumed and the key that proved it is returned so the caller can answer with
// its own proof.
func (m *Manager) Verify(method, deviceID string, transcript, proof []byte) ([]byte, error) {
	m.mu.Lock()
	o := m.offer
	if o == nil {
		m.mu.Unlock()
		return nil, ErrNoOffer
	}
	var key []byte
	switch method {
	case proto.MethodQR:
		key = o.Secret
	case proto.MethodCode:
		key = secure.CodeKey(o.Code)
	default:
		m.mu.Unlock()
		return nil, ErrBadProof
	}
	want := secure.Proof(key, "phoneset/pair/c", transcript, []byte(deviceID))
	if secure.Equal(want, proof) {
		m.offer = nil
		m.stopTimerLocked()
		m.mu.Unlock()
		return key, nil
	}

	m.fails++
	exhausted := m.fails >= m.maxFails
	if exhausted {
		m.offer = nil
		m.stopTimerLocked()
	}
	m.mu.Unlock()
	if exhausted {
		if m.OnWithdrawn != nil {
			m.OnWithdrawn("too many wrong attempts")
		}
		return nil, ErrRateLimited
	}
	return nil, ErrBadProof
}

func (m *Manager) withdraw(o *Offer, reason string) {
	m.mu.Lock()
	if m.offer != o {
		m.mu.Unlock()
		return
	}
	m.offer = nil
	m.mu.Unlock()
	if m.OnWithdrawn != nil {
		m.OnWithdrawn(reason)
	}
}

func (m *Manager) stopTimerLocked() {
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
}

func randomCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
