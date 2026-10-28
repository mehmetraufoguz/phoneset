// Package store persists this PC's identity and the phones that paired with it.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Identity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Device struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Platform string    `json:"platform"`
	Token    string    `json:"token"` // base64, 32 random bytes; also the HMAC key
	PairedAt time.Time `json:"pairedAt"`
	LastSeen time.Time `json:"lastSeen"`
}

type Store struct {
	dir string

	mu       sync.Mutex
	identity Identity
	devices  map[string]Device
}

// DefaultDir is %AppData%\phoneset on Windows.
func DefaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "phoneset"), nil
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, devices: map[string]Device{}}

	if err := readJSON(s.path("identity.json"), &s.identity); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if s.identity.ID == "" {
		id := make([]byte, 8)
		if _, err := rand.Read(id); err != nil {
			return nil, err
		}
		host, _ := os.Hostname()
		if host == "" {
			host = "PC"
		}
		s.identity = Identity{ID: hex.EncodeToString(id), Name: host}
		if err := writeJSON(s.path("identity.json"), s.identity); err != nil {
			return nil, err
		}
	}

	var list []Device
	if err := readJSON(s.path("devices.json"), &list); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, d := range list {
		s.devices[d.ID] = d
	}
	return s, nil
}

func (s *Store) Dir() string { return s.dir }

func (s *Store) Identity() Identity {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.identity
}

// SetName changes the name phones see for this PC.
func (s *Store) SetName(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.identity.Name == name {
		return nil
	}
	s.identity.Name = name
	return writeJSON(s.path("identity.json"), s.identity)
}

func (s *Store) Devices() []Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sortedLocked()
}

func (s *Store) Device(id string) (Device, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[id]
	return d, ok
}

func (s *Store) Put(d Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices[d.ID] = d
	return s.saveLocked()
}

func (s *Store) Touch(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.devices[id]; ok {
		d.LastSeen = time.Now()
		s.devices[id] = d
		_ = s.saveLocked()
	}
}

// Remove deletes the device whose id or name equals ref (case-insensitive for names),
// or every device when ref is "all". It returns the removed devices.
func (s *Store) Remove(ref string) ([]Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var removed []Device
	for _, d := range s.sortedLocked() {
		if ref == "all" || d.ID == ref || strings.EqualFold(d.Name, ref) {
			removed = append(removed, d)
			delete(s.devices, d.ID)
		}
	}
	if len(removed) == 0 {
		return nil, fmt.Errorf("no paired phone matches %q", ref)
	}
	return removed, s.saveLocked()
}

func (s *Store) sortedLocked() []Device {
	out := make([]Device, 0, len(s.devices))
	for _, d := range s.devices {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PairedAt.Before(out[j].PairedAt) })
	return out
}

func (s *Store) saveLocked() error {
	return writeJSON(s.path("devices.json"), s.sortedLocked())
}

func (s *Store) path(name string) string { return filepath.Join(s.dir, name) }

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// writeJSON writes atomically with owner-only permissions: the file holds secrets.
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
