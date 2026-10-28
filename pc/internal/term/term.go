// Package term is the small amount of terminal presentation the CLI needs:
// colour, a log handler that reads like a status feed, and a QR renderer.
package term

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"
	xterm "golang.org/x/term"
)

var colour = false

// Init prepares the console (UTF-8 and ANSI on Windows) and decides on colour.
func Init() {
	ok := prepareConsole()
	colour = ok && os.Getenv("NO_COLOR") == "" && xterm.IsTerminal(int(os.Stdout.Fd()))
}

func paint(code, s string) string {
	if !colour {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func Bold(s string) string   { return paint("1", s) }
func Dim(s string) string    { return paint("2", s) }
func Red(s string) string    { return paint("31", s) }
func Green(s string) string  { return paint("32", s) }
func Yellow(s string) string { return paint("33", s) }
func Cyan(s string) string   { return paint("36", s) }

// QR renders content as a block QR code. With colour it always draws dark modules
// on a white field so scanners see the standard polarity on any terminal theme.
func QR(content string) (string, error) {
	q, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return "", err
	}
	if !colour {
		return q.ToSmallString(false), nil
	}
	bm := q.Bitmap()
	var b strings.Builder
	for y := 0; y < len(bm); y += 2 {
		for x := range bm[y] {
			top := bm[y][x]
			bottom := y+1 < len(bm) && bm[y+1][x]
			fg, bg := 97, 107
			if top {
				fg = 30
			}
			if bottom {
				bg = 40
			}
			fmt.Fprintf(&b, "\x1b[%d;%dm▀", fg, bg)
		}
		b.WriteString("\x1b[0m\n")
	}
	return b.String(), nil
}

// Handler is a slog handler that prints "15:04:05  message  key=value".
type Handler struct {
	mu    *sync.Mutex
	out   io.Writer
	level slog.Level
	attrs []slog.Attr
}

func NewHandler(out io.Writer, level slog.Level) *Handler {
	return &Handler{mu: &sync.Mutex{}, out: out, level: level}
}

func (h *Handler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(Dim(r.Time.Format(time.TimeOnly)))
	b.WriteString("  ")
	msg := r.Message
	switch {
	case r.Level >= slog.LevelError:
		msg = Red(msg)
	case r.Level >= slog.LevelWarn:
		msg = Yellow(msg)
	}
	b.WriteString(msg)
	write := func(a slog.Attr) {
		b.WriteString("  ")
		b.WriteString(Dim(a.Key + "="))
		b.WriteString(a.Value.String())
	}
	for _, a := range h.attrs {
		write(a)
	}
	r.Attrs(func(a slog.Attr) bool { write(a); return true })
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.out, b.String())
	return err
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &c
}

func (h *Handler) WithGroup(string) slog.Handler { return h }
