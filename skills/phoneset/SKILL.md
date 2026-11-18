---
name: phoneset
description: Install, set up, run and troubleshoot phoneset, which turns a phone (iPhone or Android) into a Windows or Mac's microphone, headphones and webcam over Wi-Fi. Use when the user wants a phone to work as a PC microphone, as headphones or as a webcam (the virtual camera "phoneset Camera"), wants to install, update or pair phoneset, needs to pick audio devices (VB-CABLE, BlackHole), wants it running in the background, in the tray or at login, or is debugging audio, camera, echo, pairing or connection problems with it.
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
- **Phone camera → PC webcam.** The phone films, the PC program receives the picture, and a small native piece
  shows it as a camera called **phoneset Camera** (macOS: a camera extension; Windows: a DirectShow camera).
  It is installed on demand with `phoneset camera install`. Details: `references/camera.md`.

Repository: https://github.com/mehmetraufoguz/phoneset. More detail: `references/install.md`,
`references/commands.md`, `references/camera.md`, `references/troubleshooting.md`, `references/how-it-works.md`.

## When helping someone

1. **Find out the situation first**: which OS the PC is (Windows / macOS), whether phoneset is already
   installed (`phoneset version`), and whether the phone app is installed. Don't assume.
2. **Ask before anything that changes the machine beyond a user folder**: installing a virtual-audio driver
   (VB-CABLE needs administrator rights and a reboot; BlackHole asks for the password), installing the virtual
   camera (Windows shows a UAC window; macOS may need an approval in System Settings), enabling
   start-at-login, and editing the shell profile (the Mac installer adds `~/.local/bin` to `~/.zprofile`
   once). Everything else the installers do stays in the user's own folders.
3. **Verify each step with a command, don't guess**: `phoneset version`, `phoneset audio`, `phoneset status`,
   `phoneset test`, `phoneset camera status`. Report what they print.

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
--version vX.Y.Z` on macOS) and the manual / from-source routes are in `references/install.md`. Open a new
terminal afterwards: the installers add `phoneset` to the PATH.

Then install the virtual cable (VB-CABLE from https://vb-audio.com/Cable/ on Windows,
`brew install blackhole-2ch` on macOS) and check phoneset sees it:

```sh
phoneset audio      # the microphone target should list the cable as "chosen automatically"
```

For the webcam, once, in a new terminal (needs a release that carries the camera, v0.3.0 or newer):

```sh
phoneset camera install     # Windows: one UAC window; macOS: maybe an approval in System Settings
phoneset camera status      # "ready" when programs can list "phoneset Camera"
```

## Run and pair

```sh
phoneset            # in this terminal: prints a QR code and a code; Ctrl+C stops it
phoneset daemon     # or in the background, with a tray icon (opens a pairing page the first time)
phoneset status     # who is connected, what is streaming, the camera
phoneset pair       # a fresh QR code and code for the running service
phoneset update     # install the newest release (add --check to only look)
```

On the phone: open the app, tap **+** (top right), scan the QR code or type the code, then tap the PC and
switch **Microphone** and **Headphones** on. In the apps that should hear the phone, choose the other end of
the cable as the microphone: **CABLE Output** (Windows) or **BlackHole 2ch** (macOS). For the webcam, switch
**Use as a webcam** on (Camera section; Back/Front, Quality, Format and Smoothness are there too) and choose
**phoneset Camera** in the video app.

## The phone app

- **iPhone:** on the App Store, https://apps.apple.com/app/id6828815833. It is a paid app (one price); the
  microphone and the headphones need nothing more. **The camera is an auto-renewing subscription inside the
  app** (monthly or six months; the store shows the price). The app also has **Restore purchases**, **Redeem a
  code** and **Manage subscription**.
- **Android:** not public yet (a closed test on Google Play). Until then it is built from this repository
  (`pnpm install`, then `pnpm mobile:android`); see `references/install.md`.
- The app's version follows the PC program's (`phoneset version`).

## Quick checks

| Want to know | Run |
| --- | --- |
| Is it installed, which version | `phoneset version` |
| Which devices it would use | `phoneset audio` |
| Does the headphone source capture audio | `phoneset test` (play something on the PC; the meter should move) |
| Does the virtual microphone work | `phoneset test --mic` (a quiet tone; watch the cable's input level in an app) |
| Is the service running, who is connected | `phoneset status` (`--json` for scripts) |
| Is the virtual camera in place | `phoneset camera status` |
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
- **Camera:** the camera is only a *picture* path. "phoneset Camera" shows a "Waiting for your phone" slate until
  the phone's **Use as a webcam** switch is on and a picture arrives. A browser or app that was already open when
  the camera was installed has to be restarted to list it. Which programs were really seen with it
  (Chrome, Edge and OBS on Windows 10; Photo Booth on a Mac) and what is not verified: `references/camera.md`.
- The camera switch needs the subscription (or a redeemed code). Audio never does.
