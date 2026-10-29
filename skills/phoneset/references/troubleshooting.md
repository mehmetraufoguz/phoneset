# Troubleshooting

Start with `phoneset status`, `phoneset audio` and `phoneset logs`. Find the symptom below.

## The phone can't find or reach the PC

- Same Wi-Fi? A guest network or "client isolation / AP isolation" blocks phone-to-PC traffic. Try the
  other band or another network.
- Windows firewall: the first run asks to allow phoneset; allow **private networks**. Check with
  `Get-NetFirewallApplicationFilter` or just re-run and accept the prompt. TCP 47831 must be reachable.
- iOS: Settings → phoneset → **Local Network** must be on, or the phone sees nothing.
- mDNS can be blocked by a router. The QR code carries the PC's addresses, so scan it, or type the PC's
  address (shown at start-up and by `phoneset status`) under "Can't see your PC?" in the pairing screen.
- A VPN or virtual adapters can make the PC advertise addresses the phone can't reach; the QR code lists up to
  three addresses and the phone tries them all.

## Pairing fails

- "Too many wrong codes" / "no pairing open": run `phoneset pair` (or press `p`) for a fresh code. An offer
  lasts 5 minutes, works once, and is withdrawn after 5 wrong tries.
- "unknown phone": the PC forgot it (`phoneset forget`, or the config folder was deleted). Pair again.
- "couldn't prove who it is": the PC's pairing was reset; forget the PC in the app and pair again.

## "phoneset is already running"

Only one instance runs per config folder. `phoneset status` shows it; `phoneset stop` ends a background one.
Two copies need `--config-dir` and `--port` to differ.

## No virtual microphone / apps don't hear the phone

- `phoneset audio`: the microphone section must list a cable. If not, install VB-CABLE (Windows) or BlackHole
  2ch (macOS), or name a device with `--mic-device`.
- In the *app that records* (Discord, Teams, OBS, the OS sound settings) choose the other end of the cable:
  **CABLE Output** on Windows, **BlackHole 2ch** on macOS. Choosing the playback side ("CABLE Input") hears
  nothing.
- `phoneset test --mic` plays a quiet 440 Hz tone into the cable; the app's input meter should move.
- "the cable and the headphone source are the same device" means the headphone source resolved to the same
  device as the cable. On Windows that happens when the cable is the default playback device: make the real
  speakers the default, or pass `--speaker-device`.

## Headphones are silent

- Windows: the source is the default playback device and carries audio only while something plays. Play
  something, then `phoneset test`. If the meter moves but the phone is silent, check the headphones switch is on.
  To avoid the PC's own speakers playing too, turn them down or route audio to a second virtual cable and pass
  `--speaker-device`.
- macOS: until the one-time **System Audio Recording** permission is granted the tap delivers silence with no
  error. `phoneset test` while playing something: a flat meter plus "Nothing was captured" means permission.
  System Settings → Privacy & Security → Screen & System Audio Recording → enable the app phoneset runs from,
  then restart that app. Terminal.app declares no system-audio usage description, so macOS may never offer it
  there: run phoneset from iTerm2 or the VS Code terminal, or use the BlackHole route (below).
- macOS before 14.2, or by choice: install BlackHole 16ch, create a Multi-Output Device (your speakers +
  BlackHole 16ch) in Audio MIDI Setup, make it the output, run `phoneset --speaker-device "BlackHole 16ch"`.
  Capturing an input device also needs the Microphone permission for the terminal app.

## The phone disconnects when the PC starts playing audio

That was a bug in app builds before commit 2ffcb66 (an exception in the player ended the connection while the
screen still said "connected"). Rebuild and reinstall the phone app. If it still happens, the app now shows
an error instead; the PC log says why (`phoneset logs`: `reason=connection lost (...)`).

## Echo or feedback

Echo means the phone's microphone hears the phone's own speaker: PC audio plays out of the phone loudly
(headphones on, speaker output) and the microphone sends it straight back to the PC.

- Earphones on the phone remove the cause and always work. Lowering the phone's volume or holding the
  microphone away from the speaker helps too.
- **iPhone:** in the session screen, **Sound → Echo cancellation**. It switches the phone's audio to its
  voice-call mode (voice processing on the audio engine's input), so the PC's audio can sound different while
  it is on. Turning it on or off stops the running microphone and headphones for a moment and starts them again. This is new and has not been
  confirmed on a device yet: if it doesn't remove the echo, say so rather than pretending, and fall back to
  earphones.
- **Android:** not available yet.

## The tray icon is missing

- It lives in the Windows overflow area (the `^` by the clock) until pinned. On macOS it is in the menu bar.
- `phoneset daemon` starts it; a plain `phoneset` in a terminal has none unless started with `--tray`.
- `phoneset logs` shows start-up errors. A build with `CGO_ENABLED=0` on macOS has no tray.

## Start at login does nothing

- `phoneset autostart status`. Windows: the value is under `HKCU\...\Run`; it points at the exe's current
  path, so re-run `autostart enable` after moving the file. macOS: `launchctl print gui/$(id -u)/com.rapp.phoneset`;
  launchd restarts a crashed service but not a clean quit (Quit from the tray stays quit until next login).

## It sounds choppy

Wi-Fi jitter. Move closer or use 5 GHz, and check `phoneset status`/the app for dropped frames. The buffers
(`audio.NewPlayout`, 60 ms) are tuned for normal home Wi-Fi.

## Something else

`phoneset --verbose` in a terminal shows debug logging. For protocol problems read `docs/protocol.md`.
