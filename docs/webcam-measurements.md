# Phone camera as a PC webcam: findings and measurements

All of this was measured on 2026-11-11 with the prototype in `main` (a video connection of its own, protocol
section 8), on two phones, one Mac and one Wi-Fi; the native send path (section 3.1) was measured on 2026-11-12 on
the branch `native-video` (now merged into `main`), with the same phones, Mac and Wi-Fi. They are numbers from one room, not guarantees; what
each number does and does not show is written next to it. Sections 1 to 3 are the short version, sections 4 to 6 the
evidence, the appendix the raw runs.

## 1. Summary

The question: can a phone's camera be a PC webcam over Wi-Fi at the planned tiers, 480p to 4K at 30 and 60 frames
a second?

- **JPEG**, the first codec, reaches its rate only up to 720p60 and 1080p30 on a good iPhone, and only at 480p on a
  low to mid-range Android phone (OPPO A9 2020).
- **Hardware H.264** reaches the requested rate up to 2K60 on the iPhone 13 Pro (4K at 25 fps) and 30 fps up to
  2688 x 1512 on the OPPO (4K at 25 fps), with 5 to 15 times fewer bits than JPEG. The Android phone's camera offers
  30 fps only, so 60 fps was not tried there.
- **What limited JPEG** was not the Wi-Fi and, on the iPhone, not the encoder: it was the rule that let a large
  frame be the only one in flight (fixed), then the base64 work that React Native does on the JavaScript thread for
  every message, and on the Android phone the JPEG encoder itself. **What limited H.264 at 4K** was the same
  JavaScript hand-over of a 100 to 170 KB frame (on the OPPO), and on the iPhone the coder's delay, which keeping two
  pictures in the coder removed.
- **The native send path** (frames sealed and sent by Swift and Kotlin, not through JavaScript; now in
  `main`) took the base64 work away: iPhone JPEG went from 38.7 to 58.6 fps at 1080p60, 24.4 to
  59.4 at 2K60 and 5.7 to 30.0 at 4K30 (up to 182 Mbit/s over Wi-Fi), and the OPPO's H.264 at 4K went from 25.3 to
  30.0 fps with no stall over 100 ms. Where JavaScript was not the limit (JPEG up to 1080p30 and H.264 up to 2K60 on
  the iPhone, JPEG on the OPPO) it changed nothing. A reader that joins a stream now gets a picture after 40 to
  110 ms instead of 0.2 to 1.9 s. Its cost in processor load, heat and latency was not measured (section 3.1).
- **Still open**: the latency from the camera to a picture on the PC (never measured), a decoder on the PC and a
  virtual camera (neither exists), a run of an hour on a phone that warms up (the iPhone ran about twice as slow
  after an hour), other phones, 2.4 GHz and distance.

## 2. What the app does now (state of `main`)

| Area             | State                                                                                                                                                                |
| ---------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Default mode     | JPEG 720p30, on both platforms (the gate `CODEC_TRIED` in `mobile/src/core/videoModes.ts` offers JPEG up to 720p30)                                                  |
| H.264            | iOS and Android code it in hardware and offer it in `modes` up to 4K at 60; it is picked only by a test build (a temporary "Next mode" button that is not in `main`) |
| PC side          | accepts `jpeg` and `h264`, shows JPEG as MJPEG at `/video.mjpg` and `/video`, passes H.264 on as a raw Annex B stream at `/video.h264`; it does not decode H.264     |
| Window           | 3 frames and 2 MB in flight (it was 2 frames and 256 KB)                                                                                                             |
| Coder pipeline   | 2 pictures asked for ahead, for H.264 only                                                                                                                           |
| Native send path | in `main` since 2026-11-12, on both platforms (section 3.1): frames are sealed and sent natively                                                                     |
| Not built        | a decoder on the PC, a virtual camera, the link test of protocol 8.8, the subscription, a mode picker, thermal-aware mode changes                                    |

Changes made because of the measurements, in order:

| Commit                          | What                                                                                                                                                                       | Measured effect                                            |
| ------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| `442531f`                       | the iOS camera module takes the exact size and rate asked for (it used to pick the 720p or 480p preset from the width alone, so nothing above 720p had ever been captured) | frames arrive at 1080p, 1440p and 2160p                    |
| `145090b`                       | video window 3 frames, 2 MB                                                                                                                                                | iPhone JPEG 1080p30 16.8 to 30.1 fps, 1080p60 20.2 to 38.7 |
| `4f1e5a4`, `aed3b71`, `6cfd725` | the PC accepts h264 and relays it; protocol section 8.3 defines the payload; the iOS VideoToolbox coder                                                                    | section 5                                                  |
| `71fc0c3`                       | two pictures in the coder at a time (h264)                                                                                                                                 | iPhone 4K30 15.0 to 25.4 fps, 1080p60 53.9 to 59.5         |
| `6caf56f`                       | the PC advertises only reachable interfaces over mDNS                                                                                                                      | Android pairing by the discovered PC works (section 7)     |
| `d43e036`, `48a7379`            | a phone offers only the codecs its camera module can make; the Android MediaCodec coder                                                                                    | OPPO 480p only (JPEG) to 30 fps up to 2688 x 1512          |

## 3. Best result per mode

Frame rate averaged over 25 s; "full" means within a few percent of the requested rate on a cool phone. Android modes
are listed by the size the coder was actually given, because CameraX chooses it (section 6).

**iPhone 13 Pro**

