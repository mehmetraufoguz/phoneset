# A clean install, and removing everything

For a machine that has never had phoneset, for a start from zero after something went wrong, and for taking it all off.
Ask before every step that changes the machine (see the rules in `SKILL.md`).

## Requirements

| | Needs | Notes |
| --- | --- | --- |
| Windows PC | Windows 10 or 11 | amd64 is what was run (Windows 10 build 19045); arm64 is only compiled by CI |
| Mac | macOS 12 or later | Apple silicon is what was run; Intel is only compiled by CI. The system-audio tap (headphones) needs **14.2**, the camera **12.3**; before 14.2 use BlackHole 16ch (`troubleshooting.md`) |
| iPhone | iOS 16.4 or later | the project's deployment target |
| Android | Android 7.0 or later (`minSdk 24`, targets 36) | closed test on Google Play, not public yet |
| Network | phone and PC on the same Wi-Fi | not a guest network; TCP 47831 and mDNS (`_phoneset._tcp`) must pass |
| Virtual cable | VB-CABLE (Windows), BlackHole 2ch (macOS) | for the microphone; the headphones and the camera need none |
| Camera | PC program 0.3.0 or newer | `phoneset camera install`; verified on Windows 10 and one Mac only (`camera.md`) |
| Versions | the phone app and the PC program on the same protocol (0.2.0 and later are protocol 2) | `diagnose.md`, "The version rule" |

Verified together: Windows 10 with an iPhone 13 Pro (microphone through VB-CABLE, headphones through loopback), and an
Apple silicon Mac (macOS 26). Anything else is expected to work but was not run by the author.

## A clean install, with what each step prints

Open a new terminal after step 1 so the PATH change is picked up.

### Windows 10 / 11

1. PowerShell, no administrator rights, with the options that make a fresh PC usable:

   ```powershell
   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/mehmetraufoguz/phoneset/main/scripts/install-windows.ps1))) -Start -Autostart -WithVBCable
   ```

   Expect, in order: `Downloading phoneset (latest) for Windows amd64...`, `Installed phoneset <version> (<commit>) to
   %LOCALAPPDATA%\phoneset\bin\phoneset.exe`, `Added … to your PATH (open a new terminal window to pick it up).`, then
   `Installing VB-CABLE with winget (Windows will ask for permission)...` (a UAC window: yes), then the "Next:" hints.
2. **Allow the firewall question** on *private networks* (Windows asks the first time it runs; the installer adds no
   rule itself).
3. **Restart the PC** after VB-CABLE (VB-Audio's own instruction; whether winget's install avoids the restart was not tested).
4. New terminal: `phoneset version`, `phoneset audio` (a device marked `virtual` with `*` under the microphone), `phoneset status`.
5. A pairing page opens in the browser the first time; scan it in the app (**+**).

### macOS

1. Terminal:

   ```sh
   curl -fsSL https://raw.githubusercontent.com/mehmetraufoguz/phoneset/main/scripts/install-mac.sh | sh -s -- --start --autostart --with-blackhole
   ```

   Expect `Downloading phoneset (latest) for macOS arm64...`, `Installed phoneset <version> (<commit>) to
   ~/.local/bin/phoneset`, `Added ~/.local/bin to your PATH in ~/.zprofile (open a new terminal window to pick it up).`,
   then `Installing BlackHole 2ch (macOS will ask for your password)...`.
2. Allow incoming connections if macOS asks. If `phoneset audio` doesn't list BlackHole 2ch, restart the Mac (whether a
   restart is needed after the install on a fresh Mac was not tested).
3. New terminal: `phoneset version`, `phoneset audio`, `phoneset status`. A pairing page opens the first time.
4. For the headphones, `phoneset test` while playing something; grant the system-audio permission if it says "Nothing
   was captured" (`troubleshooting.md`).

### What the installers change

