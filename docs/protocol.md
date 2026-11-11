# phoneset protocol, version 2

The PC runs a small server; the phone connects to it. Everything after the first
two messages is encrypted and authenticated. The reference implementations are
`pc/internal/proto`, `pc/internal/secure` and `mobile/src/core`.

Version 2 replaced version 1 (X25519 and ChaCha20-Poly1305, run by a cryptography library in
the phone app) with primitives that both phone operating systems provide themselves: P-256
ECDH, HKDF-SHA-256, HMAC-SHA-256 and AES-256-GCM. The app carries no cryptography of its
own: it calls CryptoKit on iOS and the Java cryptography architecture on Android
(`mobile/modules/phoneset-crypto`), and the PC uses Go's standard library. A phone and a PC
that speak different versions do not talk to each other: the paths below carry the version.

Video (section 8) was added to version 2 without a new version number, on a connection of its own.
The rule is in section 8.6.

## 1. Finding the PC

| Source        | Provides                                                           |
| ------------- | ------------------------------------------------------------------ |
| QR code       | PC id, name, up to three addresses, port, a 128-bit pairing secret |
| mDNS          | `_phoneset._tcp` service, TXT `v=2`, `id=<pc id>`, `n=<pc name>`   |
| Typed address | host only, default port `47831`                                    |

The QR code is the URL

```
phoneset://pair?v=2&id=<pc id>&n=<url-encoded name>&h=<ip>,<ip>&p=<port>&k=<base64url secret>
```

Scanning it with the phone's system camera opens the app on its pairing screen
with the same parameters.

A PC has several addresses and only some are reachable from the phone, so the
phone asks every address at once and uses the first that answers:

```
GET http://<host>:<port>/v2/info
-> {"app":"phoneset","v":2,"id":"<pc id>","name":"<pc name>"}
```

and checks `id` against the one it expects.

## 2. Transport

WebSocket at `ws://<host>:<port>/v2/ws`. Every message is a binary frame.

## 3. Handshake (plaintext)

```
client -> server   0x02 || client_pub        66 bytes: version, P-256 public key
server -> client   server_pub                65 bytes
```

A public key is an uncompressed P-256 point, `0x04 || X || Y` (X9.63, 65 bytes). A key that is
not a point on the curve ends the connection.

Both sides compute

```
shared     = ECDH-P256(own_secret, peer_pub)      the 32-byte x coordinate
transcript = SHA-256("phoneset/v2" || client_pub || server_pub)
okm        = HKDF-SHA-256(ikm = shared, salt = transcript, info = "phoneset/v2/keys", 64 bytes)
key_c2s    = okm[0:32]      key_s2c = okm[32:64]
```

## 4. Encrypted channel

Each message is `AES-256-GCM(key, nonce, aad = transcript, plaintext)`, sent as the ciphertext
followed by the 16-byte tag. The nonce is 12 bytes: 4 zero bytes followed by a big-endian `u64` counter, one counter per
direction, starting at 0. Counters are not sent: the WebSocket is ordered and
reliable, so both sides stay in step, and a lost or reordered message fails
authentication and ends the connection.

Plaintext is `type (1 byte) || body`:

| type   | body                                                                                                    |
| ------ | ------------------------------------------------------------------------------------------------------- |
| `0x01` | control message, UTF-8 JSON                                                                             |
| `0x02` | phone microphone: `seq (u32 BE) \|\| PCM`                                                               |
| `0x03` | PC audio for the phone: `seq (u32 BE) \|\| PCM`                                                         |
| `0x04` | phone camera: `seq (u32 BE) \|\| timestamp (u32 BE) \|\| width (u16 BE) \|\| height (u16 BE) \|\| JPEG` |

A receiver ignores a type it does not know. A message may be at most 64 KiB, except on the video
connection (section 8), which allows frames after it has authenticated. Type `0x04` is used only there.

## 5. Authentication

All proofs are `HMAC-SHA-256(key, label || transcript || parts…)`, which ties
them to this connection. The first control message from the phone is one of:

**Pairing**

```json
{
  "t": "pair",
  "deviceId": "…",
  "name": "Rauf's iPhone",
  "platform": "ios",
  "method": "qr",
  "proof": "<b64>"
}
```

- `method: "qr"`: `key` is the 16-byte secret from the QR code.
- `method: "code"`: `key` is `SHA-256("phoneset/code/" || six digits)`.
- `proof = HMAC(key, "phoneset/pair/c" || transcript || deviceId)`.

The PC answers `{"t":"paired","pcId","pcName","token","proof"}` where `token` is 32
fresh random bytes (base64) and
`proof = HMAC(key, "phoneset/pair/s" || transcript || deviceId || token)`, which
proves to the phone that this is the PC that showed the QR code or code. Both
store the token.

