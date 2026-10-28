# phoneset

Use your phone as your PC's **microphone** and **headphones** over Wi-Fi.

```
 phone mic ──Wi-Fi──▶ phoneset (PC, terminal) ──▶ virtual cable ──▶ any app's microphone
 phone speaker ◀──Wi-Fi── phoneset (PC, terminal) ◀── what the PC is playing
```

|                    |                                                                                                              |
| ------------------ | ------------------------------------------------------------------------------------------------------------ |
| `pc/`              | The PC side. A Go program you run in a terminal, no window. Windows first.                                   |
| `mobile/`          | The phone app. Expo (SwiftUI on iOS, Jetpack Compose on Android through Expo UI), logic written with Effect. |
| `docs/protocol.md` | The wire protocol and its security model.                                                                    |

## Using it

### 1. On the PC (Windows 10/11)

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
a second virtual cable and capture that: `phoneset --speaker-device "CABLE-A Input"`. The device that carries the phone's
microphone can't also be the headphone source, because that would echo your voice back to
you; phoneset refuses that combination and says so.

```
phoneset audio                       list playback devices and their roles
phoneset phones                      list paired phones
phoneset forget <name|id|all>
phoneset --mic-device "CABLE"        which device carries the phone's microphone
phoneset --speaker-device "Speakers" which device is sent to the phone
phoneset --no-mic | --no-speaker
phoneset --pair                      show the pairing QR even if phones are paired
phoneset --port 47831 --name "Desk PC" --config-dir DIR --verbose
```

Pairings are stored in `%AppData%\phoneset\`.

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
(WASAPI, pure Go through `go-wca`, so it cross-compiles with no C toolchain). Other
systems get a synthetic development backend (a tone and a WAV dump) so the rest of the
program can run anywhere.

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

Tests are deliberately not committed to this repository.

## Status

Working and verified: the protocol end to end (pairing by code and by QR, rate limiting,
token reconnects, encrypted streaming both ways) between the real phone-side code and the
real Go server, and the iOS app bundle.

Not yet exercised on real hardware: the WASAPI backend on a Windows machine (it compiles
and passes `go vet` for Windows, but hasn't run against real devices), the Android build,
and background operation on either phone. Expect to tune the jitter buffer (`audio.NewPlayout`,
60 ms target) on your Wi-Fi.

Ideas for later: a macOS backend (CoreAudio, with BlackHole as the virtual device), Opus
instead of raw PCM, UDP transport for lossy networks, Linux.