| | Windows | macOS |
| --- | --- | --- |
| Program | `%LOCALAPPDATA%\phoneset\bin\phoneset.exe` | `~/.local/bin/phoneset` |
| PATH | the folder is added to the user PATH | a line ending in `# phoneset` is added to `~/.zprofile` |
| Start at login (only with the option) | `HKCU\…\Run` value `phoneset` | `~/Library/LaunchAgents/com.rapp.phoneset.plist` |
| Virtual cable (only with the option) | VB-CABLE through winget | BlackHole 2ch through Homebrew |
| Elsewhere | nothing; no firewall rule, no service | nothing; the quarantine flag is cleared on the program |

### When a prerequisite is missing

- **No winget** (older Windows 10 or no App Installer): the installer prints "Get VB-CABLE from https://vb-audio.com/Cable/
  (unzip, run VBCABLE_Setup_x64.exe as administrator, reboot)." and opens the page. Do that by hand.
- **No Homebrew**: the installer prints "Homebrew isn't installed. Get BlackHole 2ch from
  https://existential.audio/blackhole/". Download the 2ch installer there.
- `brew install` fails: the installer prints "BlackHole didn't install; see https://existential.audio/blackhole/".
- **The download fails** (a proxy, a firewall, or a private fork that answers 404): the Windows script says so and
  suggests the GitHub CLI (`gh auth login`); the GitHub CLI route is in `install.md`.
- **A managed or locked-down PC** (policies that block PowerShell, installers or drivers): not tried. VB-CABLE and
  BlackHole are kernel or audio drivers and may need an administrator; the camera needs one too.
- **PowerShell execution policy:** the one-line install passes the script as text (`irm … | iex`), not as a file, so
  the policy for script files should not apply; this was not tried on a locked-down PC.

## Starting over without removing everything

```sh
phoneset stop
phoneset forget all          # unpair every phone; also tap Forget on the phone's Home screen
phoneset                     # or: phoneset daemon, then pair again
```

To also reset the settings, stop it and delete the config folder (`%AppData%\phoneset` or
`~/Library/Application Support/phoneset`); it holds the pairings, the log and the PC's identity.

## Removing everything

1. `phoneset autostart disable`, `phoneset camera uninstall` (Windows: close a program that holds the camera first),
   `phoneset forget all`, `phoneset stop`.
2. **Windows:** delete the program folder and the config folder, and take the folder off the user PATH:

   ```powershell
   $dir = "$env:LOCALAPPDATA\phoneset\bin"
   $p = [Environment]::GetEnvironmentVariable("Path","User") -split ";" | Where-Object { $_ -and $_ -ne $dir }
   [Environment]::SetEnvironmentVariable("Path", ($p -join ";"), "User")
   Remove-Item -Recurse -Force "$env:LOCALAPPDATA\phoneset", "$env:APPDATA\phoneset"
   ```

   The firewall: Windows made an allow rule when the question was answered; remove it under Windows Security → Firewall &
   network protection → Allow an app through firewall (not tried).
3. **macOS:** 

   ```sh
   rm -f ~/.local/bin/phoneset
   rm -rf ~/Library/Application\ Support/phoneset ~/Library/Logs/phoneset-launchd.log
   ```

   and delete the line ending in `# phoneset` from `~/.zprofile`. The permission entry you granted (System Settings →
   Privacy & Security → Screen & System Audio Recording) stays until you remove it there.
4. **The virtual cable is a separate product**; phoneset does not remove it. VB-CABLE: run `VBCABLE_Setup_x64.exe` as
   administrator and use its remove option, then restart (VB-Audio's own instructions; `winget uninstall --id
   VB-Audio.VBCable` may also work; neither was run here). BlackHole: `brew uninstall --cask blackhole-2ch`, then restart
   (BlackHole's own instructions; not run here). Leave the cable alone if another program uses it.
5. **The phone:** tap Forget next to the PC on Home, delete the app. The subscription is the store's: cancel it in the
   App Store or Google Play subscriptions, deleting the app does not.
