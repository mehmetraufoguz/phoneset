# Phone camera as a PC webcam: findings and measurements

All of this was measured on 2026-11-11 with the prototype in `main` (a video connection of its own, protocol
section 8), on two phones, one Mac and one Wi-Fi. They are numbers from one room, not guarantees; what each number
does and does not show is written next to it. Sections 1 to 3 are the short version, sections 4 to 6 the evidence,
the appendix the raw runs.

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
  every message (not fixed), and on the Android phone the JPEG encoder itself. **What limits H.264 at 4K** is the
  same JavaScript hand-over of a 100 to 170 KB frame, and on the iPhone the coder's delay, which keeping two pictures
  in the coder removed.
- **Still open**: the latency from the camera to a picture on the PC (never measured), a decoder on the PC and a
  virtual camera (neither exists), a run of an hour on a phone that warms up (the iPhone ran about twice as slow
  after an hour), other phones, 2.4 GHz and distance.

## 2. What the app does now (state of `main`)

| Area           | State                                                                                                                                                                |
| -------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Default mode   | JPEG 720p30, on both platforms (the gate `CODEC_TRIED` in `mobile/src/core/videoModes.ts` offers JPEG up to 720p30)                                                  |
| H.264          | iOS and Android code it in hardware and offer it in `modes` up to 4K at 60; it is picked only by a test build (a temporary "Next mode" button that is not in `main`) |
| PC side        | accepts `jpeg` and `h264`, shows JPEG as MJPEG at `/video.mjpg` and `/video`, passes H.264 on as a raw Annex B stream at `/video.h264`; it does not decode H.264     |
| Window         | 3 frames and 2 MB in flight (it was 2 frames and 256 KB)                                                                                                             |
| Coder pipeline | 2 pictures asked for ahead, for H.264 only                                                                                                                           |
| Not built      | a decoder on the PC, a virtual camera, the link test of protocol 8.8, the subscription, a mode picker, thermal-aware mode changes                                    |

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
- **The hand-over to the socket is the next limit (not fixed).** React Native's `WebSocket.send` turns a binary
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
- **What still limits 4K is the JavaScript hand-over** of a 100 to 170 KB frame (28 to 46 ms, about 4 MB/s on the
  iPhone when it had been busy for an hour, 13 ms for 144 KB when cool). On the OPPO the coder made 27.9 frames a
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
other Wi-Fi; Wi-Fi capacity as such (the link test of protocol 8.8 is not built); a native socket instead of React
Native's `WebSocket`; Android on Android 14 or newer.

## 9. Options for what next (not decided)

1. **Measure the latency** from camera to a picture, with a millisecond counter on the Mac's screen filmed by the phone.
   It decides whether the rest is worth building, and it is independent of resolution.
2. **Show H.264 on the PC without a virtual camera**: OBS's media source may read `/video.h264`; it would give streamers
   a webcam early. Untested.
3. **Decoder and virtual camera on the PC** (Windows 11 `MFCreateVirtualCamera`, a camera extension on the Mac; the
   decoder from the operating system or ffmpeg). The main piece of work; the relay's resync rule needs replacing first.
4. **Native send path** (frames handed to the network from Swift and Kotlin instead of through JavaScript), only if
   4K is to be promised; it would take the base64 work off the JavaScript thread.
5. **Mode choice and heat**: let the user pick a tier, lower it when the phone warms up (iOS can read
   `ProcessInfo.thermalState`), offer only modes the phone has been seen to carry, and make a mode name what the frame
   is on Android.
6. **Wider tests**: Android 60 fps and another Android phone, the front camera, 2.4 GHz and distance, an hour's run.

A suggested order, not a decision: 1, then 2 and 3, with 5 alongside; 4 and 6 when a promise needs them.

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
