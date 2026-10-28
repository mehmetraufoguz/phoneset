//go:build !(windows || (darwin && cgo))

// Package tray shows phoneset in the system tray. This build has none.
package tray

import (
	"context"
	"errors"

	"github.com/mehmetraufoguz/phoneset/pc/internal/app"
	"github.com/mehmetraufoguz/phoneset/pc/internal/system"
)

var ErrUnavailable = errors.New("the tray icon isn't available in this build")

func Available() bool { return false }

type Actions struct {
	OpenPairing func()
	Disconnect  func()
	Forget      func(id string)
	Quit        func()
	OpenLog     func()
	Autostart   system.Autostart
	Exe         string
}

func Run(context.Context, *app.App, Actions, func() error) error { return ErrUnavailable }
