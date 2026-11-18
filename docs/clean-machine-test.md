# Clean-machine test sheet

Written 2026-11-18. Each item below is something the skill (`skills/phoneset/`), the README or the how-to videos says
**without having been run on a clean machine**. Run them, write down what you saw in the "Result" column, and the
text in the skill gets corrected to match. Nothing here needs code changes; it only needs a machine and a phone.

## Before you start

- **A clean machine means a machine without VB-CABLE / BlackHole, without phoneset, and without the camera.** A new
  user account is not clean enough: the virtual cables and the camera are system-wide. Use a spare PC or Mac, or a
  virtual machine (a Windows VM must be on a *bridged* network, or the phone cannot see it over mDNS).
- Have an iPhone with the store app (0.3.x), on the same Wi-Fi, and the camera subscription (or a code) for the camera items.
- For every item write the **exact text** of a message you did not expect. A screenshot is better than a description.
- Reset between runs: `phoneset autostart disable; phoneset camera uninstall; phoneset forget all; phoneset stop`, then
  the removal steps in `skills/phoneset/references/clean-install.md`.

## Windows 10 and 11

| # | Do | Expected today | Result |
| --- | --- | --- | --- |
| W1 | Note `winget --version`. Run the one-line install with `-Start -Autostart -WithVBCable` in a plain PowerShell (no administrator). | messages as in `clean-install.md`; one UAC window for VB-CABLE; a firewall question when phoneset first listens | |
| W2 | **Without restarting**, run `phoneset audio`. | **unknown:** is "CABLE Input" listed as `virtual` with `*`, or only after a restart? (`clean-install.md` says restart) | |
| W3 | Restart. Open Sound settings → Recording. | "CABLE Output" is there | |
| W4 | Pair the phone (the page that opens), switch **Microphone** on, record in an app with "CABLE Output" as the microphone. | your voice is recorded | |
| W5 | In an elevated and a non-elevated terminal, run `phoneset camera install`. | one UAC window; `camera status` says ready; slate in Chrome | |
| W6 | Switch **Use as a webcam** on. Open the camera in: Chrome, Edge, OBS, **Zoom**, **Teams**, the **Windows Camera app**. | Chrome, Edge, OBS were seen before; the rest are unknown | |
| W7 | Repeat W1 to W6 on **Windows 11** (a different machine or VM). | unknown for the camera on Windows 11 | |
| W8 | `Set-ExecutionPolicy Restricted -Scope Process`, then the one-line install. | `irm … \| iex` still works (it is not a script file) | |
| W9 | Remove everything with the steps in `clean-install.md`. Find the firewall rule: what is it called, and where? Try `winget uninstall --id VB-Audio.VBCable` **and** VB-CABLE's own setup "remove" button. | unknown: the rule's name; whether either removal works and whether a restart is needed | |
| W10 | A Windows 10 build **without winget** (an old one, or a VM image), with `-WithVBCable`. | the page https://vb-audio.com/Cable/ opens and the manual text is printed | |

## macOS

| # | Do | Expected today | Result |
| --- | --- | --- | --- |
| M1 | `brew uninstall --cask blackhole-2ch`, restart, `phoneset audio`. Then the install line with `--start --autostart --with-blackhole`. Count the password prompts. | BlackHole 2ch listed as `virtual` with `*` | |
| M2 | **Without restarting** after M1's install, run `phoneset audio` again. | **unknown:** is BlackHole there at once? If not, does `sudo killall coreaudiod` or a restart bring it? (`clean-install.md` says restart if it is missing) | |
| M3 | No Homebrew, without uninstalling it: `env PATH=/usr/bin:/bin:/usr/sbin:/sbin sh -c 'curl -fsSL https://raw.githubusercontent.com/mehmetraufoguz/phoneset/main/scripts/install-mac.sh \| sh -s -- --with-blackhole'` | "Homebrew isn't installed. Get BlackHole 2ch from https://existential.audio/blackhole/" | |
| M4 | **System-audio permission, as a background service** (`phoneset daemon`, and again after a login with start-at-login on). Run `phoneset test` while playing something. | **unknown:** which app does macOS name in System Settings → Privacy & Security → Screen & System Audio Recording (the terminal, `phoneset`, or nothing)? Write the exact name and wording. | |
| M5 | Repeat M4 from Terminal.app, iTerm2 and the VS Code terminal. | Terminal.app is expected to never offer it | |
| M6 | `phoneset camera install` on a Mac that never had it. | **unknown:** the approval wording and place on macOS 26 and on 14 or 15; whether an administrator password is asked | |
| M7 | Switch **Use as a webcam** on. Open the camera in Photo Booth, **FaceTime**, **Zoom**, **Teams**, **Chrome**, QuickTime Player. | Photo Booth was seen; the rest are unknown | |
| M8 | `phoneset camera uninstall`, then check System Settings → Login Items & Extensions → Camera Extensions. | the camera is gone | |
| M9 | Remove everything with `clean-install.md`. Check BlackHole's removal (`brew uninstall --cask blackhole-2ch`, restart). | unknown whether a restart is needed | |

## Both systems

| # | Do | Expected today | Result |
| --- | --- | --- | --- |
| X1 | **A mismatched pair.** Install the old PC program (`-Version v0.1.1` on Windows, `--version v0.1.1` on macOS; protocol 1) and try to connect the 0.3.x phone app. Look at what the phone says, at `phoneset status`, and at the log (also with `phoneset --verbose`). Then `phoneset update`. | a plain connection failure; `handshake failed` only with `--verbose` (inferred, not run) | |
| X2 | **A PC on 0.2.x with the 0.3.x app:** audio works, and the camera switch says "This PC can't receive video. Update phoneset on the PC." | as stated | |
| X3 | A guest network, or a phone on another Wi-Fi band or subnet. | the phone doesn't find the PC; the QR code's addresses and a typed address behave as the skill says | |
| X4 | A VPN on the PC. | the phone may be given an address it cannot reach; the QR carries up to three | |

## After the run

Send the filled "Result" column (and screenshots of anything unexpected). The text to correct for each item:

| Items | Where it is written |
| --- | --- |
| W2, W3, M1 to M3, M9 | `skills/phoneset/references/clean-install.md`, and the how-to videos' narration lines W9, W18 (Windows) and M8, M9, M19 (Mac) |
| M4, M5 | `skills/phoneset/SKILL.md` and `troubleshooting.md` (the system-audio permission), Mac video line M14 |
| M6, M8, W5 | `skills/phoneset/references/camera.md`, Mac video line M17 |
| W6, M7, W7 | `camera.md` (what was seen), the README Status block, `pc/cmd/phoneset/camera.go` (`pickText` names Zoom and Teams) |
| W9, M9 | `clean-install.md`, "Removing everything" |
| X1, X2 | `skills/phoneset/references/diagnose.md`, "The version rule" |
