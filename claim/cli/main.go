// promo hands out phoneset App Store promo codes. It keeps the people (names, emails) on this Mac and
// talks to the claim Worker's database through wrangler; the database only ever knows token hashes
// and promo codes. See claim/README.md.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const usage = `promo: hand out phoneset App Store promo codes

  promo add --name "Ali" --email ali@example.com [--channel twitch] [--note "..."]
  promo list [--offline]            who has what, and how far along they are
  promo show <id>                   one person in full, with the link and the code
  promo mint [--days 7] <id>        make the link for a person
  promo mail [--sent] <id>          write the email (copied to the clipboard, opens your mail app)
  promo sync                        ask the database who pressed the button
  promo revoke <id>                 withdraw a link that was not pressed yet
  promo codes import --generated YYYY-MM-DD <file>   add the codes App Store Connect gave you
  promo codes status                how many codes are left, how many links you can still make
  promo key init [--force]          make the signing key (kept in the macOS Keychain); promo key show prints it

  --local   use the local test database (wrangler dev) instead of the real one
  --home D  folder for the private data (default: docs/store/private/promo)
`

type app struct {
	local bool
	home  string
	root  string
	cfg   config
	db    d1
	st    *store
}

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help" {
		fmt.Print(usage)
		os.Exit(2)
	}
	a := &app{}
	rest, err := a.globals(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	if len(rest) == 0 {
		fmt.Print(usage)
		os.Exit(2)
	}
	cmd, args := rest[0], rest[1:]
	switch cmd {
	case "add":
		err = a.cmdAdd(args)
	case "list":
		err = a.cmdList(args)
	case "show":
		err = a.cmdShow(args)
	case "mint":
		err = a.cmdMint(args)
	case "mail":
		err = a.cmdMail(args)
	case "sync":
		err = a.cmdSync(args)
	case "revoke":
		err = a.cmdRevoke(args)
	case "codes":
		err = a.cmdCodes(args)
	case "key":
		err = a.cmdKey(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// globals takes --local and --home out of the arguments wherever they stand (before or after the
// command) and returns what is left.
func (a *app) globals(args []string) ([]string, error) {
	var rest []string
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--local" || arg == "-local":
			a.local = true
		case arg == "--home" || arg == "-home":
			if i+1 >= len(args) {
				return nil, errors.New("--home needs a folder")
			}
			a.home = args[i+1]
			i++
		case strings.HasPrefix(arg, "--home="):
			a.home = strings.TrimPrefix(arg, "--home=")
		default:
			rest = append(rest, arg)
		}
	}
	return rest, nil
}

// flags makes a flag set for one command, and parse reads flags after the words
// (promo mint ali --days 5) as well as before them.
func (a *app) flags(name string) *flag.FlagSet {
	return flag.NewFlagSet(name, flag.ContinueOnError)
}

func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var words []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return words, nil
		}
		words = append(words, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// open finds the repository (the folder that has claim/worker/wrangler.toml above here), reads the
// config and loads the people.
func (a *app) open() error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "claim", "worker", "wrangler.toml")); err == nil {
			a.root = dir
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return errors.New("run this inside the phoneset repository (could not find claim/worker/wrangler.toml)")
		}
		dir = parent
	}
	if a.home == "" {
		name := "promo"
		if a.local {
			name = "promo-local" // test data never mixes with the real people
		}
		a.home = filepath.Join(a.root, "docs", "store", "private", name)
	}
	if err := os.MkdirAll(a.home, 0o700); err != nil {
		return err
	}
	a.cfg = config{BaseURL: "https://claim-phoneset.rauf.app", PageURL: "https://rauf.app/phoneset/claim", DB: "phoneset-claim"}
	cfgPath := filepath.Join(a.home, "config.json")
	if data, err := os.ReadFile(cfgPath); err == nil {
		if err := json.Unmarshal(data, &a.cfg); err != nil {
			return fmt.Errorf("%s: %v", cfgPath, err)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		data, _ := json.MarshalIndent(a.cfg, "", "  ")
		if err := os.WriteFile(cfgPath, append(data, '\n'), 0o600); err != nil {
			return err
		}
	}
	if a.local {
		// A local test uses the local Worker, and its own page unless PROMO_PAGE_URL names a local site.
		a.cfg.BaseURL = "http://127.0.0.1:8787"
		a.cfg.PageURL = os.Getenv("PROMO_PAGE_URL")
	}
	a.cfg.BaseURL = strings.TrimRight(a.cfg.BaseURL, "/")
	a.db = d1{dir: filepath.Join(a.root, "claim", "worker"), db: a.cfg.DB, local: a.local}
	a.st, err = loadStore(a.home)
	return err
}

// link is what goes into the email. On the page the token sits after the #, which a browser never sends to
// a server, so it stays out of every log; the Worker's own page has it in the path instead.
func (a *app) link(p person) string {
	if a.cfg.PageURL != "" {
		return strings.TrimRight(a.cfg.PageURL, "/") + "#" + p.Token
	}
	return a.cfg.BaseURL + "/c/" + p.Token
}
