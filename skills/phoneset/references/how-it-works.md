# How phoneset works (for deciding what is possible)

```
phone mic ──Wi-Fi──▶ phoneset (PC) ──▶ virtual cable ──▶ any app's microphone
phone speaker ◀──Wi-Fi── phoneset (PC) ◀── what the PC is playing
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
- `docs/protocol.md` is the wire protocol and threat model. Read it before changing anything on the wire.

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

## Security

Every connection starts with an X25519 key exchange and everything after it is ChaCha20-Poly1305 with keys
bound to the handshake. Pairing uses a one-time secret from the QR code (128 bits) or the 6-digit code
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
- Android: built and wired, but not yet exercised on a device by the author.
- A phone suspended in the background by iOS drops the connection unless the microphone or headphones are on.
