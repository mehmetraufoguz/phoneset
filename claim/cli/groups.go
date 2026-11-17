package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Lists ("groups") are plain names kept on each person in people.json, on this Mac only. One list can feed
// several apps: Play's closed-test testers, the people for an X giveaway, a beta group for another app.
// A person can be on any number of lists, and the claim database never hears of them.

var groupRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

func parseGroups(v string) ([]string, error) {
	var out []string
	for _, g := range strings.Split(v, ",") {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if !groupRe.MatchString(g) {
			return nil, fmt.Errorf("list names are lowercase letters, digits and dashes (got %q)", g)
		}
		out = append(out, g)
	}
	return out, nil
}

// inGroup returns everyone when group is empty.
func (s *store) inGroup(group string) []person {
	if group == "" {
		return s.People
	}
	var out []person
	for _, p := range s.People {
		if p.inGroup(group) {
			out = append(out, p)
		}
	}
	return out
}

func addGroup(p *person, g string) bool {
	if p.inGroup(g) {
		return false
	}
	p.Groups = append(p.Groups, g)
	sort.Strings(p.Groups)
	return true
}

func removeGroup(p *person, g string) bool {
	for i, x := range p.Groups {
		if x == g {
			p.Groups = append(p.Groups[:i], p.Groups[i+1:]...)
			return true
		}
	}
	return false
}

func (s *store) byEmail(email string) *person {
	for i := range s.People {
		if strings.EqualFold(s.People[i].Email, email) {
			return &s.People[i]
		}
	}
	return nil
}

const groupUsage = `usage:
  promo group ls                          the lists and how many people are on each
  promo group add <list> <id>...          put people on a list
  promo group rm <list> <id>...           take people off a list
  promo group import <list> <file.csv>    add people from a CSV (columns: name,email[,channel[,note]]); known emails are only put on the list
  promo group export <list> [--format emails|csv]   print the list (emails: one per line, for Play's tester upload; csv: name,email,channel)
`

func (a *app) cmdGroup(args []string) error {
	if len(args) == 0 {
		return errors.New(groupUsage)
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "ls":
		return a.groupLs()
	case "add", "rm":
		return a.groupAddRm(sub, rest)
	case "import":
		return a.groupImport(rest)
	case "export":
		return a.groupExport(rest)
	}
	return errors.New(groupUsage)
}

func (a *app) groupLs() error {
	if err := a.open(); err != nil {
		return err
	}
	counts := map[string]int{}
	for _, p := range a.st.People {
		for _, g := range p.Groups {
			counts[g]++
		}
	}
	if len(counts) == 0 {
		fmt.Println("no lists yet: promo add --group play-testers ... or promo group import play-testers people.csv")
		return nil
	}
	names := make([]string, 0, len(counts))
	for g := range counts {
		names = append(names, g)
	}
	sort.Strings(names)
	for _, g := range names {
		fmt.Printf("%-24s %d\n", g, counts[g])
	}
	return nil
}

func (a *app) groupAddRm(sub string, args []string) error {
	fs := a.flags("group " + sub)
	words, err := parse(fs, args)
	if err != nil || len(words) < 2 {
		return errors.New(groupUsage)
	}
	groups, err := parseGroups(words[0])
	if err != nil || len(groups) != 1 {
		return errors.New("give one list name")
	}
	g := groups[0]
	if err := a.open(); err != nil {
		return err
	}
	for _, id := range words[1:] {
		p, err := a.st.find(id)
		if err != nil {
			return err
		}
		changed := false
		if sub == "add" {
			changed = addGroup(p, g)
		} else {
			changed = removeGroup(p, g)
		}
		fmt.Printf("%s: %s %s\n", p.ID, map[bool]string{true: sub, false: "no change for"}[changed], g)
	}
	return a.st.save()
}

func (a *app) groupImport(args []string) error {
	fs := a.flags("group import")
	words, err := parse(fs, args)
	if err != nil || len(words) != 2 {
		return errors.New(groupUsage)
	}
	groups, err := parseGroups(words[0])
	if err != nil || len(groups) != 1 {
		return errors.New("give one list name")
	}
	g := groups[0]
	f, err := os.Open(words[1])
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	if err := a.open(); err != nil {
		return err
	}
	added, listed, line := 0, 0, 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		line++
		for len(rec) < 4 {
			rec = append(rec, "")
		}
		name, email := strings.TrimSpace(rec[0]), strings.TrimSpace(rec[1])
		if line == 1 && strings.EqualFold(email, "email") { // a header row
			continue
		}
		if !strings.Contains(email, "@") {
			return fmt.Errorf("line %d: %q is not an email (columns: name,email[,channel[,note]])", line, email)
		}
		if name == "" {
			name = strings.SplitN(email, "@", 2)[0]
		}
		p := a.st.byEmail(email)
		if p == nil {
			a.st.People = append(a.st.People, person{ID: a.st.newID(name), Name: name, Email: email, Channel: strings.TrimSpace(rec[2]), Note: strings.TrimSpace(rec[3]), Created: time.Now().Format(time.RFC3339)})
			p = &a.st.People[len(a.st.People)-1]
			added++
		}
		if addGroup(p, g) {
			listed++
		}
	}
	if err := a.st.save(); err != nil {
		return err
	}
	fmt.Printf("%d new people, %d put on %s (now %d on the list)\n", added, listed, g, len(a.st.inGroup(g)))
	return nil
}

func (a *app) groupExport(args []string) error {
	fs := a.flags("group export")
	format := fs.String("format", "emails", "emails (one per line) or csv")
	words, err := parse(fs, args)
	if err != nil || len(words) != 1 {
		return errors.New(groupUsage)
	}
	if *format != "emails" && *format != "csv" {
		return errors.New("--format must be emails or csv")
	}
	if err := a.open(); err != nil {
		return err
	}
	people := a.st.inGroup(words[0])
	if len(people) == 0 {
		return fmt.Errorf("no one on the list %q (see: promo group ls)", words[0])
	}
	w := csv.NewWriter(os.Stdout)
	for _, p := range people {
		if *format == "emails" {
			fmt.Println(p.Email)
		} else {
			w.Write([]string{p.Name, p.Email, p.Channel})
		}
	}
	w.Flush()
	if *format == "emails" {
		fmt.Fprintf(os.Stderr, "%d email(s)\n", len(people))
	}
	return w.Error()
}