| Mode               | JPEG                  | H.264                 |
| ------------------ | --------------------- | --------------------- |
| 480p30             | 30.1 fps, 7.4 Mbit/s  | 30.0 fps, 0.5 Mbit/s  |
| 480p60             | 60.0 fps, 14.3 Mbit/s | 59.0 fps, 1.6 Mbit/s  |
| 720p30             | 30.1 fps, 16.6 Mbit/s | 29.8 fps, 2.6 Mbit/s  |
| 720p60             | 59.9 fps, 32.7 Mbit/s | 56.4 fps, 5.3 Mbit/s  |
| 1080p30            | 30.1 fps, 46.9 Mbit/s | 30.0 fps, 6.2 Mbit/s  |
| 1080p60            | 38.7 fps, 54.6 Mbit/s | 59.5 fps, 10.8 Mbit/s |
| 2K30 (1920 x 1440) | 19.9 fps, 38.4 Mbit/s | 30.0 fps, 2.4 Mbit/s  |
| 2K60               | 19.5 fps, 37.7 Mbit/s | 60.0 fps, 14.4 Mbit/s |
| 4K30               | 4.4 fps, 34.0 Mbit/s  | 25.4 fps, 24.9 Mbit/s |
| 4K60               | 4.7 fps, 28.7 Mbit/s  | 25.6 fps, 34.5 Mbit/s |

**OPPO A9 2020** (Snapdragon 665, Android 11), mode asked and size coded:

| Mode asked / coded        | JPEG                 | H.264                 |
| ------------------------- | -------------------- | --------------------- |
| 864 x 480 / 864 x 480     | 24.2 fps, 2.8 Mbit/s | 30.0 fps, 1.5 Mbit/s  |
| 1600 x 720 / 1920 x 1080  | 5.4 fps, 4.4 Mbit/s  | 30.0 fps, 6.2 Mbit/s  |
| 2400 x 1080 / 2688 x 1512 | 1.3 fps, 2.5 Mbit/s  | 30.1 fps, 12.2 Mbit/s |
| 1920 x 1440 / 2688 x 1512 | 1.6 fps, 3.0 Mbit/s  | 30.1 fps, 12.2 Mbit/s |
| 3840 x 2160 / 3840 x 2160 | 0.4 fps, 2.0 Mbit/s  | 25.4 fps, 21.1 Mbit/s |

The iPhone JPEG numbers are from the runs described in section 4 (the 3-frame window from 1080p up, a cool phone
below it, 4K60 from the older window); the iPhone H.264 numbers from 480p30 to 720p30 are from the run with one picture
in the coder, the rest with two (appendix A.5). The scene was not the same in every run and a frame's size depends on
it, so Mbit/s differ a little between runs.

### 3.1 With the native send path (2026-11-12)

What was built (commits `a6c8f9e`, `a172042`, `a709e38`, `c20a050`, `bd0a389`): JavaScript still opens the video
connection, authenticates and negotiates the mode, on a socket that Swift (`URLSessionWebSocketTask`) or Kotlin
(OkHttp) owns. It then hands the keys and the counters over, and a native pipe pulls frames from the camera, seals
them (AES-GCM), sends them, reads the acknowledgements, keeps the window and answers the PC's key frame request;
JavaScript only sees the other control messages and, once a second, the numbers. The protocol is unchanged on the
wire, apart from two additions: a phone that lists `keyreq` in `features` gets a `vkey` message when a reader joins
or loses a frame, and the frame flags carry the picture's rotation and mirror (section 8.3 of the protocol).

How it was measured: the same script as before, but 20 s per mode, native and JavaScript path one after the other in
one session on the same scene (a test build with a button to switch). The scene was still, so H.264 frames were small
(2 to 18 KB up to 2K60) and the Mbit/s are lower than in the earlier runs. One run per mode and path; runs vary, so
differences of a few percent mean nothing.

**iPhone 13 Pro, JPEG** (a test build opened the gate that holds JPEG at 720p30 on `main`):

| Mode       | Native: fps (worst second), Mbit/s | JavaScript: fps (worst second), Mbit/s |
| ---------- | ---------------------------------- | -------------------------------------- |
| 1080p30    | 30.1 (30), 37.2                    | 30.1 (29), 37.2                        |
| 1080p60    | **58.6** (56), 71.7                | 40.2 (37), 49.5                        |
| 2K30       | **30.1** (30), 45.4                | 25.4 (23), 37.9                        |
| 2K60       | **59.4** (57), 94.5                | 24.4 (24), 38.8                        |
| 4K30       | **30.0** (29), 182.0               | 5.7 (5), 34.4                          |
| 4K60 asked | 30.1 (23), 193.7                   | 5.3 (4), 34.2                          |

At 4K30 the native path had no gap over 100 ms (longest 79 ms) and at 4K60 one (100 ms); the JavaScript path had 114
and 103 of them. A 4K JPEG frame is 735 to 790 KB. The phone never made more than 30 frames a second at 4K, for a
reason that was not looked into (the H.264 runs stop at 25 as well). The Wi-Fi carried 194 Mbit/s here, so for these
modes it is not the limit at this spot.

**iPhone 13 Pro, H.264** (native; the JavaScript figures are from section 3 unless it says otherwise):

| Mode    | Native: fps (worst second)   | Section 3 (JavaScript)                                                                  |
| ------- | ---------------------------- | --------------------------------------------------------------------------------------- |
| 480p30  | 29.4 (25)                    | 30.0                                                                                    |
| 480p60  | 46.4 (45), one gap of 1.75 s | 59.0                                                                                    |
| 720p30  | 29.6 (28)                    | 29.8                                                                                    |
| 720p60  | 57.5 (51)                    | 56.4                                                                                    |
| 1080p30 | 30.0 (29)                    | 30.0                                                                                    |
| 1080p60 | 58.5 (55)                    | 59.5                                                                                    |
| 2K30    | 30.0 (30)                    | 30.0                                                                                    |
| 2K60    | 59.6 (58)                    | 60.0                                                                                    |
| 4K30    | 25.7 (25)                    | 25.4                                                                                    |
| 4K60    | 25.6 (25), 41.7 Mbit/s       | 25.6; 22.4 (18) at 37.7 Mbit/s when the JavaScript path was run right after, same scene |

