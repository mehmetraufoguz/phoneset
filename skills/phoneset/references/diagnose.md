# Diagnosing a problem

Use this when someone says "it doesn't work". Work in this order and stop at the first thing that explains it.
Everything in step 2 only reads; nothing there changes the machine.

## 1. Ask (don't assume)

| Ask | Why |
| --- | --- |
| Which part fails: pairing, the phone is not found, the microphone, the headphones, the camera, or a drop-out? | each has its own section in `troubleshooting.md` |
| PC: Windows (10 or 11, which CPU) or Mac (Apple silicon or Intel, macOS version)? | `requirements` in `clean-install.md`; the system-audio tap needs macOS 14.2, the camera 12.3 |
| Phone: iPhone or Android, which OS version, which app version, installed from the store or built by hand? | iOS 16.4 or newer; the app's version follows the PC program's |
| Did something change just before (an app update, a PC update, a new router, a VPN, a new PC)? | a phone and a PC on different protocol generations do not talk (see "The version rule") |
| Same Wi-Fi? A guest network, a mesh "client isolation", a VPN, a work PC? | the commonest reason a phone does not find the PC |
| Is the virtual cable installed (VB-CABLE, BlackHole) and was the PC restarted after VB-CABLE? | apps can't hear the phone without it |
| First time, or did it work before? | a first-time problem is install or permissions; "worked before" is usually network or versions |

## 2. Collect (read-only)

```sh
phoneset version            # e.g. "phoneset 0.3.1 (c9d407e)"
phoneset status             # is it running, is a phone connected, the camera, start at login
phoneset audio              # which devices it would use; "*" marks the chosen ones
phoneset camera status      # only for camera problems
phoneset logs               # the log's path and its last lines
```

A healthy, idle PC looks like this (names and addresses replaced):

```
phoneset 0.3.1 (c9d407e) running in the background  (PID 10720, up 37h33m0s)
  name             <this PC's name>
  address          <local ip>:47831
  phone            waiting for a phone
  virtual camera   not installed Run `phoneset camera install`.
  paired phones    <phone name>
  start at login   on
```

```
Audio devices  (CoreAudio)

  The phone's microphone is played into (apps record from the other end):
    * BlackHole 2ch  chosen automatically, virtual
  Sent to the phone's headphones:
    * System audio (no driver needed)  chosen automatically
```

Reading it:

- "not running" or no PID: start it (`phoneset daemon`); the phone can't connect to a PC that is not running.
- `phone  waiting for a phone` with a phone that is sure it is connected: not the same network, or the phone is on
  another protocol generation (see below).
- In `phoneset audio`, the microphone section must list a device marked `virtual` with `*`. None means no cable is
  installed (or the PC wasn't restarted after VB-CABLE).
- The headphone source on macOS 14.2+ is "System audio"; if the permission is missing it is silent with no error
  (`phoneset test` shows it).
- `virtual camera  not installed`: only matters for the webcam; `phoneset camera install`.

The log is one line per event, `time=… level=… msg="…" key=value`:

| Line | Means |
| --- | --- |
| `phoneset started version=… commit=… mode=background pid=…` | the program (re)started; two of these close together is a crash or an update |
| `phone connected name=… from=<ip>` | a phone got in and from which address |
| `phone disconnected name=… reason="…"` | it left; `connection lost (…)` means the connection dropped (the network, or the phone stopped), not a fault in the audio |
| `camera on facing=… codec=… size=… fps=…` / `camera off` | the phone started or stopped sending its camera |
| `level=WARN …` | read the text; `phoneset --verbose` in a terminal shows more |

Nothing in the log for a phone that "tried to connect" does **not** prove it never arrived: failed handshakes and
failed authentication are logged only at debug level. Run `phoneset --verbose` in a terminal (stop the background one
first: `phoneset stop`), try again, and look for `handshake failed` or `authentication failed` with the phone's address.
No line at all even then means the phone never reached the PC: network, firewall, Local Network permission on iPhone.

## 3. The version rule

The phone and the PC speak a numbered protocol and the number is in the address (`/v2/…`). 0.1.x was protocol 1, 0.2.0
and everything after it is protocol 2. Phones and PCs on different numbers **do not talk at all**: expect a plain connection
failure, as if the PC could not be reached (inferred from the protocol; not tried with a mismatched pair). Video needs a PC on 0.3.0 or newer (the app then says "This PC can't receive video.
Update phoneset on the PC."). After the phone app updates, update the PC too:

```sh
phoneset update --check
phoneset update
```

## 4. Where to go next

| Symptom | First check | File |
| --- | --- | --- |
| Phone doesn't find the PC | same Wi-Fi, firewall (private networks), iPhone Local Network, the version rule | `troubleshooting.md` |
| Pairing fails | `phoneset pair` for a fresh code (5 minutes, one use) | `troubleshooting.md` |
| Apps don't hear the phone | `phoneset audio`; the app must pick the cable's other end | `troubleshooting.md` |
| Phone is silent | headphones switch; macOS permission; Windows plays nothing | `troubleshooting.md` |
| Echo | earphones | `troubleshooting.md` |
| "phoneset Camera" missing or black | `phoneset camera status`; the phone's switch | `troubleshooting.md`, `camera.md` |
| The camera switch turns itself off | the subscription | `troubleshooting.md` |
| Install fails, or a clean start is wanted | the installer's own messages | `clean-install.md` |
| Something that looks like a bug | is it in the known issues? | `troubleshooting.md` (end) |

## 5. What not to ask for, what to redact

- **Never ask for or paste `devices.json` or `control.json`**: they hold the pairing tokens and the control token of the
  running program.
- `phoneset status`, `phoneset logs` and `phoneset audio` contain the PC's name, the phone's name and local network
  addresses (with `--verbose` more of them). They are safe to share with the person who is helping, but cut the names
  and addresses before posting them anywhere public.
- The pairing QR code and the 6-digit code are secrets for five minutes; don't paste a screenshot of them.

## 6. Reporting a real bug

Collect the PC's OS and version, the phone's model, OS and app version, `phoneset version`, what was done, what happened
and what was expected, and the log lines around the time (redacted). Then:

- Issues: https://github.com/mehmetraufoguz/phoneset/issues
- The user-facing help page: https://rauf.app/phoneset/support
- What the app stores and sends: https://rauf.app/phoneset/privacy
