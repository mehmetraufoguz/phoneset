# phoneset protocol, version 1

The PC runs a small server; the phone connects to it. Everything after the first
two messages is encrypted and authenticated. The reference implementations are
`pc/internal/proto`, `pc/internal/secure` and `mobile/src/core`.

## 1. Finding the PC

| Source        | Provides                                                           |
| ------------- | ------------------------------------------------------------------ |
| QR code       | PC id, name, up to three addresses, port, a 128-bit pairing secret |
| mDNS          | `_phoneset._tcp` service, TXT `v=1`, `id=<pc id>`, `n=<pc name>`   |
| Typed address | host only, default port `47831`                                    |

The QR code is the URL

```
phoneset://pair?v=1&id=<pc id>&n=<url-encoded name>&h=<ip>,<ip>&p=<port>&k=<base64url secret>
```

Scanning it with the phone's system camera opens the app on its pairing screen
with the same parameters.

A PC has several addresses and only some are reachable from the phone, so the
phone asks every address at once and uses the first that answers:

```
GET http://<host>:<port>/v1/info
-> {"app":"phoneset","v":1,"id":"<pc id>","name":"<pc name>"}
```

and checks `id` against the one it expects.

## 2. Transport

WebSocket at `ws://<host>:<port>/v1/ws`. Every message is a binary frame.

## 3. Handshake (plaintext)

```
client -> server   0x01 || client_pub        33 bytes: version, X25519 public key
server -> client   server_pub                32 bytes
```

Both sides compute

```
shared     = X25519(own_secret, peer_pub)
transcript = SHA-256("phoneset/v1" || client_pub || server_pub)
okm        = HKDF-SHA-256(ikm = shared, salt = transcript, info = "phoneset/v1/keys", 64 bytes)
key_c2s    = okm[0:32]      key_s2c = okm[32:64]
```

## 4. Encrypted channel

Each message is `ChaCha20-Poly1305(key, nonce, aad = transcript, plaintext)`.
The nonce is 4 zero bytes followed by a big-endian `u64` counter, one counter per
direction, starting at 0. Counters are not sent: the WebSocket is ordered and
reliable, so both sides stay in step, and a lost or reordered message fails
authentication and ends the connection.

Plaintext is `type (1 byte) || body`:

| type   | body                                  |
| ------ | ------------------------------------- |
| `0x01` | control message, UTF-8 JSON           |
| `0x02` | phone microphone: `seq (u32 BE)       |     | PCM` |
| `0x03` | PC audio for the phone: `seq (u32 BE) |     | PCM` |

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
  "speaker": { "available": true, "device": "Speakers (Realtek)" }
}
```

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

Fixed in v1: 48 kHz, signed 16-bit little endian PCM, 20 ms frames.
Microphone audio is mono (960 samples, 1920 bytes); PC audio is interleaved
stereo (960 frames, 3840 bytes).

Receivers keep a small jitter buffer (about 60 ms), play at a speed that follows
the buffer level by up to ±2 % so the two devices' clocks can drift apart, and
drop the oldest audio rather than let latency grow after a stall. Windows only
delivers loopback audio while something is playing, so the PC sends nothing
during silence; the phone treats an empty buffer as silence and rebuilds its
cushion when audio resumes. `seq` counts frames actually sent.

## 8. Threat model

- Passive listeners on the Wi-Fi see nothing but ciphertext.
- A different phone cannot connect without a token, and a rogue PC cannot
  impersonate a paired one: both directions are proven with the token.
- Pairing with the **QR code** is strong: the secret has 128 bits, so even an
  attacker in the middle of the pairing handshake cannot produce a proof.
- Pairing with the **6-digit code** is protected by the 5-attempt limit and the
  5-minute window, but an attacker who is actively _in the middle_ of that
  one handshake can brute-force a 20-bit code offline against the proof they
  relay. Prefer the QR code on networks you do not trust.
- Audio frames are not padded; their size and timing reveal when someone is speaking.
