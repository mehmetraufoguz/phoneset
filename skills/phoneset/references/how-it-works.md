# How phoneset works (for deciding what is possible)

```
phone mic ──Wi-Fi──▶ phoneset (PC) ──▶ virtual cable ──▶ any app's microphone
phone speaker ◀──Wi-Fi── phoneset (PC) ◀── what the PC is playing
phone camera ──Wi-Fi──▶ phoneset (PC) ──▶ camera helper ──▶ "phoneset Camera" ──▶ any video app
```

## Pieces

- `pc/` Go agent. `internal/audio` has one backend per OS behind an interface: WASAPI on Windows (pure Go,
  cross-compiles without a C toolchain) and CoreAudio on macOS (cgo, Objective-C; the real-time audio threads
  stay in C and Go only polls a ring buffer). Elsewhere, or macOS built without cgo, a synthetic development
  backend runs so the rest of the program still works.
- `mobile/` Expo app. Logic is Effect code (`src/core`, no platform imports: services wired by layers, a typed
  protocol, the encrypted channel, the session state machine that reconnects after a drop). `src/platform`
  has the native pieces (audio engine, mDNS, secure storage). Screens use Expo UI's universal components
  (SwiftUI on iOS, Jetpack Compose on Android).
- `macos/` is the virtual camera for macOS (a Core Media I/O camera extension and a helper app, `phoneset Camera.app`);
  `windows/` is the one for Windows (a DirectShow filter from softcam plus `phoneset-camera.exe`). `pc/internal/camera`
  is the part of the program that installs, removes and watches them, the same on every system.
- `docs/protocol.md` is the wire protocol and threat model. Read it before changing anything on the wire.
  `docs/virtual-camera.md` and `docs/webcam-measurements.md` are the camera's plan, what was run and the numbers.

## Audio

48 kHz, signed 16-bit PCM, 20 ms frames. Microphone audio is mono; PC audio is stereo. No codec, so a stream is
roughly 0.8 Mbit/s (mic) and 1.5 Mbit/s (headphones) and uses little CPU.

Each receiver keeps a jitter buffer (about 60 ms) and plays at a speed that follows the buffer level by up to
±2 %, so the two devices' clocks can drift apart for hours without underruns. After a stall it drops the
oldest audio instead of letting delay grow.

**Latency:** the app's "ms" figure is the network round trip of a small control ping, not audio delay. The
audio path adds the buffers above plus a device queue: estimated roughly 100–150 ms end to end (an estimate
from the settings, not a measurement). A field report on a clean home Wi-Fi: round trip 4–10 ms, no audible
problems while gaming with wired earphones. Buffers could be made smaller on a good network at the price of
glitches when Wi-Fi gets bad.

**Battery:** a field report: about 40 minutes of streaming with wired earphones used 2–3 % of an iPhone's
battery with no warming.

## Video

The phone films and sends H.264 or JPEG frames (the app picks the codec; sizes from 480p to 4K and 30 or 60 frames a
second, as far as the phone's camera and the PC allow). The PC program does not decode them (it stays a plain Go
program, no cgo): it keeps the frames and serves them on the loopback interface (`/video.frames` with codec, size,
rotation and mirror per frame, plus the raw `/video.mjpg` and `/video.h264`). The camera helper reads
`/video.frames`, decodes with the system's own decoder (VideoToolbox on macOS, Media Foundation on Windows), turns the
picture upright, fits it into the camera's frame (a portrait picture gets bars at the sides) and hands it to the
virtual camera. With no picture the camera shows its own slate. The camera is a subscription in the app (gated when
video starts); the microphone and the headphones are not.

## Security

Every connection starts with a P-256 key exchange and everything after it is AES-256-GCM with keys
bound to the handshake (the phone uses its operating system's own cryptography, the PC Go's standard library). Pairing uses a one-time secret from the QR code (128 bits) or the 6-digit code
(5 attempts, 5 minutes, single use) and ends with the PC handing the phone a random token that both use to
prove themselves on every later connection. The QR path is strong; the 6-digit path is vulnerable to an
attacker who is actively *in the middle of that one pairing* (the code is only 20 bits), so prefer the QR code
on networks you don't trust. The loopback control API needs a token and rejects a non-loopback `Host`
header. Details and the threat model: `docs/protocol.md`.

## Limits worth knowing

- One phone connected at a time; the same phone reconnecting replaces its stale connection.
- iOS has no notification that can't be dismissed; the nearest is a Live Activity (opt-in, see install.md).
- macOS: the system-audio tap exists from 14.2 and needs permission; before that, capture a virtual input
  device (BlackHole 16ch behind a Multi-Output device).
- Android: in a closed test on Google Play, not public. The camera and the subscription flow were run on a real
  phone; the repository's Status section still lists Android audio as not exercised on a device.
- The virtual camera is verified on Windows 10 and on one Mac only (see `camera.md`); Linux has none.
- A phone suspended in the background by iOS drops the connection unless the microphone or headphones are on.