An offer is valid for five minutes and is consumed by the first success. Five
wrong proofs withdraw it.

**Reconnecting**

```json
{ "t": "auth", "deviceId": "…", "proof": "<b64>" }
```

`proof = HMAC(token, "phoneset/auth/c" || transcript || deviceId)`. The PC answers
`{"t":"authed","pcId","pcName","proof"}` with label `phoneset/auth/s`.

**Failure** is `{"t":"error","code","message"}` and a close. Codes: `bad_proof`,
`unknown_device`, `no_pairing`, `rate_limited`, `busy`, `protocol`.

## 6. Session

After authentication the PC sends

```json
{
  "t": "ready",
  "audio": { "rate": 48000, "frameMs": 20, "micChannels": 1, "speakerChannels": 2 },
  "mic": { "available": true, "device": "CABLE Input (VB-Audio Virtual Cable)" },
  "speaker": { "available": true, "device": "Speakers (Realtek)" },
  "video": {
    "available": true,
    "path": "/v2/video",
    "maxFrameBytes": 4194304,
    "codecs": ["jpeg"],
    "features": []
  }
}
```

`video` is present only on a PC that knows about video. A phone that finds it missing, or
`available: false` (with a `reason`), does not offer a camera. Otherwise `path` is where the video
connection is opened (section 8) and `maxFrameBytes` the largest frame the PC reads.

and then:

| direction  | message                                                                                                                      |
| ---------- | ---------------------------------------------------------------------------------------------------------------------------- |
| phone → PC | `{"t":"stream","mic":true,"speaker":false}`, either field optional                                                           |
| PC → phone | `{"t":"stream","mic":true,"speaker":false,"micError":"…"}`, the effective state, also sent unprompted if a device disappears |
| phone → PC | `{"t":"ping","n":1,"c":<ms>}`; the PC replies `{"t":"pong",…}` with the same fields                                          |
| PC → phone | `{"t":"stats","mic":{"bufferMs","underruns","drops"},"speaker":{"sent","dropped"}}` every 2 s                                |
| either     | `{"t":"bye"}`                                                                                                                |

Only one phone can be connected at a time. The same phone reconnecting replaces
its stale connection; a different phone gets `busy`.

## 7. Audio

Fixed in v2: 48 kHz, signed 16-bit little endian PCM, 20 ms frames.
Microphone audio is mono (960 samples, 1920 bytes); PC audio is interleaved
stereo (960 frames, 3840 bytes).

Receivers keep a small jitter buffer (about 60 ms), play at a speed that follows
the buffer level by up to ±2 % so the two devices' clocks can drift apart, and
drop the oldest audio rather than let latency grow after a stall. Windows only
delivers loopback audio while something is playing, so the PC sends nothing
during silence; the phone treats an empty buffer as silence and rebuilds its
cushion when audio resumes. `seq` counts frames actually sent.

## 8. Video

The phone's camera as a webcam, stage one: the phone sends pictures and the PC hands the bytes on to a
local program (section 9). Only one way of coding a picture, a JPEG per frame, is defined so far, but
the messages name the codec, the size and the frame rate, so more can be added (8.7) without changing
them. There is no video decoding or encoding on the PC. Whatever turns this into a camera that other
programs can pick (a virtual camera driver) is not part of the protocol yet.

Nothing in this section assumes how fast the link is, or which Wi-Fi band it is on. What a link can
carry is found out by measuring it (8.8), not by reading the network.

### 8.1 A connection of its own

Video does not travel on the connection of sections 2 to 7. It has its own, so a video frame that is
slow to send can never sit in front of a piece of microphone audio on the same TCP connection, and so
audio, which every phone uses, stays separate from video, which is optional and negotiated on its own.

- The PC offers it with `video` in `ready` (section 6). `codecs` are the codecs the PC accepts,
  `maxFrameBytes` the largest frame it reads, and `features` optional extras it supports (none yet; 8.8
  reserves one). A phone that sees no `video`, or `available: false`, does not open it, and a PC that was
  started with `--no-video` answers `video_off`.
- It is a WebSocket at `ws://<host>:<port>/v2/video` (the `path` in `ready`) with the same handshake and
  encrypted channel as sections 3 and 4. Its keys are its own: it is a new handshake.
- The first message is `{"t":"auth", …}` exactly as in section 5, proven with the pairing token. A
  `pair` message is refused (`protocol`): the camera is for a phone that is already paired, and that
  keeps one place, the main connection, where a phone is introduced.
- The phone must have its main connection open, because there is only ever one phone: otherwise the
  answer to `auth` is `authed` followed by `{"t":"error","code":"no_session",…}` and a close. When the
  main connection ends, the PC closes the video connection too. A new video connection from the same
  phone replaces the old one.

