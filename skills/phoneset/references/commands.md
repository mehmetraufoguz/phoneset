# phoneset command reference

```
phoneset [serve] [flags]      run in this terminal; Ctrl+C stops it
phoneset daemon [flags]       start in the background and return (tray icon on Windows and macOS)
phoneset status [--json]      what is running, who is connected, what streams
phoneset pair [--browser]     fresh QR code and code for the running service (--browser: open the page)
phoneset stop                 stop the background service
phoneset logs                 log file path and its last 40 lines
phoneset autostart enable|disable|status    start in the background at every login
phoneset audio                list devices and how phoneset would use them
phoneset test [--mic] [--seconds N]         headphone-source level meter, or a quiet tone into the mic cable
phoneset phones               list paired phones
phoneset forget <name|id|all> unpair a phone (put flags before the name)
phoneset update [--check] [--force] [--json]    install the newest release if this build is behind it
phoneset version              version and the commit it was built from, e.g. `phoneset 0.2.0 (3f2a9c1)`
```

## Updating

`phoneset update` compares the commit this binary was built from with the commit of the latest GitHub release
(through GitHub's compare API), not version numbers. If the release is ahead it downloads the archive for this
OS and CPU, checks it against the release's `SHA256SUMS`, runs the new binary once (`version`) and only then
replaces the old one. A background service is stopped and started again with the same flags; a service running
in a terminal is left alone (restart it yourself).

- `--check` only reports; `--json` prints the comparison for scripts; `--force` installs the latest release
  even when the comparison says there is nothing to do (or can't be made, as for a `go run` build).
- A build without a known commit (built outside a git checkout) answers "can't tell"; `--force` or the
  installer fixes that.
- Private repositories: set `GITHUB_TOKEN`, or sign in with `gh auth login` (the GitHub CLI is used as a
  fallback).
- Nothing checks for updates by itself. The only network requests happen when you run `phoneset update` or
  click **Check for updates** in the tray menu.

## Flags (serve and daemon)

| Flag | Meaning |
| --- | --- |
| `--port N` | TCP port, default 47831 |
| `--name NAME` | the name the phone shows for this PC (default: the computer's name) |
| `--mic-device TEXT` | device the phone's microphone is played into: part of its name. Default: auto-detect a virtual cable |
| `--speaker-device TEXT` | device whose audio goes to the phone. Default: the system default playback (Windows), system audio (macOS 14.2+) |
| `--no-mic`, `--no-speaker` | disable one direction |
| `--tray` | show the tray icon while running in a terminal |
| `--no-tray` | no tray icon in the background |
| `--pair` | show pairing even when phones are already paired |
| `--config-dir DIR` | where pairings, log and control file live |
| `--verbose` | debug logging |
| `--quiet`, `--no-open` | (daemon) print nothing / don't open the pairing page |

## Keys in the terminal

`p` pair another phone · `l` list phones · `d` disconnect the phone · `a` audio devices · `h` help · `q` quit

## Tray / menu bar

The icon is the app's icon, with a green dot while a phone is connected. Menu: status, **Pair a phone…**
(opens a local page with the QR code and code), **Disconnect phone**, **Paired phones** (forget one),
**Start at login**, **Check for updates** (a click checks; if a newer build exists the same item becomes
**Install update …**, and a background service restarts itself on the new build), **Open log**, the version,
**Quit phoneset**.

## Files

| | Windows | macOS |
| --- | --- | --- |
| Config folder | `%AppData%\phoneset\` | `~/Library/Application Support/phoneset/` |
| `devices.json` | paired phones and their tokens (owner-only; treat as secrets) | same |
| `identity.json` | this PC's id and name | same |
| `phoneset.log` | background service log (rotates at 2 MB to `.1`) | same |
| `control.json` | port and token of the running instance, deleted on exit | same |
| Start at login | `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` value `phoneset` | `~/Library/LaunchAgents/dev.mehmetraufoguz.phoneset.plist` (+ `~/Library/Logs/phoneset-launchd.log`) |

## Network

TCP 47831 (WebSocket `/v1/ws`, info at `/v1/info`) plus an mDNS service `_phoneset._tcp`. The control API
and pairing page listen on `127.0.0.1` only, on a random port, and need the token from `control.json`.

## Exit and error behaviour

- A second `phoneset` for the same config folder is refused (`already running (PID n)`); the background
  service started by the system exits quietly instead.
- `phoneset status` exits 0 whether or not it is running; use `--json` and read `"running"`.
