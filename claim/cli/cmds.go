package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"
)

var (
	codeRe = regexp.MustCompile(`^[A-Za-z0-9]{6,32}$`)
	dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	hashRe = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// quote is only ever used on values that were checked above (codes, dates, hashes), but it still doubles
// any quote so a typo can never become a query.
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func (a *app) cmdAdd(args []string) error {
	fs := a.flags("add")
	name := fs.String("name", "", "their name")
	email := fs.String("email", "", "their email (stays on this Mac)")
	channel := fs.String("channel", "", "where you met them, e.g. twitch")
	note := fs.String("note", "", "anything you want to remember")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*name) == "" || !strings.Contains(*email, "@") {
		return errors.New("--name and a valid --email are needed")
	}
	if err := a.open(); err != nil {
		return err
	}
	p := person{ID: a.st.newID(*name), Name: strings.TrimSpace(*name), Email: strings.TrimSpace(*email), Channel: *channel, Note: *note, Created: time.Now().Format(time.RFC3339)}
	a.st.People = append(a.st.People, p)
	if err := a.st.save(); err != nil {
		return err
	}
	fmt.Printf("added %s (id: %s)\nnext: promo mint %s\n", p.Name, p.ID, p.ID)
	return nil
}

// sync asks the database which of the known tokens got a code.
func (a *app) sync() error {
	rows, err := a.db.query("SELECT t.hash AS hash, COALESCE(c.code, '') AS code FROM tokens t LEFT JOIN codes c ON c.token_hash = t.hash")
	if err != nil {
		return err
	}
	codes := map[string]string{}
	for _, r := range rows {
		codes[str(r["hash"])] = str(r["code"])
	}
	changed := false
	for i := range a.st.People {
		p := &a.st.People[i]
		if p.TokenHash != "" && p.Code == "" && codes[p.TokenHash] != "" {
			p.Code = codes[p.TokenHash]
			p.ClaimedSeen = time.Now().Format(time.RFC3339)
			changed = true
		}
	}
	if changed {
		return a.st.save()
	}
	return nil
}

func (a *app) cmdSync(args []string) error {
	fs := a.flags("sync")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	if err := a.open(); err != nil {
		return err
	}
	if err := a.sync(); err != nil {
		return err
	}
	fmt.Println("up to date")
	return nil
}

func mask(code string) string {
	if len(code) <= 4 {
		return code
	}
	return "…" + code[len(code)-4:]
}

