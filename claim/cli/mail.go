package main

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func firstName(full string) string {
	if f := strings.Fields(full); len(f) > 0 {
		return f[0]
	}
	return full
}

// compose writes the email. It asks for nothing in return: no rating, no review (App Review's rules
// forbid trading free copies for reviews, and a code cannot leave one anyway).
func compose(p person, link string) (subject, body string) {
	subject = "phoneset: a free copy for you"
	body = fmt.Sprintf(`Hi %s,

Thanks for being up for trying phoneset. It turns an iPhone into the microphone and headphones of a Windows PC or a Mac, over your own Wi-Fi. Here is a free copy of the app, no strings attached:

%s

The link opens a page with a button; press it and you get one App Store code to redeem. The link works for 7 days, and the code has its own end date (the page shows it).

The app is half of it: you also need the free phoneset program on your PC, which is one command on Windows:
https://github.com/mehmetraufoguz/phoneset#1-on-the-pc

If anything is unclear or breaks, just reply to this email. I would love to hear how it goes, but you owe me nothing: no review, no rating.

Mehmet
`, firstName(p.Name), link)
	return subject, body
}

func (a *app) cmdMail(args []string) error {
	fs := a.flags("mail")
	sent := fs.Bool("sent", false, "mark it as sent without asking")
	noOpen := fs.Bool("no-open", false, "do not open the mail app")
	noCopy := fs.Bool("no-copy", false, "do not touch the clipboard")
	words, err := parse(fs, args)
	if err != nil || len(words) != 1 {
		return errors.New("usage: promo mail [--sent] <id>")
	}
	if err := a.open(); err != nil {
		return err
	}
	p, err := a.st.find(words[0])
	if err != nil {
		return err
	}
	if err := a.sync(); err != nil {
		return err
	}
	switch s := p.status(time.Now()); s {
	case "new":
		return fmt.Errorf("make the link first: promo mint %s", p.ID)
	case "expired", "revoked":
		return fmt.Errorf("this link is %s; make a new one", s)
	case "claimed":
		return fmt.Errorf("%s already pressed the button, nothing to send", p.Name)
	}
	subject, body := compose(*p, a.link(*p))
	fmt.Printf("To: %s\nSubject: %s\n\n%s\n", p.Email, subject, body)

	if runtime.GOOS == "darwin" {
		if !*noCopy {
			clip := exec.Command("pbcopy")
			clip.Stdin = strings.NewReader(body)
			if err := clip.Run(); err == nil {
				fmt.Println("(the text is on your clipboard)")
			}
		}
		if !*noOpen {
			esc := func(s string) string { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }
			_ = exec.Command("open", "mailto:"+p.Email+"?subject="+esc(subject)+"&body="+esc(body)).Run()
		}
	}
	if !*sent {
		fmt.Print("Did you send it? [y/N] ")
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			fmt.Println("not marked as sent")
			return nil
		}
	}
	p.MailedAt = time.Now().Format(time.RFC3339)
	return a.st.save()
}
