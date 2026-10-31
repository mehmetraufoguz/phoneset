package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Everything personal stays in this file on the owner's Mac (git-ignored folder); the database only ever
// sees a token hash.
type person struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Channel     string `json:"channel,omitempty"`
	Note        string `json:"note,omitempty"`
	Created     string `json:"created"`
	Token       string `json:"token,omitempty"`
	TokenHash   string `json:"tokenHash,omitempty"`
	MintedAt    string `json:"mintedAt,omitempty"`
	ExpiresAt   string `json:"expiresAt,omitempty"`
	MailedAt    string `json:"mailedAt,omitempty"`
	Code        string `json:"code,omitempty"`
	ClaimedSeen string `json:"claimedSeen,omitempty"`
	Revoked     bool   `json:"revoked,omitempty"`
}

type config struct {
	BaseURL string `json:"baseUrl"`           // the Worker, where the API is
	PageURL string `json:"pageUrl,omitempty"` // the page people open (on rauf.app); empty means the Worker's own page
	DB      string `json:"db"`
}

type store struct {
	dir    string
	People []person `json:"people"`
}

func (s *store) path() string { return filepath.Join(s.dir, "people.json") }

func loadStore(dir string) (*store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &store{dir: dir}
	data, err := os.ReadFile(s.path())
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %v", s.path(), err)
	}
	return s, nil
}

func (s *store) save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path() + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path())
}

func (s *store) find(id string) (*person, error) {
	for i := range s.People {
		if s.People[i].ID == id {
			return &s.People[i], nil
		}
	}
	return nil, fmt.Errorf("no one with the id %q (see: promo list)", id)
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func (s *store) newID(name string) string {
	base := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if base == "" {
		base = "person"
	}
	id := base
	for n := 2; ; n++ {
		if _, err := s.find(id); err != nil {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
}

func parseTime(v string) time.Time {
	t, _ := time.Parse(time.RFC3339, v)
	return t
}

// status: new, minted, mailed, claimed, expired or revoked. "claimed" means the person pressed the button
// and was shown a code (that is all the database can tell; whether Apple's code was redeemed is not known here).
func (p person) status(now time.Time) string {
	switch {
	case p.Revoked:
		return "revoked"
	case p.Code != "":
		return "claimed"
	case p.Token == "":
		return "new"
	case parseTime(p.ExpiresAt).Before(now):
		return "expired"
	case p.MailedAt != "":
		return "mailed"
	}
	return "minted"
}