Up to 2K60 H.264 is no different on the two paths, as expected: its frames are small. The one result worse than before
is 480p60 (46.4 fps and one freeze of 1.75 s); it was run once and not repeated, so it is not known whether it is the
native path, the scene or the mode change. At 4K the native path keeps 25 fps like the JavaScript one did in section 3
(the camera or the coder is the limit there); the JavaScript path, run right after, gave 22.4.

**OPPO A9 2020**, mode asked and size coded (JavaScript path run right after the native one):

| Mode asked / coded          | Native: fps (worst second), Mbit/s, stalls over 100 ms | JavaScript: the same                |
| --------------------------- | ------------------------------------------------------ | ----------------------------------- |
| H.264 864x480               | 30.1 (30), 1.4, none                                   | 30.0 (30), 1.4, none                |
| H.264 1600x720 / 1920x1080  | 30.0 (30), 6.2, none                                   | 30.0 (30), 6.2, none                |
| H.264 2400x1080 / 2688x1512 | 30.0 (29), 12.2, none                                  | 30.1 (29), 12.2, 2 (longest 107 ms) |
| H.264 1920x1440 / 2688x1512 | 30.0 (29), 12.2, none                                  | 30.1 (30), 12.2, none               |
| H.264 3840x2160             | **30.0** (29), 24.9, none (longest gap 58 ms)          | 25.3 (21), 21.0, 9 (longest 219 ms) |
| JPEG 864x480                | 25.0 (24), 1.4                                         | 24.6 (23), 1.4                      |
| JPEG 1600x720 / 1080x1920   | 6.5 (6), 4.6                                           | 6.4 (6), 4.5                        |

The OPPO's 4K H.264 was limited by the JavaScript hand-over (section 5.3), and the native path removes it: 30 fps, no
stall, 16 key frames in 20 s on the JavaScript path against 10 on the native one (each dropped frame forces one). Its JPEG
is limited by the encoder, so the path changes nothing.

**A reader that joins a running stream** (`/video.h264`, new connection at random moments, time from the request to
the first SPS):

| Phone, path                   | Joins | Median | Slowest                  |
| ----------------------------- | ----- | ------ | ------------------------ |
| iPhone 4K60, native (`vkey`)  | 10    | 110 ms | 121 ms                   |
| iPhone 4K60, JavaScript path  | 16    | 717 ms | 1880 ms (fastest 243 ms) |
| OPPO 864x480, native (`vkey`) | 16    | 43 ms  | 58 ms                    |
| OPPO 864x480, JavaScript path | 16    | 507 ms | 1942 ms                  |

Without `vkey` the reader waits for the next key frame, up to the 2 s interval. (A first run of the JavaScript
case with a fixed 1.37 s pause fell in step with the key frame interval and gave 620 ms every time; it was discarded
and the run repeated with random pauses.)

