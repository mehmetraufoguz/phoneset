# Installing phoneset

## PC

### Windows 10 / 11

One line, in PowerShell, no administrator rights. It downloads the latest release, puts `phoneset.exe` in
`%LOCALAPPDATA%\phoneset\bin` and adds that folder to your PATH (open a new terminal afterwards):

```powershell
irm https://raw.githubusercontent.com/mehmetraufoguz/phoneset/main/scripts/install-windows.ps1 | iex
```

With options, which go after the script block:

```powershell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/mehmetraufoguz/phoneset/main/scripts/install-windows.ps1))) -Start -Autostart
```

| Option | Does |
| --- | --- |
| `-Start` | start phoneset in the background right away |
| `-Autostart` | start it in the background at every login |
| `-WithVBCable` | try to install VB-CABLE with winget, else open its download page |
| `-Version vX.Y.Z` | install that release instead of the latest |
| `-NoModifyPath` | leave the PATH alone |
| `-InstallDir DIR` | install somewhere else |
| `-ArchivePath FILE.zip` | install from a local zip, no download |

It handles amd64 and arm64, stops a running phoneset before replacing the file and starts it again.

Windows asks once to allow phoneset through the firewall: allow it on **private networks**. SmartScreen or
Defender may warn about an unsigned program; the binary is built by this repository's CI.

**The virtual microphone:** install VB-CABLE (https://vb-audio.com/Cable/, donationware): unzip, run
`VBCABLE_Setup_x64.exe` as administrator, reboot. Apps then pick **CABLE Output** as their microphone.

### macOS 12+

```sh
curl -fsSL https://raw.githubusercontent.com/mehmetraufoguz/phoneset/main/scripts/install-mac.sh | sh
```

It picks arm64 or amd64, installs to `~/.local/bin`, adds that to `~/.zprofile` once, clears the quarantine
flag, and stops and restarts a running phoneset. Options go after `sh -s --`:

```sh
curl -fsSL https://raw.githubusercontent.com/mehmetraufoguz/phoneset/main/scripts/install-mac.sh | sh -s -- --start --autostart
```

| Option | Does |
| --- | --- |
| `--start` | start phoneset in the background right away |
| `--autostart` | start it in the background at every login (LaunchAgent) |
| `--with-blackhole` | `brew install blackhole-2ch` (asks for your password) |
| `--version vX.Y.Z` | install that release |
| `--no-modify-path` | leave `~/.zprofile` alone |

Environment: `PHONESET_BIN_DIR` (install folder), `PHONESET_TARBALL` (install from a local file).

**The virtual microphone:** `brew install blackhole-2ch` (or https://existential.audio/blackhole/). Apps pick
**BlackHole 2ch** as their microphone.

A private fork answers 404 to both URLs. Use the GitHub CLI instead (`gh auth login` first):

```sh
gh api repos/<owner>/<repo>/contents/scripts/install-mac.sh -H "Accept: application/vnd.github.raw" | sh
```

### From source

Needs Go (the version in `pc/go.mod`; `GOTOOLCHAIN=auto` fetches it).

```sh
cd pc
go build -o phoneset ./cmd/phoneset                         # macOS needs cgo: Xcode command line tools
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o phoneset.exe ./cmd/phoneset   # from any OS
```

A macOS build with `CGO_ENABLED=0` compiles but uses a synthetic development backend (a test tone), not real
audio. Release archives are built by `.github/workflows/release.yml` when a `vX.Y.Z` tag is pushed:
`phoneset-macos-{arm64,amd64}.tar.gz` and `phoneset-windows-{amd64,arm64}.zip`.

## Phone

The app uses native modules (audio engine, mDNS, camera), so it needs a development build; Expo Go can't
run it. From the repository root:

```sh
pnpm install
pnpm mobile:ios         # Xcode, an Apple Developer team for a real device
pnpm mobile:android     # Android Studio / SDK
```

- iOS asks for **Local Network** (to find the PC), **Microphone** and **Camera** (to scan the QR code).
- Check free disk space first: a native iOS build writes several GB.
- The iOS Live Activity (lock screen / Dynamic Island) is off by default because its widget extension needs an
  App Group registered once in the Apple Developer account (`group.com.rapp.phoneset`). Then:
  `PHONESET_LIVE_ACTIVITY=1 pnpm mobile:ios`.
- Android shows a notification while connected (a foreground service) and needs the microphone permission first.

## Updating

```sh
phoneset update --check   # is there something newer? (compares commits, not version numbers)
phoneset update           # download, verify, replace, restart the background service
```

The tray menu has the same thing (**Check for updates**). Running the installer again also works. Rebuild the
phone app for new phone-side features: updating the PC side never touches the phone.

## Uninstalling

```sh
phoneset autostart disable
phoneset stop
phoneset forget all           # optional: unpair every phone
```

Then delete the binary (`%LOCALAPPDATA%\phoneset\bin`, or `~/.local/bin/phoneset`) and the config folder
(`%AppData%\phoneset`, or `~/Library/Application Support/phoneset`). The installers' PATH entry is the only
other trace (`~/.zprofile` line ending in `# phoneset`; the user PATH on Windows).