The phone opens it when the user turns the camera on and closes it when they turn it off, and does not
open it otherwise. Nothing about video is sent on the main connection; a phone that never opens the
video connection behaves exactly as before.

### 8.2 Modes, starting and stopping

A _mode_ is a codec, a size and a frame rate, written `{"codec":"jpeg","width":1280,"height":720,"fps":30}`.
`width` and `height` are the upright picture; `fps` is the rate the camera is asked for (what is sent can
be lower, 8.4). The protocol names no resolutions or rates; 480p, 720p, 1080p, 2K and 4K at 30 or 60
frames a second are just modes. After `authed` the phone starts the camera with

```json
{
  "t": "video",
  "on": true,
  "camera": "back",
  "mode": { "codec": "jpeg", "width": 1280, "height": 720, "fps": 30 },
  "modes": [
    { "codec": "jpeg", "width": 640, "height": 480, "fps": 30 },
    { "codec": "jpeg", "width": 1280, "height": 720, "fps": 30 }
  ]
}
```

- `mode` is what the phone is about to send. `modes` is everything the phone is willing to send with this
  camera, the list the PC (or later the user, or a link test, 8.8) can pick from. It is optional, and
  it says what the phone can capture and code, nothing about the link. `mode` must be one of `modes`
  when `modes` is present.
- The PC answers with the state in effect: `{"t":"video","on":true,"camera":"back","mode":{…}}`, or
  `"on":false` and an `error`, for instance when `mode.codec` is not one of `ready.video.codecs`. In this
  version the PC answers with the mode it was sent. A phone that gets a different mode may end the video; the phone app does.
- `camera` is `"back"` or `"front"`. Sending the message again with another camera or mode switches
  while running. The PC does not act on `camera`, it shows it.
- `{"t":"video","on":false}` stops the camera without closing the connection; `bye` or closing the
  connection stops it as well. Frames sent while the camera is off are ignored and not acknowledged.

### 8.3 Frames

Type `0x04` carries one frame:

| field       | size                          | meaning                                                                                                                                                        |
| ----------- | ----------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `seq`       | u32 BE                        | counts frames actually sent, from 0 for each start of the video stream                                                                                         |
| `timestamp` | u32 BE                        | milliseconds on the phone's steady clock when the frame was captured; it wraps after 49.7 days, so only differences between frames of one stream mean anything |
| `codec`     | u8                            | how `payload` is coded: 1 `jpeg`, 2 `h264`, 3 `hevc` (see 8.7)                                                                                                 |
| `flags`     | u8                            | bit 0: the frame can be decoded on its own (a key frame). All other bits are 0                                                                                 |
| `width`     | u16 BE                        | pixels of the decoded picture                                                                                                                                  |
| `height`    | u16 BE                        | pixels of the decoded picture                                                                                                                                  |
| payload     | rest, at most `maxFrameBytes` | the coded picture                                                                                                                                              |

The header is 14 bytes. The `codec` must be the one of the `mode` in effect; a frame with another is
dropped like one that does not decode. For `jpeg` the payload is a complete baseline JPEG that starts with
`FF D8`, and every frame is a key frame (`flags` 1): nothing depends on an earlier frame.
The picture is already upright: the phone rotates it, and `width` and `height` are the upright size. The
front camera is not mirrored. As a guide, a 1280 x 720 JPEG at quality 70 is about 50 to 120 KB. A frame
that is not what it says it is is dropped (and acknowledged, 8.4) and the connection stays. A sender must
not send more than `maxFrameBytes`: the PC reads messages only slightly larger than that, and ends the
connection on a bigger one. Until `authed` a message may be at most 64 KiB, as everywhere.

### 8.4 Not building a backlog

The two connections share the Wi-Fi link and the phone's processor, so video must not pile up even on
a connection of its own: a queue of frames is latency for the picture and a burden on the network for
everything else. Two rules:

- **The sender drops, before it encrypts.** An encrypted message that was made but never sent ends the
  connection (section 4), so frames are dropped at the source, never after sealing.
- **The PC acknowledges each frame** the moment it has been read: `{"t":"vack","seq":n}`. The sender
  limits how much is sent and not yet acknowledged, and discards (never queues) the pictures the camera
  produces beyond that. The limit is the sender's policy, not part of the protocol; it has to allow at
  least one frame whatever its size. The phone app keeps at most three frames in flight, and the bytes in
  flight plus one more frame of the size of the last stay under 2 MB (it was two frames and 256 KB; that
  left a frame over 128 KB alone in flight and halved the frame rate at 1080p, see
  `docs/webcam-measurements.md`). A slow link therefore lowers the frame rate, and a fast one does not need
  a different rule.

### 8.5 Statistics

While the camera is on the PC sends `{"t":"vstats","frames":n,"bytes":n,"viewers":n}` every 2 s: frames
and bytes received since the camera was turned on, and how many programs are reading it on the PC
(section 9).

