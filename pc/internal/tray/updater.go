package tray

import (
	"context"

	"github.com/mehmetraufoguz/phoneset/pc/internal/update"
)

// Updater is what the tray's update item calls. Nil functions hide the item.
type Updater struct {
	Version string // shown at the bottom of the menu, e.g. "0.2.0 (3f2a9c1)"
	Check   func(ctx context.Context) (update.Result, error)
	// Install puts the release in place. restarting is true when the service is about to quit and
	// come back on the new build by itself.
	Install func(ctx context.Context, rel *update.Release) (restarting bool, err error)
}
