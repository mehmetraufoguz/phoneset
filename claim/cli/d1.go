package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// d1 talks to the claim database through wrangler, so there is no admin endpoint on the internet and no
// API token to keep: wrangler's own login does the work.
type d1 struct {
	dir   string // claim/worker, where wrangler.toml is
	db    string
	local bool
}

type row = map[string]any

func (d d1) query(sql string) ([]row, error) {
	if _, err := exec.LookPath("wrangler"); err != nil {
		return nil, fmt.Errorf("wrangler is not installed (npm i -g wrangler) or not on PATH")
	}
	args := []string{"d1", "execute", d.db, "--json", "--command", sql}
	if d.local {
		args = append(args, "--local")
	} else {
		args = append(args, "--remote")
	}
	cmd := exec.Command("wrangler", args...)
	cmd.Dir = d.dir
	cmd.Env = append(os.Environ(), "WRANGLER_SEND_METRICS=false", "NO_COLOR=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return nil, fmt.Errorf("wrangler: %v\n%s", err, msg)
	}
	out := stdout.Bytes()
	if i := bytes.IndexByte(out, '['); i >= 0 {
		out = out[i:]
	}
	var resp []struct {
		Results []row `json:"results"`
		Success bool  `json:"success"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("could not read wrangler's answer: %v\n%s", err, stdout.String())
	}
	var all []row
	for _, r := range resp {
		if !r.Success {
			return nil, fmt.Errorf("the database refused the query")
		}
		all = append(all, r.Results...)
	}
	return all, nil
}

func num(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	}
	return 0
}

// str reads a text column. Some wrangler versions print an SQL NULL as the word "null" in --json, so that
// word counts as empty (a real promo code is never four letters long).
func str(v any) string {
	if s, ok := v.(string); ok && s != "null" {
		return s
	}
	return ""
}
