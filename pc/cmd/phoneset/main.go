// Command phoneset turns a phone into this PC's microphone and headphones over Wi-Fi.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mehmetraufoguz/phoneset/pc/internal/store"
	"github.com/mehmetraufoguz/phoneset/pc/internal/term"
	"github.com/mehmetraufoguz/phoneset/pc/internal/update"
)

var version = "dev"

const usage = `phoneset - use your phone as this PC's microphone and headphones over Wi-Fi

Run it in a terminal:
  phoneset [serve] [flags]   start here, show the pairing QR code, stop with Ctrl+C

Or in the background (tray icon on Windows and macOS):
  phoneset daemon            start in the background, then give the terminal back
  phoneset status            what is running, who is connected
  phoneset pair              show a pairing QR code and code for the running service
  phoneset stop              stop the background service
  phoneset logs              where the log is, and its last lines
  phoneset autostart enable  start in the background at every login (or: disable, status)
  phoneset update            install the newest release if this build's commit is behind it
                             (--check only looks; --force reinstalls; --json for scripts)

Other:
  phoneset audio             list audio devices and how phoneset would use them
  phoneset test              check the headphone source (level meter) or --mic (tone)
  phoneset phones            list paired phones
  phoneset forget <name|id|all>
  phoneset version

Flags for serve and daemon:
  --port N              TCP port (default 47831)
  --name NAME           name shown on the phone (default: this PC's name)
  --mic-device TEXT     device the phone's microphone is played into, a virtual
                        cable (default: auto-detect VB-CABLE or BlackHole)
  --speaker-device TEXT device whose audio is sent to the phone (default: the system
                        default playback on Windows; system audio on macOS)
  --no-mic, --no-speaker
  --tray                show the tray icon while running in the terminal
  --no-tray             no tray icon in the background
  --pair                open pairing even when phones are already paired
  --config-dir DIR      where pairings are stored
  --verbose
`

func main() {
	term.Init()
	update.CleanupOld() // the previous executable, left behind by a Windows update
	cmd, args := "serve", os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = cmdServe(args)
	case "daemon", "start":
		err = cmdDaemon(args)
	case "stop":
		err = cmdStop(args)
	case "status":
		err = cmdStatus(args)
	case "pair":
		err = cmdPair(args)
	case "logs":
		err = cmdLogs(args)
	case "autostart":
		err = cmdAutostart(args)
	case "update":
		err = cmdUpdate(args)
	case "relaunch": // internal: started by a service that has just updated itself
		err = cmdRelaunch(args)
	case "audio":
		err = cmdAudio()
	case "test":
		err = cmdTest(args)
	case "phones":
		err = cmdPhones(args)
	case "forget":
		err = cmdForget(args)
	case "version":
		fmt.Println("phoneset", buildLabel(version, currentBuild().Commit))
	case "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, term.Red("error:"), err)
		os.Exit(1)
	}
}

// configDir is where pairings, the log and the control file live.
func configDir(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	dir, err := store.DefaultDir()
	if err != nil {
		return "."
	}
	return dir
}

func openStore(dir string) (*store.Store, error) { return store.Open(configDir(dir)) }
