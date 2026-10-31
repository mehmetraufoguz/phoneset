package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// A link token is 32 bytes: 16 random bytes, then the first 16 bytes of HMAC-SHA256(key, tokenDomain + those
// bytes). The Worker (claim/worker/src/index.ts) checks the same thing before it asks the database anything,
// so a made-up link costs no database read.
const tokenDomain = "phoneset-claim-v1"

func keychainService() string {
	if s := os.Getenv("PROMO_KEYCHAIN_SERVICE"); s != "" {
		return s
	}
	return "phoneset-claim"
}

// The real key and the one for `--local` tests live under different names, so a test can never touch the real one.
func (a *app) keychainAccount() string {
	if a.local {
		return "claim-key-local"
	}
	return "claim-key"
}

func (a *app) storedKey() (string, error) {
	out, err := exec.Command("security", "find-generic-password", "-s", keychainService(), "-a", a.keychainAccount(), "-w").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// keyText is the key as base64url text: PROMO_CLAIM_KEY if set, else the macOS Keychain.
func (a *app) keyText() (string, error) {
	if t := strings.TrimSpace(os.Getenv("PROMO_CLAIM_KEY")); t != "" {
		return t, nil
	}
	t, err := a.storedKey()
	if err != nil {
		return "", errors.New("there is no signing key yet: run `promo key init`")
	}
	return t, nil
}

func (a *app) claimKey() ([]byte, error) {
	text, err := a.keyText()
	if err != nil {
		return nil, err
	}
	key, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil || len(key) < 16 {
		return nil, errors.New("the signing key is not valid (base64url text of 16 bytes or more)")
	}
	return key, nil
}

func signToken(key []byte) (string, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(tokenDomain))
	mac.Write(id)
	return base64.RawURLEncoding.EncodeToString(append(id, mac.Sum(nil)[:16]...)), nil
}

func (a *app) cmdKey(args []string) error {
	if len(args) == 0 || (args[0] != "init" && args[0] != "show") {
		return errors.New("usage: promo key init [--force] | promo key show")
	}
	fs := a.flags("key " + args[0])
	force := fs.Bool("force", false, "replace the existing key (links not pressed yet stop working)")
	if _, err := parse(fs, args[1:]); err != nil {
		return err
	}
	if err := a.open(); err != nil {
		return err
	}
	if args[0] == "show" {
		text, err := a.keyText()
		if err != nil {
			return err
		}
		fmt.Println(text)
		return nil
	}

	if _, err := a.storedKey(); err == nil && !*force {
		return errors.New("a key already exists. Replacing it makes every link that was not pressed yet stop working; use --force if you mean it")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	text := base64.RawURLEncoding.EncodeToString(raw)
	// The key is on the command line of `security` for a moment; this is your own Mac.
	add := exec.Command("security", "add-generic-password", "-U", "-s", keychainService(), "-a", a.keychainAccount(), "-l", "phoneset promo claim signing key", "-w", text)
	if out, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("could not save the key in the Keychain: %v\n%s", err, strings.TrimSpace(string(out)))
	}
	if a.local {
		path := filepath.Join(a.root, "claim", "worker", ".dev.vars")
		if err := os.WriteFile(path, []byte("CLAIM_KEY="+text+"\n"), 0o600); err != nil {
			return err
		}
		fmt.Println("key saved in the Keychain and written to claim/worker/.dev.vars (git-ignored) for the local Worker")
		return nil
	}
	fmt.Println("key saved in the Keychain. Give the Worker the same key (the key is piped, never printed):")
	fmt.Println("  promo key show | (cd claim/worker && wrangler secret put CLAIM_KEY)")
	return nil
}
