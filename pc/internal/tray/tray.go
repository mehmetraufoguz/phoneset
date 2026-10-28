//go:build windows || (darwin && cgo)

// Package tray shows phoneset in the system tray (Windows) or menu bar (macOS): who is
// connected, what is streaming, and the few actions worth a click.
package tray

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"fyne.io/systray"

	"github.com/mehmetraufoguz/phoneset/pc/internal/app"
	"github.com/mehmetraufoguz/phoneset/pc/internal/icons"
	"github.com/mehmetraufoguz/phoneset/pc/internal/system"
	"github.com/mehmetraufoguz/phoneset/pc/internal/update"
)

func init() {
	// The tray's event loop has to run on the main thread (required on macOS).
	runtime.LockOSThread()
}

const maxPhones = 8

// Available reports whether this build can show a tray icon.
func Available() bool { return true }

type Actions struct {
	OpenPairing func()
	Disconnect  func()
	Forget      func(id string)
	Quit        func()
	OpenLog     func()
	Autostart   system.Autostart
	Exe         string
	Update      Updater
}

func icon(connected bool) []byte {
	if runtime.GOOS == "windows" { // Windows wants .ico, the others take PNG
		if connected {
			return icons.TrayConnectedICO
		}
		return icons.TrayIdleICO
	}
	if connected {
		return icons.TrayConnectedPNG
	}
	return icons.TrayIdlePNG
}

// Run shows the icon and blocks until the tray is closed. work runs alongside it; the tray
// closes when work returns or ctx ends.
func Run(ctx context.Context, a *app.App, act Actions, work func() error) error {
	errc := make(chan error, 1)
	systray.Run(func() {
		systray.SetIcon(icon(false))
		systray.SetTooltip("phoneset")

		status := systray.AddMenuItem("Waiting for a phone", "")
		status.Disable()
		mic := systray.AddMenuItem("", "")
		mic.Disable()
		speaker := systray.AddMenuItem("", "")
		speaker.Disable()
		systray.AddSeparator()

		pair := systray.AddMenuItem("Pair a phone…", "Show a QR code to pair a phone")
		disconnect := systray.AddMenuItem("Disconnect phone", "")
		phones := systray.AddMenuItem("Paired phones", "")
		forget := make([]*systray.MenuItem, maxPhones)
		ids := make([]string, maxPhones)
		for i := range forget {
			forget[i] = phones.AddSubMenuItem("", "Unpair this phone")
			forget[i].Hide()
		}
		none := phones.AddSubMenuItem("No phones paired", "")
		none.Disable()
		systray.AddSeparator()

		login := systray.AddMenuItemCheckbox("Start at login", "Run phoneset in the background when you log in", false)
		if on, err := act.Autostart.Enabled(); err == nil && on {
			login.Check()
		}
		var checkItem *systray.MenuItem
		if act.Update.Check != nil {
			checkItem = systray.AddMenuItem("Check for updates", "Look for a newer phoneset")
		}
		logItem := systray.AddMenuItem("Open log", "")
		systray.AddSeparator()
		if act.Update.Version != "" {
			ver := systray.AddMenuItem("phoneset "+act.Update.Version, "")
			ver.Disable()
		}
		quit := systray.AddMenuItem("Quit phoneset", "")

		update := func(s app.Snapshot) {
			connected := s.Phone != nil
			systray.SetIcon(icon(connected))
			if connected {
				status.SetTitle("● " + s.Phone.Name + " is connected")
				systray.SetTooltip("phoneset: " + s.Phone.Name + " is connected")
				disconnect.Show()
				mic.SetTitle(streamLine("Microphone", s.Mic.On, s.Mic.Device))
				speaker.SetTitle(streamLine("Headphones", s.Speaker.On, s.Speaker.Device))
				mic.Show()
				speaker.Show()
			} else {
				status.SetTitle("Waiting for a phone")
				systray.SetTooltip("phoneset: waiting for a phone")
				disconnect.Hide()
				mic.Hide()
				speaker.Hide()
			}
			for i := range forget {
				if i < len(s.Phones) {
					ids[i] = s.Phones[i].ID
					forget[i].SetTitle("Forget " + s.Phones[i].Name)
					forget[i].Show()
				} else {
					forget[i].Hide()
				}
			}
			if len(s.Phones) == 0 {
				none.Show()
			} else {
				none.Hide()
			}
		}
		a.Subscribe(update)

		go func() {
			for {
				select {
				case <-pair.ClickedCh:
					act.OpenPairing()
				case <-disconnect.ClickedCh:
					act.Disconnect()
				case <-logItem.ClickedCh:
					act.OpenLog()
				case <-login.ClickedCh:
					var err error
					if login.Checked() {
						err = act.Autostart.Disable()
					} else {
						err = act.Autostart.Enable(act.Exe)
					}
					if err != nil {
						a.Log.Warn("couldn't change start at login", "err", err)
					} else if login.Checked() {
						login.Uncheck()
					} else {
						login.Check()
					}
				case <-quit.ClickedCh:
					act.Quit()
					return
				}
			}
		}()
		for i := range forget {
			go func(i int) {
				for range forget[i].ClickedCh {
					act.Forget(ids[i])
				}
			}(i)
		}

		if checkItem != nil {
			go updateItem(ctx, a, checkItem, act.Update)
		}

		go func() {
			errc <- work()
			systray.Quit()
		}()
		go func() {
			<-ctx.Done()
			systray.Quit()
		}()
	}, func() {})

	select {
	case err := <-errc:
		return err
	default:
		return nil
	}
}

// updateItem drives the "Check for updates" entry: a click checks; once a newer build is found, the
// next click installs it.
func updateItem(ctx context.Context, a *app.App, item *systray.MenuItem, u Updater) {
	var found *update.Release
	for {
		select {
		case <-ctx.Done():
			return
		case <-item.ClickedCh:
		}
		item.Disable()
		if found != nil {
			item.SetTitle("Installing the update…")
			ictx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			restarting, err := u.Install(ictx, found)
			cancel()
			switch {
			case err != nil:
				a.Log.Warn("update failed", "err", err)
				item.SetTitle("Update failed, try again")
				item.Enable()
			case restarting:
				item.SetTitle("Restarting…")
			default:
				item.SetTitle("Updated: restart phoneset to use it")
				found = nil
			}
			continue
		}
		item.SetTitle("Checking for updates…")
		cctx, cancel := context.WithTimeout(ctx, time.Minute)
		res, err := u.Check(cctx)
		cancel()
		switch {
		case err != nil:
			a.Log.Warn("couldn't check for updates", "err", err)
			item.SetTitle("Couldn't check, try again")
		case res.Status == update.Available && u.Install != nil:
			found = res.Latest
			item.SetTitle("Install update " + res.Latest.Tag + " (" + res.Latest.Short() + ")")
		case res.Status == update.UpToDate:
			item.SetTitle("✓ Up to date")
		case res.Status == update.Ahead:
			item.SetTitle("✓ This build is newer than the release")
		default:
			item.SetTitle("Can't tell if there is an update")
		}
		item.Enable()
	}
}

func streamLine(name string, on bool, device string) string {
	if !on {
		return fmt.Sprintf("   %s: off", name)
	}
	if device == "" {
		return fmt.Sprintf("   %s: on", name)
	}
	return fmt.Sprintf("   %s: on · %s", name, device)
}
