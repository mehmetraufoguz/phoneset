// Package icons embeds the phoneset icon in the sizes the PC side needs.
// The files are drawn by scripts/icons.py, the same script that draws the phone app's icon.
package icons

import _ "embed"

var (
	//go:embed tray-idle.png
	TrayIdlePNG []byte
	//go:embed tray-connected.png
	TrayConnectedPNG []byte
	//go:embed tray-idle.ico
	TrayIdleICO []byte
	//go:embed tray-connected.ico
	TrayConnectedICO []byte
	//go:embed logo.png
	LogoPNG []byte
)
