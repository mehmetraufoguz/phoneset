# phoneset

Use your phone as your PC's **microphone** and **headphones** over Wi-Fi.

```
 phone mic ──Wi-Fi──▶ phoneset (PC, terminal) ──▶ virtual cable ──▶ any app's microphone
 phone speaker ◀──Wi-Fi── phoneset (PC, terminal) ◀── what the PC is playing
```

|                    |                                                                                                              |
| ------------------ | ------------------------------------------------------------------------------------------------------------ |
| `pc/`              | The PC side. A Go program you run in a terminal, no window. Windows and macOS.                               |
| `mobile/`          | The phone app. Expo (SwiftUI on iOS, Jetpack Compose on Android through Expo UI), logic written with Effect. |
| `docs/protocol.md` | The wire protocol and its security model.                                                                    |

## Using it

### 1. On the PC

#### Windows 10/11

Get `phoneset.exe` from the CI artifacts (the **ci** workflow, `phoneset-windows`) or build it:

```sh
pnpm pc:build:windows        # from a Mac/Linux box
cd pc && go build -o phoneset.exe ./cmd/phoneset   # on Windows
```

Install **[VB-CABLE](https://vb-audio.com/Cable/)**. Windows can't create a microphone
out of thin air, so phoneset plays your phone's voice into the cable's _CABLE Input_,
and apps record from its other end, _CABLE Output_. Pick **CABLE Output** as the
microphone in Discord, Teams, OBS, Windows Sound settings, and so on.

Then run it:

```
phoneset
```

It prints a QR code and a 6-digit code. Allow it through the Windows firewall for
_private networks_ the first time (it listens on TCP 47831 and announces itself over mDNS).

Typing in the window: `p` pair another phone, `l` list phones, `a` audio devices, `q` quit.

**Headphones.** phoneset captures what a playback device is playing and streams it to
the phone (WASAPI loopback). By default that is the system default device, so the PC's
own speakers keep playing too. Either turn the PC speakers down (on most drivers the
loopback tap ignores the speaker volume, but that depends on the driver) or send audio to
a second virtual cable and capture that: `phoneset --speaker-device "CABLE-A Input"`. The
device that carries the phone's microphone can't also be the headphone source, because that
would echo your voice back to you; phoneset refuses that combination and says so.

```
phoneset audio                       list devices and how phoneset would use them
phoneset test                        level meter for what would be sent to the phone
phoneset test --mic                  quiet tone into the microphone cable, to check it
phoneset phones                      list paired phones
phoneset forget <name|id|all>
phoneset --mic-device "CABLE"        which device carries the phone's microphone
phoneset --speaker-device "Speakers" which device is sent to the phone
phoneset --no-mic | --no-speaker
phoneset --pair                      show the pairing QR even if phones are paired
phoneset --port 47831 --name "Desk PC" --config-dir DIR --verbose
```

Pairings are stored in `%AppData%\phoneset\`.

#### macOS (12 or later)

Build it on the Mac (macOS audio needs cgo, so the Xcode command line tools must be
installed), or take `phoneset-macos-arm64` / `phoneset-macos-amd64` from the CI artifacts
(`chmod +x phoneset` and `xattr -d com.apple.quarantine phoneset` for a downloaded one):

```sh
pnpm pc:build        # writes pc/bin/phoneset
```

Install **[BlackHole 2ch](https://existential.audio/blackhole/)** (`brew install blackhole-2ch`).
macOS can't create a microphone out of thin air either: phoneset plays your phone's voice
into BlackHole's output, and apps pick **BlackHole 2ch** as their microphone (Zoom, Discord,
OBS, QuickTime, System Settings > Sound).

```
phoneset
```

It prints the same QR code and 6-digit code. Allow incoming connections if macOS asks.

**Headphones.** On macOS 14.2 or later phoneset taps the system audio, so no second driver
is needed. The Mac's own speakers keep playing, and phoneset leaves its own output out of
the tap so your voice never comes back to you. macOS asks for permission once, and until you
allow it, it hands over silence without any error:

1. Run `phoneset test` and play something on the Mac. The meter should move.
2. If it stays silent and says "Nothing was captured", open System Settings > Privacy &
   Security > Screen & System Audio Recording, switch on the app you run phoneset from
   (Terminal, iTerm...), quit and reopen it, and run `phoneset test` again.

On older macOS, or if you would rather route it yourself, capture a virtual input instead:
install BlackHole 16ch, make a Multi-Output Device in Audio MIDI Setup with your speakers
and BlackHole 16ch, set it as the system output, and run
`phoneset --speaker-device "BlackHole 16ch"`. Capturing an input device also needs the
Microphone permission for your terminal.

Pairings are stored in `~/Library/Application Support/phoneset/`.

### 2. On the phone

The app uses native modules (audio engine, mDNS), so it runs as a development build, not
in Expo Go.

```sh
pnpm install
pnpm mobile:ios        # needs Xcode
pnpm mobile:android    # needs Android Studio / SDK
```

Open the app, tap **Pair a PC**, scan the QR code (or type the 6-digit code; the PC shows
up in the list through mDNS, or type its address). Scanning the QR with the phone's own
camera app works too. After that, tap the PC on the home screen to connect, and switch
**Microphone** and **Headphones** on.

If the phone can't find the PC: both must be on the same Wi-Fi (guest networks and
"client isolation" block it), and the PC's firewall must allow phoneset on private networks.

## How it's built

**PC (Go, `pc/`).** `cmd/phoneset` is the CLI; `internal/` holds `proto` (wire types),
`secure` (X25519 + ChaCha20-Poly1305 channel), `pairing` (code/secret offers),
`store` (identity and paired phones), `server` (WebSocket, auth, sessions),
`discovery` (mDNS and addresses), `audio` (the `Backend` interface, jitter buffer,
resampling) and `term` (QR and logging). Windows audio is `audio/backend_windows.go`
(WASAPI, pure Go through `go-wca`, so it cross-compiles with no C toolchain). macOS audio
is `audio/backend_darwin.go` plus `ca_darwin.m` (CoreAudio, cgo): the audio threads and a
lock-free ring buffer live in C/Objective-C, and Go only polls the ring, so no Go code ever
runs on a real-time audio thread. `audio/bridge.go` holds what both share (converting the
phone's 48 kHz mono to a device's format, and a device's audio to 20 ms stereo frames).
Anywhere else, and macOS built with `CGO_ENABLED=0`, gets a synthetic development backend
(a tone and a WAV dump) so the rest of the program still runs.

**Phone (Expo, `mobile/`).** `src/core` is platform-free Effect code: services
(`Session`, `PairedPcs`, `Discovery`, `AudioIO`, `SocketFactory`, `KeyValue`, `Entropy`)
wired by layers, a typed protocol (`Schema`), the encrypted channel, and the session state
machine, which reconnects on its own after a drop. `src/platform` supplies the native
implementations (`react-native-audio-api`, `react-native-zeroconf`, `expo-secure-store`,
`expo-crypto`). `src/app` is expo-router; screens use Expo UI's universal components, so
iOS renders SwiftUI and Android renders Compose from one source.

## Development

```sh
pnpm install
pnpm check          # Biome lint + format check
pnpm fix            # apply fixes
pnpm typecheck
cd pc && go vet ./... && GOOS=windows go vet ./...
```

On a Mac, `go vet ./...` covers the CoreAudio backend; the Windows one is checked by the
second command.

Tests are deliberately not committed to this repository.

## Status

Working and verified: the protocol end to end (pairing by code and by QR, rate limiting,
token reconnects, encrypted streaming both ways) between the real phone-side code and the
real Go server, and the iOS app bundle. On macOS, the CoreAudio backend lists the real
devices, plays into an output device for seconds on end without a single underrun, and runs
a full session against the phone-side code (the tap opens, starts and stops cleanly).

Not yet exercised on real hardware:

- the WASAPI backend on a Windows machine (it compiles and passes `go vet` on a Windows
  runner, but hasn't run against real devices);
- on macOS, **the audio content of the system-audio tap** (macOS delivers silence until the
  permission above is granted, which couldn't be done where this was developed; use
  `phoneset test` to check), the BlackHole microphone path (BlackHole wasn't installed),
  capturing an input device, Intel Macs and macOS before 14.2;
- the Android build, and background operation on either phone.

Expect to tune the jitter buffer (`audio.NewPlayout`, 60 ms target) on your Wi-Fi.

Ideas for later: Opus instead of raw PCM, UDP transport for lossy networks, Linux.
