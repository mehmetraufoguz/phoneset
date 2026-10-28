---
name: phoneset
description: Install, set up, run and troubleshoot phoneset, which turns a phone (iPhone or Android) into a Windows or Mac's microphone and headphones over Wi-Fi. Use when the user wants a phone to work as a PC microphone or as headphones, wants to install, update or pair phoneset, needs to pick audio devices (VB-CABLE, BlackHole), wants it running in the background, in the tray or at login, or is debugging audio, echo, pairing or connection problems with it.
license: MIT
---

# phoneset

phoneset has two halves. A small Go program (`phoneset`, terminal only, optional tray icon) runs on the
PC. An Expo app runs on the phone. They pair once (QR code or 6-digit code), then talk over Wi-Fi with an
encrypted connection:

- **Phone microphone → PC.** The PC plays the phone's voice into a *virtual audio cable*; apps record from
  the cable's other end. A virtual cable is required: **VB-CABLE** on Windows, **BlackHole 2ch** on macOS.
- **PC audio → phone headphones.** The PC captures what it plays (Windows loopback; on macOS 14.2+ a
  system-audio tap) and sends it to the phone.

Repository: https://github.com/mehmetraufoguz/phoneset. More detail: `references/install.md`,
`references/commands.md`, `references/troubleshooting.md`, `references/how-it-works.md`.

## When helping someone

1. **Find out the situation first**: which OS the PC is (Windows / macOS), whether phoneset is already
   installed (`phoneset version`), and whether the phone app is installed. Don't assume.
2. **Ask before anything that changes the machine beyond a user folder**: installing a virtual-audio driver
   (VB-CABLE needs administrator rights and a reboot; BlackHole asks for the password), enabling
   start-at-login, and editing the shell profile (the Mac installer adds `~/.local/bin` to `~/.zprofile`
   once). Everything else the installers do stays in the user's own folders.
3. **Verify each step with a command, don't guess**: `phoneset version`, `phoneset audio`, `phoneset status`,
   `phoneset test`. Report what they print.

## Install on the PC

Windows 10/11 (PowerShell, no administrator rights):

```powershell
irm https://raw.githubusercontent.com/mehmetraufoguz/phoneset/main/scripts/install-windows.ps1 | iex
```

macOS 12+ (needs the Xcode command line tools only to build from source, not for this):

```sh
curl -fsSL https://raw.githubusercontent.com/mehmetraufoguz/phoneset/main/scripts/install-mac.sh | sh
```

Options (`-Start -Autostart -WithVBCable -Version vX.Y.Z` on Windows; `--start --autostart --with-blackhole
--version vX.Y.Z` on macOS) and the manual / from-source routes are in `references/install.md`.

Then install the virtual cable (VB-CABLE from https://vb-audio.com/Cable/ on Windows,
`brew install blackhole-2ch` on macOS) and check phoneset sees it:

```sh
phoneset audio      # the microphone target should list the cable as "chosen automatically"
```

## Run and pair

```sh
phoneset            # in this terminal: prints a QR code and a code; Ctrl+C stops it
phoneset daemon     # or in the background, with a tray icon (opens a pairing page the first time)
phoneset status     # who is connected, what is streaming
phoneset pair       # a fresh QR code and code for the running service
phoneset update     # install the newest release (add --check to only look)
```

On the phone: open the app, tap **+** (top right), scan the QR code or type the code, then tap the PC and
switch **Microphone** and **Headphones** on. In the apps that should hear the phone, choose the other end of
the cable as the microphone: **CABLE Output** (Windows) or **BlackHole 2ch** (macOS).

The phone app is not on an app store. It is built from this repository (`pnpm install`, then
`pnpm mobile:ios` or `pnpm mobile:android`); see `references/install.md`.

## Quick checks

| Want to know | Run |
| --- | --- |
| Is it installed, which version | `phoneset version` |
| Which devices it would use | `phoneset audio` |
| Does the headphone source capture audio | `phoneset test` (play something on the PC; the meter should move) |
| Does the virtual microphone work | `phoneset test --mic` (a quiet tone; watch the cable's input level in an app) |
| Is the service running, who is connected | `phoneset status` (`--json` for scripts) |
| What went wrong | `phoneset logs` (background service) |

## Things that are easy to get wrong

- The microphone cable and the headphone source can't be the same device: the phone would hear its own voice
  back. phoneset refuses that and says so. Don't make the cable the Windows default playback device.
- On Windows the headphone source is the default playback device and only carries audio while something is
  playing. Silence on the phone with nothing playing is normal.
- On macOS the system-audio tap needs a one-time permission, and without it the audio is *silent with no
  error*. `phoneset test` shows it. Terminal.app can't be granted it (it declares no usage description);
  iTerm2 or the VS Code terminal can, or use BlackHole 16ch (`references/troubleshooting.md`).
- The phone's speaker + microphone at once echoes back to the PC. Use earphones; see the echo section of
  `references/troubleshooting.md`.
- Phone and PC must be on the same Wi-Fi, and not a guest network with client isolation.