**Orientation flags** (read from the PC's debug log): the OPPO sends rotation 1 for the rear camera (on both paths)
and 3 for the front one (native path only) in H.264, mirror 0; its JPEG, like the iPhone's JPEG, is upright (rotation 0). The picture
itself was not looked at on a screen, so that these values make it upright is expected from the camera's metadata,
not seen. The iPhone's H.264 flags were not logged.

**Not measured**: the processor load and the temperature of either phone with the native path (the point of it was
less work on the JavaScript thread, which was not read), the latency from camera to screen, a run longer than
20 s per mode, the front camera's rate, and any other phone.

## 4. Method

- **Phones**: iPhone 13 Pro (band not shown by iOS); OPPO A9 2020 (CPH1941_11_F.98, 5 GHz, 433 Mbit/s, -25 dBm,
  battery 32 to 42 degrees C during the evening). Release builds of `main` plus temporary measuring code.
- **PC**: the `phoneset` program on a Mac (Wi-Fi 6, channel 64, 5 GHz, 80 MHz, -32 dBm, next to the router) with
  its development audio backend, which serves the camera on its local addresses (protocol section 9).
- **Reading the stream**: a script reads `/video.mjpg` (every part has `Content-Length` and `X-Phoneset-Seq`) or
  `/video.h264` (pictures counted from the slice NAL units that start one) for 25 s after 4 s of settling, one run per
  mode, back camera, app in front (iOS stops the camera in the background; an 8 s stall in an early run was most
  likely that, and it was repeated). Frame rate, bit rate, gaps between frames and, for JPEG, missing sequence numbers
  were read from the stream itself.
- **Phone-side counters**: temporary code logged once a second the frames the camera delivered, those coded, the
  coder's time, the time JavaScript took to hand a frame to the socket and the time to its acknowledgement. They are
  not in `main`, except that the Android H.264 source prints its own rate and queue with
  `adb shell setprop log.tag.PhonesetCamera VERBOSE`. The measuring scripts are not in the repository.
- **Settings**: JPEG quality 70 (`VIDEO_QUALITY`); H.264 about 0.1 bit per pixel per frame (at least 1.5 Mbit/s),
  High profile when accepted, no B-frames, a key frame every 2 s.
- **Reliability**: the same mode varied between runs by up to 40 percent (iPhone JPEG 2K60: 20.0, 20.3 and 12.7 fps;
  4K30: 7.7, 6.5, 5.0), so one run is not a figure to quote. Comparisons were therefore made back to back on the same
  scene (the window test).

## 5. What limits what

### 5.1 JPEG on the iPhone

- **The JPEG encoder is not the limit**: 2 to 6 ms per frame up to 2K, 16 to 18 ms at 4K. Sealing (AES-GCM, native)
  takes 0.1 to 0.3 ms, under 1 ms at 4K.
- **The window was a limit (fixed).** With 2 frames and 256 KB in flight, a frame over about 128 KB (1080p and up)
  was alone in flight and the next picture was asked for only after the PC acknowledged it. A cycle slightly longer
  than a camera frame period then misses the next camera frame, so the rate falls in steps (every second frame at
  30 fps: 15 fps at 2K30; every third at 60 fps: 20 fps at 2K60). At 1080p30 the phone's counters showed it: the
  next picture was asked for 30 ms after the last was handed over and 17.8 JPEGs a second were made out of the
  camera's 29.9; with 3 frames and 2 MB it is asked for after 0.1 ms and all 29.9 are made (table in appendix A.4).
- **The hand-over to the socket was the next limit (fixed by the native send path: section 3.1; this describes
  the JavaScript path that was used before it).** React Native's `WebSocket.send` turns a binary
  message into base64 in JavaScript (`binaryToBase64`) before `sendBinary` hands it to the native side, and the call
  returns without waiting for the network, so this is work on the phone. It costs 5.7 ms for a 66 KB frame, 13 ms for
  144 KB, 20 ms for 185 KB and about 160 ms for 749 KB (5 to 12 MB/s), plus a fixed cost of about 4 ms per message
  even for an 11 KB frame. At 1080p60 with the new window it takes about 97 percent of every second, so the rate stops
  where the JavaScript thread is full. The modes that did not reach their rate stop at 34 to 55 Mbit/s; for 2K and 4K
  the counters of that run are missing, so that they stop for the same reason is an inference.
- **A larger window costs latency**: the time from sending a frame to its acknowledgement rose (1080p60: 24 to 55 ms)
  because frames wait their turn. The time from camera to screen was not measured.

### 5.2 JPEG on the Android phone

The JPEG encoder is the limit: 28 ms per frame at 480p, 179 ms at 1080 x 1920, 644 ms at 1512 x 2688 and 2.1 s at 4K.
The Wi-Fi carried 2 to 4 Mbit/s of the phone's 433 Mbit/s link and the hand-over to the socket took 4 to 100 ms. At 480p
the camera itself delivered only 23.9 fps (not looked into).

### 5.3 H.264

- **The coder's delay matters, not its speed.** The iPhone's coder took 7 ms to give back a picture at 720p60, 11.5 ms at
  1080p60 and 37.5 ms at 4K30. With one picture in the coder at a time that delay became the frame period and 4K30
  ran at 15 fps (4K60 at 20); with two it ran at 25.4 (4K60 25.6, 1080p60 59.5, 2K60 60.0). The cost is latency: with two
  in the coder a 4K picture came back after 57 to 60 ms, not measured end to end.
- **What limited 4K on the OPPO was the JavaScript hand-over** (the native path of section 3.1 gives 30.0 fps and no
  stall); on the iPhone the native path stays at 25.7 fps, so what limits it there is the camera or the coder. The
  measurements of the JavaScript path: handing over a 100 to 170 KB frame took 28 to 46 ms (about 4 MB/s) on the
  iPhone when it had been busy for an hour, and 13 ms for 144 KB when cool. On the OPPO the coder made 27.9 frames a
  second at 4K but JavaScript took about 25, so about 4.6 a second were dropped in the phone's queue (208 in 45 s);
  each overflow forces a key frame (17 in 23 s instead of 12) and left 10 gaps over 100 ms, the longest 222 ms.
- **iPhone 720p60 stays at 56 fps** in both runs for a reason the counters do not show (the camera gave 59.6 a
  second, 56.2 were coded, pictures were being asked for in time).
- **The raw relay's resync rule** is rough: a reader that misses a frame waits for the next key frame, up to 2 s. A
  freeze of 1.9 s was seen once on the iPhone at 1080p60 (the phone's counters showed no gap). A virtual camera needs a
  better rule: a short queue per reader, or a key frame on request.
- **`ffmpeg -f h264` decodes the stream** with no error (High profile, yuv420p, upright 480 x 640 from the iPhone).

### 5.4 The link, the phone and the scene

- **The Wi-Fi was not the limit at the spots measured.** On the iPhone a 745 KB JPEG frame took 32 ms from the end of the
  hand-over to its acknowledgement, at least 23 MB/s or 186 Mbit/s; the OPPO used 2 to 21 Mbit/s of 433. Both were close
  to the router; a worse spot, 2.4 GHz and distance were not measured.
- **The phone slows down with use.** After about an hour of runs the iPhone's 720p60 control run gave 39.5 fps instead
  of 59.8, the hand-over 14.3 ms instead of 5.7 and the JPEG 4.5 ms instead of 2.6: native and JavaScript work were both
  about twice as slow. Heat is likely but the temperature was not read, so it is not proven. Hardware H.264 should
  lower the load; whether it holds for an hour was not measured.
- **A frame's size depends on the scene**: 1080p JPEG frames were 144 to 192 KB and 4K 626 to 942 KB across the
  evening. Hardware H.264 frames are 2 to 30 KB up to 2K60 and 100 to 200 KB at 4K.

## 6. Android specifics

- **The size coded is not the mode asked.** CameraX's Preview chooses a 16:9 size, the closest larger one, so 1600 x 720
  became 1920 x 1080 and two modes became 2688 x 1512. The coder is made for what CameraX gives and the frame header
  carries that size; the PC does not check it against the mode. JPEG had the same effect (three modes arrived as
  1080 x 1920 or 1512 x 2688).
- **The H.264 picture is not rotated**: a phone held across is upright (protocol 8.3 notes the exception). The iPhone's
  frames are upright for a phone held upright.
- **This phone's camera offers 30 fps only** through the module, so there is no 60 fps mode; why was not looked into.
- **One Qualcomm hardware coder** was tried; a software coder on another phone would be slower.

## 7. Other findings made on the way

- **Android pairing by the discovered PC failed** ("Couldn't reach the PC") on the OPPO while typing the PC's address
  worked. Cause (found with logs from the app): the PC's mDNS answer carried every address of every multicast
  interface, among them an alias on the Mac's loopback interface (10.77.0.20); an Android phone with Android 13 or older
  is given one address of a service, and it was that one. Fixed in `6caf56f` (only interfaces a phone can reach are
  advertised) and checked on the phone. An earlier note blamed a host name `R-M-9.local.local.` in the system log; that
  was a red herring.
- **Microphone and headphones on a real Android phone** (OPPO, with the Mac's test PC and its development audio
  backend): the permission prompt appears, the PC received real room audio, a foreground service and its notification run
  and the stream continued for 12 s with the app in the background; headphones on gave no error or crash. Not checked:
  sound quality, a real virtual cable, a locked screen, Android 14 or newer, an hour.
- **The iOS Simulator** has no camera (the camera switch stays off: "This phone can't send its camera yet") and the app
  crashed about 9 s after Headphones was switched on, with a CoreAudio "RPC timeout" in the log, three times out of
  three. On the iPhone the same step ran 55 s with the camera on and did not crash, so it looks specific to the
  Simulator; a build without the video changes was not tried there.
- **The live privacy policy** says the camera is used only on the QR screen, which is out of date now that the camera
  can be sent to the PC; a replacement paragraph is in `docs/store/google.md`.

## 8. Not tested

Latency from camera to a picture on the PC; a decoder on the PC and anything shown through it; a virtual camera; a run
longer than two minutes and the phones' temperature; 60 fps on Android; the front camera at the higher modes; a
locked screen; other iPhones and other Android phones (hardware coders vary); dark scenes; distance, 2.4 GHz and
other Wi-Fi; Wi-Fi capacity as such (the link test of protocol 8.8 is not built); Android on Android 14 or newer.
Of the native send path (section 3.1): its processor load and heat, its latency, the iPhone's H.264 orientation flags,
a repeat of the iPhone's H.264 480p60 run (46.4 fps and a 1.75 s freeze, once), the front camera's frame rate, and
that the rotation values really make the picture upright on a screen.

## 9. Options for what next (not decided)

1. **Measure the latency** from camera to a picture, with a millisecond counter on the Mac's screen filmed by the phone.
   It decides whether the rest is worth building, and it is independent of resolution.
2. **Show H.264 on the PC without a virtual camera**: OBS's media source may read `/video.h264`; it would give streamers
   a webcam early. Untested.
3. **Decoder and virtual camera on the PC** (Windows 11 `MFCreateVirtualCamera`, a camera extension on the Mac; the
   decoder from the operating system or ffmpeg). The main piece of work; the relay's resync rule needs replacing first.
4. **Native send path**: built, measured (section 3.1) and merged into `main`. It is what lets JPEG carry 4K30 on
   the iPhone and H.264 carry 4K30 on the OPPO, and it gives a joining reader a picture in about 100 ms. Its processor
   and heat cost is not measured.
5. **Mode choice and heat**: let the user pick a tier, lower it when the phone warms up (iOS can read
   `ProcessInfo.thermalState`), offer only modes the phone has been seen to carry, and make a mode name what the frame
   is on Android.
6. **Wider tests**: Android 60 fps and another Android phone, the front camera, 2.4 GHz and distance, an hour's run.

A suggested order, not a decision: 1, then 2 and 3, with 5 alongside; 4 (already built) before any promise of 4K; 6 when
a promise needs it.

## Appendix: the raw runs

Frame rate is the average over 25 s, the worst second in brackets; every JPEG run in every pass had 0 missing sequence
numbers.

### A.1 iPhone, JPEG, first pass (2-frame window, one run per mode)

| Mode    | fps (worst second) | Mbit/s | KB per frame | Notes                |
| ------- | ------------------ | ------ | ------------ | -------------------- |
| 480p30  | 30.1 (30)          | 8.5    | 34           |                      |
| 480p60  | 55.7 (22)          | 14.1   | 31           | 2 gaps over 100 ms   |
| 720p30  | 30.0 (29)          | 18.1   | 74           |                      |
| 720p60  | 59.4 (45)          | 35.3   | 73           | one gap of 259 ms    |
| 1080p30 | 28.8 (23)          | 37.6   | 159          |                      |
| 1080p60 | 20.7 (20)          | 27.4   | 161          |                      |
| 2K30    | 15.1 (14)          | 28.1   | 228          |                      |
| 2K60    | 20.0 (19)          | 38.6   | 235          |                      |
| 4K30    | 7.7 (6)            | 39.3   | 626          | 188 gaps over 100 ms |
| 4K60    | 4.7 (4)            | 30.6   | 788          | 118 gaps over 100 ms |

A longer run at 720p30 (120 s) held 30.0 fps and 16.8 Mbit/s with a longest gap of 70 ms. A first 1080p60 run was
spoiled by the app switching mode and was discarded.

### A.2 iPhone, JPEG, second pass with the phone's counters

| Mode    | fps  | Camera fps / JPEGs per s | Same mode, first pass |
| ------- | ---- | ------------------------ | --------------------- |
| 480p30  | 30.1 | 29.9 / 29.8              | 30.1                  |
| 480p60  | 60.0 | 59.8 / 59.7              | 55.7                  |
| 720p30  | 30.1 | 29.9 / 29.9              | 30.0                  |
| 720p60  | 59.9 | 59.9 / 59.7              | 59.4                  |
| 1080p30 | 29.7 | 29.9 / 29.2              | 28.8                  |
| 1080p60 | 29.9 | 59.9 / 29.7              | 20.7                  |
| 2K30    | 16.0 | 30.0 / 15.9              | 15.1                  |
| 2K60    | 20.3 | 59.8 / 19.9              | 20.0                  |
| 4K30    | 6.5  | 29.9 / 6.3               | 7.7                   |
| 4K60    | 4.5  | 59.9 / 4.5               | 4.7                   |

### A.3 iPhone, JPEG, where the time of one frame goes (per frame; cool phone)

| Mode    | KB per frame | Making the JPEG | Hand-over to the socket | Send to acknowledgement | fps  |
| ------- | ------------ | --------------- | ----------------------- | ----------------------- | ---- |
| 720p60  | 66           | 2.6 ms          | 5.7 ms                  | 15 ms                   | 59.9 |
| 1080p30 | 149          | 4.4 ms          | 13.8 ms                 | 25 ms                   | 29.7 |
| 1080p60 | 144          | 4.4 ms          | 12.8 ms                 | 24 ms                   | 29.9 |
| 2K30    | 185          | 5.7 ms          | 19.9 ms                 | 33 ms                   | 16.0 |
| 2K60    | 204          | 6.0 ms          | 27.5 ms (45 ms later)   | 41 ms                   | 20.3 |
| 4K30    | 634          | 16.0 ms         | 93 to 116 ms            | 121 to 167 ms           | 6.5  |
| 4K60    | 749          | 17.5 ms         | 156 to 159 ms           | 187 to 192 ms           | 4.5  |

### A.4 iPhone, JPEG, the window: old (2 frames, 256 KB) and new (3 frames, 2 MB) back to back

A control run at 720p60 with the old window gave 60.0 fps first, so the phone was not slowed down. The window was
switched by a temporary button.

| Mode    | KB per frame | Old window fps | New window fps | Old Mbit/s | New Mbit/s | Worst gap, new |
| ------- | ------------ | -------------- | -------------- | ---------- | ---------- | -------------- |
| 1080p30 | 190          | 16.8           | **30.1**       | 26.4       | 46.9       | 40 ms          |
| 1080p60 | 172          | 20.2           | **38.7**       | 28.3       | 54.6       | 345 ms         |
| 2K30    | 235          | 12.9           | **19.9**       | 25.0       | 38.4       | 69 ms          |
| 2K60    | 236          | 10.9           | **19.5**       | 21.1       | 37.7       | 122 ms         |
| 4K30    | 942          | 3.0            | **4.4**        | 22.9       | 34.0       | 348 ms         |

### A.5 iPhone, H.264: one picture in the coder, and two

| Mode    | One picture: fps        | Mbit/s | KB/frame | Two pictures: fps | Mbit/s | KB/frame |
| ------- | ----------------------- | ------ | -------- | ----------------- | ------ | -------- |
| 480p30  | 30.0                    | 0.5    | 2        |                   |        |          |
| 480p60  | 59.0                    | 1.6    | 3        |                   |        |          |
| 720p30  | 29.8                    | 2.6    | 11       |                   |        |          |
| 720p60  | 56.5                    | 5.0    | 11       | 56.4              | 5.3    | 11       |
| 1080p30 | 30.0                    | 6.2    | 25       | 30.0              | 6.2    | 25       |
| 1080p60 | 53.9 (one 1.9 s freeze) | 9.5    | 21       | **59.5**          | 10.8   | 22       |
| 2K30    | 30.0                    | 2.7    | 11       | 30.0              | 2.4    | 10       |
| 2K60    | 59.0                    | 13.8   | 29       | **60.0**          | 14.4   | 29       |
| 4K30    | 15.0                    | 24.9   | 202      | **25.4**          | 24.9   | 120      |
| 4K60    | 20.0                    | 27.9   | 170      | **25.6**          | 34.5   | 164      |

Coder delay (hand-over to coded frame) with one picture: 7.1 ms at 720p60, 11.5 ms at 1080p60, 37.5 ms at 4K30; with
two pictures 57 ms at 4K30 and 59.5 ms at 4K60.

### A.6 OPPO A9 2020, JPEG

| Mode offered | Frame received | fps (worst second) | Mbit/s | KB per frame | Making the JPEG |
| ------------ | -------------- | ------------------ | ------ | ------------ | --------------- |
| 864x480@30   | 480x864        | 24.2 (19)          | 2.8    | 14           | 28.5 ms         |
| 1600x720@30  | 1080x1920      | 5.4 (1)            | 4.4    | 99           | 179 ms          |
| 2400x1080@30 | 1512x2688      | 1.3 (1)            | 2.5    | 235          | 644 ms          |
| 1920x1440@30 | 1512x2688      | 1.6 (1)            | 3.0    | 235          | not read        |
| 3840x2160@30 | 2160x3840      | 0.4 (1)            | 2.0    | 566          | 2124 ms         |

### A.7 OPPO A9 2020, H.264

| Mode asked | Size coded | fps (worst second) | Mbit/s | KB per frame | Key frames in 23 to 25 s | Gap over 100 ms    |
| ---------- | ---------- | ------------------ | ------ | ------------ | ------------------------ | ------------------ |
| 864x480    | 864x480    | 30.0 (30)          | 1.5    | 6            | 12                       | none               |
| 1600x720   | 1920x1080  | 30.0 (29)          | 6.2    | 25           | 13                       | none               |
| 2400x1080  | 2688x1512  | 30.1 (27)          | 12.2   | 50           | 12                       | one, 260 ms        |
| 1920x1440  | 2688x1512  | 30.1 (29)          | 12.2   | 50           | 12                       | none               |
| 3840x2160  | 3840x2160  | 25.4 (22)          | 21.1   | 101          | 17                       | 10, longest 222 ms |

The raw stream has no sequence numbers, so lost frames were not counted for H.264.

### A.8 The native send path, raw lines of the measuring script (2026-11-12)

One line per mode, 20 s each (a run that was cut short says so); "requested" is the mode asked for. The first block is
the iPhone with H.264, native path (#7 is JPEG 720p30, and the last line, #15, is the JavaScript path run right after); the second the iPhone
with JPEG, native path first (lines #7 to #12) then the JavaScript path (#13 to #18); the third the OPPO, native
path first (#17 to #23, the first of them JPEG) then the JavaScript path (#24 to #30). The "stalls" count gaps over 100 ms.

```
#3 back h264 requested 640x480@30 | 29.4 fps (min 25, median 30) | 0.5 Mbit/s | 2 KB/frame | key frames 11 | gap p95 41 ms max 164 ms | stalls>100ms 5 | measured 20s | starts with SPS/PPS
#5 back h264 requested 640x480@60 | 46.4 fps (min 45, median 49.5) | 0.7 Mbit/s | 2 KB/frame | key frames 13 | gap p95 55 ms max 1753 ms | stalls>100ms 11 | measured 20s | starts with SPS/PPS
#6 back h264 requested 1280x720@30 | 29.6 fps (min 28, median 30) | 0.9 Mbit/s | 4 KB/frame | key frames 11 | gap p95 70 ms max 133 ms | stalls>100ms 8 | measured 20s | starts with SPS/PPS
#7 back jpeg requested 1280x720@30 | 28.3 fps (min 25, median 29) | 16.3 Mbit/s | 70 KB/frame | key frames 568 | gap p95 71 ms max 240 ms | stalls>100ms 15 | measured 20s | 
#8 back h264 requested 1280x720@60 | 57.5 fps (min 51, median 58) | 1.8 Mbit/s | 4 KB/frame | key frames 11 | gap p95 21 ms max 84 ms | stalls>100ms 0 | measured 20s | starts with SPS/PPS
#9 back h264 requested 1920x1080@30 | 30.0 fps (min 29, median 30) | 1.3 Mbit/s | 5 KB/frame | key frames 10 | gap p95 36 ms max 56 ms | stalls>100ms 0 | measured 20s | starts with SPS/PPS
#10 back h264 requested 1920x1080@60 | 58.5 fps (min 55, median 58) | 3.3 Mbit/s | 7 KB/frame | key frames 11 | gap p95 20 ms max 36 ms | stalls>100ms 0 | measured 20s | starts with SPS/PPS
#11 back h264 requested 1920x1440@30 | 30.0 fps (min 30, median 30) | 0.9 Mbit/s | 4 KB/frame | key frames 11 | gap p95 35 ms max 39 ms | stalls>100ms 0 | measured 20s | starts with SPS/PPS
#12 back h264 requested 1920x1440@60 | 59.6 fps (min 58, median 60) | 2.5 Mbit/s | 5 KB/frame | key frames 11 | gap p95 19 ms max 62 ms | stalls>100ms 0 | measured 20s | starts with SPS/PPS
#13 back h264 requested 3840x2160@30 | 25.7 fps (min 25, median 26) | 3.8 Mbit/s | 18 KB/frame | key frames 10 | gap p95 40 ms max 49 ms | stalls>100ms 0 | measured 20s | starts with SPS/PPS
#14 back h264 requested 3840x2160@60 | 25.6 fps (min 25, median 26) | 41.7 Mbit/s | 199 KB/frame | key frames 11 | gap p95 42 ms max 48 ms | stalls>100ms 0 | measured 20s | starts with SPS/PPS
#15 back h264 requested 3840x2160@60 | 22.4 fps (min 18, median 22) | 37.7 Mbit/s | 205 KB/frame | key frames 10 | gap p95 63 ms max 96 ms | stalls>100ms 0 | measured 19s | starts with SPS/PPS
```

```
#7 back jpeg requested 1920x1080@60 | 58.6 fps (min 56, median 59) | 71.7 Mbit/s | 149 KB/frame | key frames 1173 | gap p95 22 ms max 48 ms | stalls>100ms 0 | measured 20s | 
#8 back jpeg requested 1920x1440@30 | 30.1 fps (min 30, median 30) | 45.4 Mbit/s | 184 KB/frame | key frames 602 | gap p95 37 ms max 41 ms | stalls>100ms 0 | measured 20s | 
#9 back jpeg requested 1920x1440@60 | 59.4 fps (min 57, median 60) | 94.5 Mbit/s | 194 KB/frame | key frames 1188 | gap p95 20 ms max 41 ms | stalls>100ms 0 | measured 20s | 
#10 back jpeg requested 3840x2160@30 | 30.0 fps (min 29, median 30) | 182.0 Mbit/s | 740 KB/frame | key frames 601 | gap p95 41 ms max 79 ms | stalls>100ms 0 | measured 20s | 
#11 back jpeg requested 3840x2160@60 | 30.1 fps (min 23, median 30) | 193.7 Mbit/s | 787 KB/frame | key frames 602 | gap p95 56 ms max 100 ms | stalls>100ms 1 | measured 20s | 
#12 back jpeg requested 1920x1080@30 | 30.1 fps (min 30, median 30) | 37.2 Mbit/s | 151 KB/frame | key frames 602 | gap p95 37 ms max 39 ms | stalls>100ms 0 | measured 20s | 
#13 back jpeg requested 1920x1080@30 | 30.1 fps (min 29, median 30) | 37.2 Mbit/s | 151 KB/frame | key frames 602 | gap p95 36 ms max 56 ms | stalls>100ms 0 | measured 20s | 
#14 back jpeg requested 1920x1080@60 | 40.2 fps (min 37, median 40) | 49.5 Mbit/s | 150 KB/frame | key frames 805 | gap p95 31 ms max 37 ms | stalls>100ms 0 | measured 20s | 
#15 back jpeg requested 1920x1440@30 | 25.4 fps (min 23, median 25) | 37.9 Mbit/s | 182 KB/frame | key frames 508 | gap p95 47 ms max 66 ms | stalls>100ms 0 | measured 20s | 
#16 back jpeg requested 1920x1440@60 | 24.4 fps (min 24, median 25) | 38.8 Mbit/s | 194 KB/frame | key frames 489 | gap p95 50 ms max 60 ms | stalls>100ms 0 | measured 20s | 
#17 back jpeg requested 3840x2160@30 | 5.7 fps (min 5, median 6) | 34.4 Mbit/s | 735 KB/frame | key frames 115 | gap p95 214 ms max 219 ms | stalls>100ms 114 | measured 20s | 
#18 back jpeg requested 3840x2160@60 | 5.3 fps (min 4, median 5) | 34.2 Mbit/s | 787 KB/frame | key frames 107 | gap p95 242 ms max 441 ms | stalls>100ms 103 | measured 20s | 
```

```
#17 back jpeg requested 864x480@30 | 25.0 fps (min 24, median 25) | 1.4 Mbit/s | 7 KB/frame | key frames 501 | gap p95 44 ms max 48 ms | stalls>100ms 0 | measured 20s | 
#18 back h264 requested 1600x720@30 | 30.0 fps (min 29, median 30) | 6.2 Mbit/s | 25 KB/frame | key frames 10 | gap p95 45 ms max 68 ms | stalls>100ms 0 | measured 20s | starts with SPS/PPS
#19 back jpeg requested 1600x720@30 | 6.5 fps (min 6, median 6) | 4.6 Mbit/s | 86 KB/frame | key frames 130 | gap p95 167 ms max 189 ms | stalls>100ms 128 | measured 20s | 
#20 back h264 requested 2400x1080@30 | 30.0 fps (min 29, median 30) | 12.2 Mbit/s | 50 KB/frame | key frames 10 | gap p95 46 ms max 63 ms | stalls>100ms 0 | measured 20s | starts with SPS/PPS
#21 back h264 requested 1920x1440@30 | 30.0 fps (min 29, median 30) | 12.2 Mbit/s | 50 KB/frame | key frames 10 | gap p95 43 ms max 58 ms | stalls>100ms 0 | measured 20s | starts with SPS/PPS
#22 back h264 requested 3840x2160@30 | 30.0 fps (min 29, median 30) | 24.9 Mbit/s | 101 KB/frame | key frames 10 | gap p95 40 ms max 58 ms | stalls>100ms 0 | measured 20s | starts with SPS/PPS
#23 back h264 requested 864x480@30 | 30.1 fps (min 30, median 30) | 1.4 Mbit/s | 6 KB/frame | key frames 10 | gap p95 37 ms max 42 ms | stalls>100ms 0 | measured 20s | starts with SPS/PPS
#24 back h264 requested 864x480@30 | 30.0 fps (min 30, median 30) | 1.4 Mbit/s | 6 KB/frame | key frames 10 | gap p95 40 ms max 56 ms | stalls>100ms 0 | measured 20s | starts with SPS/PPS
#25 back jpeg requested 864x480@30 | 24.6 fps (min 23, median 25) | 1.4 Mbit/s | 7 KB/frame | key frames 493 | gap p95 48 ms max 74 ms | stalls>100ms 0 | measured 20s | 
#26 back h264 requested 1600x720@30 | 30.0 fps (min 30, median 30) | 6.2 Mbit/s | 25 KB/frame | key frames 10 | gap p95 49 ms max 63 ms | stalls>100ms 0 | measured 20s | starts with SPS/PPS
#27 back jpeg requested 1600x720@30 | 6.4 fps (min 6, median 6) | 4.5 Mbit/s | 86 KB/frame | key frames 128 | gap p95 179 ms max 199 ms | stalls>100ms 127 | measured 20s | 
#28 back h264 requested 2400x1080@30 | 30.1 fps (min 29, median 30.0) | 12.2 Mbit/s | 50 KB/frame | key frames 10 | gap p95 59 ms max 107 ms | stalls>100ms 2 | measured 18s | starts with SPS/PPS
#29 back h264 requested 1920x1440@30 | 30.1 fps (min 30, median 30.0) | 12.2 Mbit/s | 50 KB/frame | key frames 10 | gap p95 51 ms max 81 ms | stalls>100ms 0 | measured 18s | starts with SPS/PPS
#30 back h264 requested 3840x2160@30 | 25.3 fps (min 21, median 26) | 21.0 Mbit/s | 101 KB/frame | key frames 16 | gap p95 50 ms max 219 ms | stalls>100ms 9 | measured 20s | starts with SPS/PPS
```