func (a *app) cmdList(args []string) error {
	fs := a.flags("list")
	offline := fs.Bool("offline", false, "do not ask the database, show what was known last time")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	if err := a.open(); err != nil {
		return err
	}
	if !*offline {
		if err := a.sync(); err != nil {
			fmt.Fprintln(os.Stderr, "could not reach the database, showing the last known state:", firstLine(err.Error()))
		}
	}
	if len(a.st.People) == 0 {
		fmt.Println("no one yet: promo add --name ... --email ...")
		return nil
	}
	now := time.Now()
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tCHANNEL\tSTATUS\tLINK ENDS\tCODE")
	for _, p := range a.st.People {
		ends := ""
		if p.ExpiresAt != "" {
			ends = parseTime(p.ExpiresAt).Local().Format("2006-01-02")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", p.ID, p.Name, p.Channel, p.status(now), ends, mask(p.Code))
	}
	return tw.Flush()
}

func (a *app) cmdShow(args []string) error {
	fs := a.flags("show")
	words, err := parse(fs, args)
	if err != nil || len(words) != 1 {
		return errors.New("usage: promo show <id>")
	}
	if err := a.open(); err != nil {
		return err
	}
	p, err := a.st.find(words[0])
	if err != nil {
		return err
	}
	fmt.Printf("%s <%s>\nchannel: %s\nnote: %s\nstatus: %s\n", p.Name, p.Email, p.Channel, p.Note, p.status(time.Now()))
	if p.Token != "" {
		fmt.Printf("link: %s\nminted: %s, ends: %s\nmailed: %s\n", a.link(*p), p.MintedAt, p.ExpiresAt, p.MailedAt)
	}
	if p.Code != "" {
		fmt.Printf("code: %s (shown to them %s)\n", p.Code, p.ClaimedSeen)
	}
	return nil
}

func (a *app) cmdMint(args []string) error {
	fs := a.flags("mint")
	days := fs.Int("days", 7, "how long the link works (1 to 7)")
	force := fs.Bool("force", false, "make a new link even if the person has a live one")
	words, err := parse(fs, args)
	if err != nil || len(words) != 1 {
		return errors.New("usage: promo mint [--days 7] <id>")
	}
	if *days < 1 || *days > 7 {
		return errors.New("--days must be 1 to 7 (the rule that links never outnumber codes counts on at most 7)")
	}
	if err := a.open(); err != nil {
		return err
	}
	p, err := a.st.find(words[0])
	if err != nil {
		return err
	}
	if p.Revoked {
		return errors.New("this person's link was revoked; add them again if you want to start over")
	}
	if err := a.sync(); err != nil {
		return err
	}
	switch s := p.status(time.Now()); s {
	case "claimed":
		return fmt.Errorf("%s already pressed the button and has a code (promo show %s)", p.Name, p.ID)
	case "minted", "mailed":
		if !*force {
			return fmt.Errorf("%s already has a live link (promo show %s); use --force to make another", p.Name, p.ID)
		}
	}

	key, err := a.claimKey()
	if err != nil {
		return err
	}

	free, pending, err := a.capacity()
	if err != nil {
		return err
	}
	if pending+1 > free {
		return fmt.Errorf("not enough codes: %d link(s) are waiting and only %d code(s) have 14+ days left. Import more codes first", pending, free)
	}

	token, err := signToken(key)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(sum[:])
	if !hashRe.MatchString(hash) {
		return errors.New("internal error: bad hash")
	}
	if _, err := a.db.query(fmt.Sprintf("INSERT INTO tokens(hash, expires_at) VALUES (%s, CAST(strftime('%%s','now') AS INTEGER) + %d)", quote(hash), *days*86400)); err != nil {
		return err
	}
	now := time.Now()
	p.Token, p.TokenHash = token, hash
	p.MintedAt, p.ExpiresAt = now.Format(time.RFC3339), now.Add(time.Duration(*days)*24*time.Hour).Format(time.RFC3339)
	p.MailedAt, p.Code, p.ClaimedSeen = "", "", ""
	if err := a.st.save(); err != nil {
		return err
	}
	fmt.Printf("link for %s (works %d days):\n%s\n\nnext: promo mail %s\n", p.Name, *days, a.link(*p), p.ID)
	return nil
}

// capacity returns the codes that still have 14+ days left (7 days for the link plus 7 to use the code)
// and the links that are waiting to be pressed.
func (a *app) capacity() (free, pending int64, err error) {
	rows, err := a.db.query(`SELECT
		(SELECT COUNT(*) FROM codes WHERE token_hash IS NULL AND generated_on >= date('now', '-14 days')) AS free,
		(SELECT COUNT(*) FROM tokens t WHERE t.expires_at > CAST(strftime('%s','now') AS INTEGER)
			AND NOT EXISTS (SELECT 1 FROM codes c WHERE c.token_hash = t.hash)) AS pending`)
	if err != nil || len(rows) != 1 {
		return 0, 0, fmt.Errorf("could not count the codes: %v", err)
	}
	return num(rows[0]["free"]), num(rows[0]["pending"]), nil
}

func (a *app) cmdRevoke(args []string) error {
	fs := a.flags("revoke")
	words, err := parse(fs, args)
	if err != nil || len(words) != 1 {
		return errors.New("usage: promo revoke <id>")
	}
	if err := a.open(); err != nil {
		return err
	}
	p, err := a.st.find(words[0])
	if err != nil {
		return err
	}
	if p.TokenHash == "" {
		return errors.New("this person has no link")
	}
	if err := a.sync(); err != nil {
		return err
	}
	if p.Code != "" {
		return fmt.Errorf("%s was already shown code %s; a shown code stays with them", p.Name, mask(p.Code))
	}
	if _, err := a.db.query(fmt.Sprintf("DELETE FROM tokens WHERE hash = %s AND NOT EXISTS (SELECT 1 FROM codes WHERE token_hash = %s)", quote(p.TokenHash), quote(p.TokenHash))); err != nil {
		return err
	}
	p.Revoked = true
	if err := a.st.save(); err != nil {
		return err
	}
	fmt.Println("link withdrawn")
	return nil
}

func (a *app) cmdCodes(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: promo codes import --generated YYYY-MM-DD <file> | promo codes status")
	}
	switch args[0] {
	case "import":
		return a.codesImport(args[1:])
	case "status":
		return a.codesStatus(args[1:])
	}
	return fmt.Errorf("unknown: promo codes %s", args[0])
}