### 8.6 Version

Video stays **version 2**. The version number names the handshake and the framing of the encrypted
channel (the `/v2/` paths, the HKDF and transcript labels, `v=2` in the QR code and mDNS record), and
none of that changes: the video connection is the same handshake and framing on one more path. What
is added is optional and negotiated: `video` in `ready`, a path, a type byte and a few control tags on that
path, and codecs, sizes and rates that are named in the messages rather than fixed by the version. The rules that make this safe, and that further additions to version 2 must follow:

- A receiver ignores an unknown frame type and unknown JSON fields. (The phone app's message parser
  already drops unknown fields.)
- A control message with a tag the receiver does not know is an error for the phone app, which ends the
  connection. So a PC sends a new tag only to a phone that asked for the feature, and a phone sends a
  new message only to a PC that offered it. (Here: the video tags exist only on the video connection.)
- A phone opens the video connection only when `ready` offers it. An older PC has no such path.

A **version 3** is needed when something changes that cannot be switched on by negotiation: the
handshake or key derivation, the layout of an existing message, or a new type that every peer must
understand.

### 8.7 Adding a codec

`jpeg` is the only codec defined. `h264` and `hevc` have numbers (2 and 3) so that nothing has to move
when they are defined, but their payload (Annex B or length-prefixed units, where the parameter sets
go, what a key frame is) is not written yet, and a phone does not offer them in `modes`, and a PC does
not list them in `codecs`, until it is. Coding one with a hardware encoder on the phone and showing it
on the PC needs a decoder that is not in this program's standard library; that is the reason for the
split between the codec in the message and the codecs a PC accepts. A new codec is a new value in an
existing field, negotiated with `codecs` and `modes`, so it is not a new protocol version.

### 8.8 Reserved: a link test

Which modes a particular link can carry is not read from the network (an iPhone cannot see its Wi-Fi
band, a 5 GHz link in a bad spot can be slower than a good 2.4 GHz one, and the PC may be on Ethernet).
It is meant to be measured, on the video connection, before a high mode is allowed. Not implemented
yet; reserved so that it can be:

- A PC that supports it lists `"probe"` in `ready.video.features`. A phone sends nothing of it otherwise.
- The phone sends a timed burst of type `0x05` messages (`id u32 || index u32 ||` padding) and then
  `{"t":"vprobe","id":n,"messages":m,"bytes":b}`; the PC answers
  `{"t":"vprobe","id":n,"messages":m,"bytes":b,"ms":t,"longestGapMs":g}`, the bytes it received, the
  time they took and the longest wait between two messages.
- From that result either side may cap the modes it allows: the PC answers a `video` request with
  `"on":false` and an `error`, and may add `"limit":{"width":w,"height":h,"fps":f}`, the largest mode it
  will take on this connection; the phone does not offer modes above a limit it has been given.

## 9. Local video output (PC)

The frames never leave the PC. The agent hands the JPEG bytes unchanged to HTTP clients on its local
control server (`127.0.0.1`, a random port written to `control.json`), the same server, token and `Host`
check that `phoneset status` uses, so nothing is reachable from the network:

| request                     | answer                                                                                                           |
| --------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `GET /video.mjpg?t=<token>` | `multipart/x-mixed-replace`: every frame as one `image/jpeg` part, newest first, for as long as the client reads |
| `GET /video.jpg?t=<token>`  | the latest frame as one JPEG (404 until there is one)                                                            |
| `GET /video?t=<token>`      | a page that shows the stream in a browser                                                                        |

`phoneset video` prints the address, and `phoneset status` shows it while a phone is sending video. A
reader that is slower than the phone gets the newest frame and misses the ones in between; it never
slows the phone down.

## 10. Threat model

- Passive listeners on the Wi-Fi see nothing but ciphertext.
- A different phone cannot connect without a token, and a rogue PC cannot
  impersonate a paired one: both directions are proven with the token.
- Pairing with the **QR code** is strong: the secret has 128 bits, so even an
  attacker in the middle of the pairing handshake cannot produce a proof.
- Pairing with the **6-digit code** is protected by the 5-attempt limit and the
  5-minute window, but an attacker who is actively _in the middle_ of that
  one handshake can brute-force a 20-bit code offline against the proof they
  relay. Prefer the QR code on networks you do not trust.
- Audio frames are not padded; their size and timing reveal when someone is speaking. Video frames
  are not padded either: their size and rate reveal movement in front of the camera.
- The local video address is as sensitive as the control API: the token is in the URL, the server only
  answers a `Host` of `127.0.0.1` or `localhost` (so a web page cannot read it through DNS rebinding),
  and any program of the same user that can read `control.json` could read the camera as well.