func (a *app) codesImport(args []string) error {
	fs := a.flags("codes import")
	generated := fs.String("generated", "", "the date App Store Connect shows for this batch (Promo Codes, History), YYYY-MM-DD")
	words, err := parse(fs, args)
	if err != nil || len(words) != 1 || !dateRe.MatchString(*generated) {
		return errors.New("usage: promo codes import --generated YYYY-MM-DD <file with one code per line>")
	}
	day, err := time.Parse("2006-01-02", *generated)
	if err != nil {
		return fmt.Errorf("--generated: %v", err)
	}
	if age := time.Since(day); age > 27*24*time.Hour {
		return errors.New("that batch is almost or fully expired (codes last 4 weeks)")
	} else if age < -24*time.Hour {
		return errors.New("--generated is in the future")
	}
	f, err := os.Open(words[0])
	if err != nil {
		return err
	}
	defer f.Close()
	seen := map[string]bool{}
	var codes []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !codeRe.MatchString(line) {
			return fmt.Errorf("%q does not look like a promo code (letters and digits, 6 to 32)", line)
		}
		if !seen[line] {
			seen[line] = true
			codes = append(codes, line)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if len(codes) == 0 {
		return errors.New("no codes in that file")
	}
	if err := a.open(); err != nil {
		return err
	}
	count := func() (int64, error) {
		rows, err := a.db.query("SELECT COUNT(*) AS n FROM codes")
		if err != nil || len(rows) != 1 {
			return 0, fmt.Errorf("could not count: %v", err)
		}
		return num(rows[0]["n"]), nil
	}
	before, err := count()
	if err != nil {
		return err
	}
	for i := 0; i < len(codes); i += 40 {
		end := min(i+40, len(codes))
		var values []string
		for _, c := range codes[i:end] {
			values = append(values, fmt.Sprintf("(%s, %s)", quote(c), quote(*generated)))
		}
		if _, err := a.db.query("INSERT OR IGNORE INTO codes(code, generated_on) VALUES " + strings.Join(values, ", ")); err != nil {
			return err
		}
	}
	after, err := count()
	if err != nil {
		return err
	}
	fmt.Printf("added %d new code(s); %d were already in the database\n", after-before, int64(len(codes))-(after-before))
	return nil
}

func (a *app) codesStatus(args []string) error {
	fs := a.flags("codes status")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	if err := a.open(); err != nil {
		return err
	}
	rows, err := a.db.query(`SELECT COUNT(*) AS total,
		COALESCE(SUM(token_hash IS NOT NULL), 0) AS assigned,
		COALESCE(SUM(token_hash IS NULL AND generated_on >= date('now', '-21 days')), 0) AS usable,
		COALESCE(SUM(token_hash IS NULL AND generated_on >= date('now', '-14 days')), 0) AS fresh,
		COALESCE(SUM(token_hash IS NULL AND generated_on < date('now', '-21 days')), 0) AS stale
		FROM codes`)
	if err != nil || len(rows) != 1 {
		return fmt.Errorf("could not read the codes: %v", err)
	}
	r := rows[0]
	_, pending, err := a.capacity()
	if err != nil {
		return err
	}
	fresh := num(r["fresh"])
	fmt.Printf("codes in the database:           %d\n  given to a link:               %d\n  free, 7+ days left (can be given): %d\n  free, 14+ days left (counted by mint): %d\n  free but under 7 days left (too late): %d\nlinks waiting to be pressed:      %d\nlinks you can still make:         %d\n",
		num(r["total"]), num(r["assigned"]), num(r["usable"]), fresh, num(r["stale"]), pending, max(fresh-pending, 0))
	return nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}
