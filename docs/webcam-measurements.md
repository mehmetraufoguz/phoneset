# Phone camera over Wi-Fi: measurements

Measured on 2026-11-11 with the prototype in `main` (JPEG frames over the video connection, protocol section 8).
These are numbers from one phone, one Mac and one room, not a guarantee. What each number does and does not
show is written next to it.

## Setup

- Phone: iPhone 13 Pro, back camera, a Release build of `main` plus the format selection described below.
- PC side: the `phoneset` program on a Mac with its development audio backend (no real audio), taking the
  frames and serving them on its local MJPEG address (`phoneset video --mjpeg`, section 9).
- Link: both on the same Wi-Fi. The Mac was on Wi-Fi 6, channel 64 (5 GHz, 80 MHz), signal -32 dBm, next to
  the router. The phone's band is not known (iOS does not show it).
- Method: for each mode the camera was started, 4 s were left to settle, then the MJPEG stream was read for
  25 s. Every part carries `Content-Length` and `X-Phoneset-Seq`, so frame rate, bytes, the gaps between
  frames and missing sequence numbers were read from the stream itself. The app was in the foreground the
  whole time (iOS stops the camera in the background; an 8 s stall in an early run was most likely that,
  and the run was repeated).
- For the second and third pass the phone also logged its own timings once a second, read from the device
  console. They were temporary measuring code and are not in `main`.
- JPEG quality 70 (`VIDEO_QUALITY`). The phone holds the picture upright, so a 1920 x 1080 mode arrives
  as 1080 x 1920.

## What had to change first

The iOS camera module ignored the size it was asked for: it chose the 1280 x 720 or the 640 x 480 preset
from the width alone, so nothing above 720p had ever been captured, whatever the mode said. It now picks the
camera format that has exactly the requested size and can run at the requested rate, and sets the frame
duration (`PhonesetCameraCore.format(of:width:height:fps:)`). With that, frames arrive at the sizes asked
for. The Android module already asks CameraX for a size but sets no frame rate; it was not measured here.

## Results

First pass, all ten modes. Frame rate is the average over 25 s (the worst single second in brackets). Every
run in every pass had 0 missing sequence numbers.

| Mode             | fps (worst second) | Mbit/s | KB per frame | Notes                                   |
| ---------------- | ------------------ | ------ | ------------ | --------------------------------------- |
| 480p30           | 30.1 (30)          | 8.5    | 34           |                                         |
| 480p60           | 55.7 (22)          | 14.1   | 31           | 2 gaps over 100 ms                      |
| 720p30           | 30.0 (29)          | 18.1   | 74           |                                         |
| 720p60           | 59.4 (45)          | 35.3   | 73           | one gap of 259 ms                       |
| 1080p30          | 28.8 (23)          | 37.6   | 159          |                                         |
| 1080p60          | 20.7 (20)          | 27.4   | 161          | see the second and third pass           |
| 2K30 (1920x1440) | 15.1 (14)          | 28.1   | 228          |                                         |
| 2K60             | 20.0 (19)          | 38.6   | 235          |                                         |
| 4K30             | 7.7 (6)            | 39.3   | 626          | 188 gaps over 100 ms                    |
| 4K60             | 4.7 (4)            | 30.6   | 788          | 118 gaps over 100 ms                    |

A longer run at 720p30 (120 s) held 30.0 fps, 16.8 Mbit/s, with a longest gap of 70 ms.

Second pass, the same modes again (fps; JPEG made per second out of the camera's own rate):

| Mode    | fps  | Camera fps / JPEGs per s | Same mode in the first pass |
| ------- | ---- | ------------------------ | --------------------------- |
| 480p30  | 30.1 | 29.9 / 29.8              | 30.1                        |
| 480p60  | 60.0 | 59.8 / 59.7              | 55.7                        |
| 720p30  | 30.1 | 29.9 / 29.9              | 30.0                        |
| 720p60  | 59.9 | 59.9 / 59.7              | 59.4                        |
| 1080p30 | 29.7 | 29.9 / 29.2              | 28.8                        |
| 1080p60 | 29.9 | 59.9 / 29.7              | 20.7                        |
| 2K30    | 16.0 | 30.0 / 15.9              | 15.1                        |
| 2K60    | 20.3 | 59.8 / 19.9              | 20.0                        |
| 4K30    | 6.5  | 29.9 / 6.3               | 7.7                         |
| 4K60    | 4.5  | 59.9 / 4.5               | 4.7                         |

The first pass's 1080p60 figure was measured while the app was switching to the next mode and is not
trusted; the second and third pass agree on about 30 fps.

## Where the time of one frame goes

The phone logged, per frame and averaged over the run (a cool phone, second pass; the hand-over column is from
the third pass, run a few minutes later):

| Mode    | KB per frame | Making the JPEG | Hand-over to the socket | Send to acknowledgement | fps  |
| ------- | ------------ | --------------- | ----------------------- | ----------------------- | ---- |
| 720p60  | 66           | 2.6 ms          | 5.7 ms                  | 15 ms                   | 59.9 |
| 1080p30 | 149          | 4.4 ms          | 13.8 ms                 | 25 ms                   | 29.7 |
| 1080p60 | 144          | 4.4 ms          | 12.8 ms                 | 24 ms                   | 29.9 |
| 2K30    | 185          | 5.7 ms          | 19.9 ms                 | 33 ms                   | 16.0 |
| 2K60    | 204          | 6.0 ms          | 27.5 ms (45 ms later)   | 41 ms                   | 20.3 |
| 4K30    | 634          | 16.0 ms         | 93 to 116 ms            | 121 to 167 ms           | 6.5  |
| 4K60    | 749          | 17.5 ms         | 156 to 159 ms           | 187 to 192 ms           | 4.5  |

"Hand-over to the socket" is the time `WebSocket.send` takes in JavaScript. The sealing (AES-GCM, native) took
0.1 to 0.3 ms per frame up to 2K and under 1 ms at 4K; copying the message together took 0.0 ms.

What this shows:

- **The JPEG encoder is not the limit.** 2 to 6 ms up to 2K, 16 to 18 ms at 4K.
- **The hand-over to the socket is the biggest cost of a frame**, and it grows with the frame: 5.7 ms for 66 KB,
  13 ms for 144 KB, 20 ms for 185 KB, about 160 ms for 749 KB, a rate of 5 to 12 MB/s. `WebSocket.send` returns
  without waiting for the network, so this is work on the phone, not the link. In React Native's
  `WebSocket.send` a binary message is turned into base64 in JavaScript (`binaryToBase64`) before
  `sendBinary` hands it to the native side.
- **Up to 720p60 two frames are in flight and the pipeline keeps up.** The next frame is asked for less than
  1 ms after the last one was handed over. From 1080p on, a frame is over half of the 256 KB window of
  `VIDEO_WINDOW`, only one frame may be in flight, and the next one is asked for only after the PC has
  acknowledged the last (the time to ask is the same as the acknowledgement time in the table). One cycle is
  then the wait for the camera, plus the JPEG, plus the hand-over and the acknowledgement: 1080p60 is
  5.3 + 4.4 + 23.9 = 33.6 ms, which is 29.7 fps.
- **That explains the steps in the frame rate.** A cycle a little longer than one camera frame (33.3 ms at 30 fps)
  misses the next camera frame and waits for the one after it, so half of the frames are used (15 fps at 2K30);
  at 60 fps a cycle between 33 and 50 ms uses every third (20 fps at 2K60, and 1080p60 only reaching
  30 fps).
- **The Wi-Fi was not the limit at this spot.** Between the end of the hand-over and the acknowledgement a
  745 KB frame took 32 ms in the 4K60 mode, which is at least 23 MB/s, about 186 Mbit/s, on this link, close to the
  router. A worse spot may well be slower; that was not measured.
- **The phone slows down with use.** After about an hour of runs, mostly with the camera at the higher modes,
  720p60 was measured again with the same 65 KB frames: 39.5 fps instead of 59.8, the hand-over 14.3 ms
  instead of 5.7 ms and the JPEG 4.5 ms instead of 2.6 ms. Both the native and the JavaScript work were about
  twice as slow. Heat or another load on the phone is the likely reason, but the phone's temperature was not read,
  so it is not proven. The same mode also varied between runs by up to 40 percent, so one run is not a figure
  to quote: 2K60 gave 20.0, 20.3 and 12.7 fps, 4K30 7.7, 6.5 and 5.0.

## A larger window (iPhone, old and new side by side)

The window of section 8.4 (`VIDEO_WINDOW`: 2 frames and about 256 KB in flight) was the suspect for the steps
in the frame rate. Later the same evening, with a clean build and a cool phone, each mode was measured twice
in a row with the old window (2 frames, 256 KB) and a new one (3 frames, 2 MB), switched by a temporary button
on the session screen, so that the scene and the phone's state were the same for both. A control run at 720p60
with the old window gave 60.0 fps, the same as the first time, so the phone was not slowed down.

| Mode    | KB per frame | Old window fps | New window fps | Old Mbit/s | New Mbit/s | Worst gap, new |
| ------- | ------------ | -------------- | -------------- | ---------- | ---------- | -------------- |
| 1080p30 | 190          | 16.8           | **30.1**       | 26.4       | 46.9       | 40 ms          |
| 1080p60 | 172          | 20.2           | **38.7**       | 28.3       | 54.6       | 345 ms         |
| 2K30    | 235          | 12.9           | **19.9**       | 25.0       | 38.4       | 69 ms          |
| 2K60    | 236          | 10.9           | **19.5**       | 21.1       | 37.7       | 122 ms         |
| 4K30    | 942          | 3.0            | **4.4**        | 22.9       | 34.0       | 348 ms         |

What this shows:

- **The window was a limit, and widening it helps every mode above 1080p.** At 1080p30 the phone's counters
  show why: with the old window the next frame was asked for 30 ms after the last was handed over (about the
  acknowledgement time), 17.8 JPEGs a second were made out of the camera's 29.9; with the new one it was asked
  for after 0.1 ms and all 29.9 were made. 1080p30 now holds the full 30 fps (worst second 29).
- **The next limit looks like the JavaScript thread.** With the new window the hand-over to the socket takes
  about 16 ms per frame at 1080p30 (30 frames a second, under half of every second) and 27 ms at 1080p60 (172 KB);
  at 36.5 frames a second that is about 97 percent of every second, so 1080p60 stops where the thread is full.
  The modes that did not reach their rate all stop at 34 to 55 Mbit/s, which is 4 to 7 MB/s; for 2K and 4K the
  counters of the new run are missing, so that they stop for the same reason is an inference. This is the
  base64 work of the section "Where the time of one frame goes", now unblocked.
- **A larger window costs latency.** The time from sending a frame to its acknowledgement rose (1080p60: 24 to
  55 ms) because frames wait their turn. The time from the camera to the screen was not measured.
- **A frame's size depends on the scene.** The same modes made 144 to 159 KB frames at 1080p and 626 to 749 KB at
  4K earlier in the evening, and 172 to 192 KB and 940 KB here, so every figure here is for one scene; that
  is why each pair was measured back to back.

Not measured in this part: the phone's counters for 2K and 4K (the console capture stopped), 720p with the new
window, and any Wi-Fi other than the one above.

## Android: OPPO A9 2020

The same evening, with the same Mac and Wi-Fi: an OPPO A9 2020 (CPH1941), Android 11, Snapdragon 665, a Release
build of `main` plus the temporary measuring code. The phone was on 5 GHz at 433 Mbit/s, signal -25 dBm. Its
battery temperature, as the phone reports it, went from 31.6 to 41.6 degrees C during the runs. The phone logged
the same kind of counters as the iPhone did, to logcat.

The Android module offered five modes, all at 30 fps (what the module reports for this camera; why there is no
60 fps mode was not looked into). Same method as above, 25 s per mode, back camera:

| Mode offered | Frame received | fps (worst second) | Mbit/s | KB per frame | Making the JPEG |
| ------------ | -------------- | ------------------ | ------ | ------------ | --------------- |
| 864x480@30   | 480x864        | 24.2 (19)          | 2.8    | 14           | 28.5 ms         |
| 1600x720@30  | 1080x1920      | 5.4 (1)            | 4.4    | 99           | 179 ms          |
| 2400x1080@30 | 1512x2688      | 1.3 (1)            | 2.5    | 235          | 644 ms          |
| 1920x1440@30 | 1512x2688      | 1.6 (1)            | 3.0    | 235          | not read        |
| 3840x2160@30 | 2160x3840      | 0.4 (1)            | 2.0    | 566          | 2124 ms         |

Every run had 0 missing sequence numbers.

What this shows:

- **On this phone only the smallest mode is usable, and the limit is making the JPEG.** 28 ms at 480p, 179 ms at
  1080 x 1920, 644 ms at 1512 x 2688, over 2 s at 4K. The Wi-Fi carried 2 to 4 Mbit/s of a 433 Mbit/s link and the
  hand-over to the socket was 4 to 100 ms, so the link was not what limited it. At 480p the camera itself delivered
  only 23.9 fps; why was not looked into.
- **The frame is not the size of the mode.** The module asks CameraX for a size with a 16:9 aspect ratio and the
  closest larger one, so the two 20:9 sizes and the 4:3 size it offered arrived as 1920 x 1080 and 2688 x 1512
  (two modes ended up as the same 2688 x 1512). The frame header carries the true size and the PC does not check
  it against the mode, so nothing broke, but a mode should name what the frame is.
- **The phone's own discovery did not work.** It found the PC but resolved its name as `R-M-9.local.local.`, and
  pairing by code ended with "Couldn't reach the PC"; typing the PC's address worked. This is about discovery,
  not video, and was not investigated further.

## H.264 in hardware (iPhone, a prototype)

Later the same night the iPhone was given a hardware H.264 coder (VideoToolbox, High profile, no B-frames, a key
frame every 2 s, about 0.1 bit per pixel per frame as the target bit rate) to see whether it lifts the limits
JPEG has. The payload is defined in protocol section 8.3; the PC passes the frames on unchanged and serves
them as a raw Annex B stream (`/video.h264`); nothing decodes them in the program yet. The same Wi-Fi, the
same phone, the 3-frame window, 25 s per mode, the back camera. `ffmpeg -f h264 -i <address>` decoded six seconds
of the stream with no error (High profile, yuv420p, upright 480x640). Frame rate and bit rate are read from
the stream on the PC side (pictures are counted from the slice NAL units that start one).

| Mode    | JPEG fps | JPEG Mbit/s | JPEG KB/frame | H.264 fps | H.264 Mbit/s | H.264 KB/frame | H.264 with 2 pictures in the coder |
| ------- | -------- | ----------- | ------------- | --------- | ------------ | -------------- | ---------------------------------- |
| 480p30  | 30.1     | 7.4         | 30            | 30.0      | 0.5          | 2              |                                    |
| 480p60  | 60.0     | 14.3        | 29            | 59.0      | 1.6          | 3              |                                    |
| 720p30  | 30.1     | 16.6        | 67            | 29.8      | 2.6          | 11             |                                    |
| 720p60  | 59.9     | 32.7        | 67            | 56.5      | 5.0          | 11             | 56.4 (5.3 Mbit/s)                  |
| 1080p30 | 30.1     | 46.9        | 190           | 30.0      | 6.2          | 25             | 30.0 (6.2)                         |
| 1080p60 | 38.7     | 54.6        | 172           | 53.9      | 9.5          | 21             | **59.5** (10.8)                    |
| 2K30    | 19.9     | 38.4        | 235           | 30.0      | 2.7          | 11             | 30.0 (2.4)                         |
| 2K60    | 19.5     | 37.7        | 236           | 59.0      | 13.8         | 29             | **60.0** (14.4)                    |
| 4K30    | 4.4      | 34.0        | 942           | 15.0      | 24.9         | 202            | **25.4** (24.9)                    |
| 4K60    | 4.7      | 28.7        | 745           | 20.0      | 27.9         | 170            | **25.6** (34.5)                    |

The JPEG columns are the best runs of the earlier sections (the 3-frame window for 1080p to 4K30, the 2-frame window
for the modes below and for 4K60, a cool phone); the scene was not the same in every run, so frame sizes differ a
little. "With 2 pictures
in the coder" is a change to how the phone asks for pictures: the next picture is asked for before the last one
has come back from the coder, so that the coder's own delay does not become the frame period.

What this shows:

- **The bit rate is 5 to 15 times lower at the same size**: 1080p30 6.2 against 36 to 47 Mbit/s, 2K60 14 against 38,
  4K30 25 against 34 at a fifth of the frame rate. Frames are 2 to 30 KB up to 2K60 where JPEG made 30 to 240 KB.
- **Every mode up to 2K60 reaches its rate**, 720p60 at 56 of 60 and the rest at 30 or 59 to 60, and in the second
  pass none of them had a 100 ms stall. 4K reaches 25 fps. JPEG reached its rate only up to 720p60 and 1080p30.
- **The coder's delay matters, not its speed.** At 4K30 the phone logged 37.5 ms from handing a picture over to
  getting the coded frame back (7 ms at 720p60, 11.5 ms at 1080p60). With one picture in the coder at a time that delay
  became the frame period and 4K30 ran at 15 fps; with two it ran at 25.4 (the coded frame took 57 ms
  to come back, the cost is that much more latency, which was not measured).
- **What still limits 4K** looks like the JavaScript hand-over of a 120 to 170 KB frame (28 to 46 ms, about 4 MB/s when
  the phone had been busy for an hour; it was 13 ms for 144 KB when cool) together with the coder's delay.
  A fixed cost of about 4 ms per message showed even for an 11 KB frame, which at 60 fps is a quarter of the
  JavaScript thread. 720p60 stays at 56 fps in both runs for a reason the counters do not show (the camera
  gave 59.6, 56.2 were coded, pictures were being asked for in time).
- **One freeze of 1.9 s** was seen in the first 1080p60 run: the reader on the PC missed a frame and, by the rule
  of `/video.h264`, waited for the next key frame. The phone's counters showed no gap. A real consumer (the virtual
  camera) needs a better rule than waiting up to 2 s: a short queue per reader, or a key frame on request.

Not measured here: how long a run holds once the phone has warmed (the later modes ran on a warm phone), the latency
from the camera to a picture on the PC, the PC's decoding, other iPhones (the hardware coder is assumed on every
supported model), Android (MediaCodec was not tried), the front camera, dark scenes (the bit rate depends on the
scene; these runs had one) and a worse Wi-Fi.

## Not tested

Wi-Fi capacity as such (the link test of protocol section 8.8 is not built), distance, 2.4 GHz, the front camera
at the higher modes, a locked screen, runs longer than two minutes, battery, the iPhone's temperature, a larger
window, a native socket, a second Android phone or a faster one, an Android hardware encoder, and the PC side
beyond receiving the frames (there is no virtual camera yet).

## What this means for the product

- With JPEG as it is built now, 480p to 720p60 and 1080p30 run at the requested rate on a cool phone, 1080p60
  and 2K reach about half to two thirds of it, and 4K about a sixth. The gate for what is offered is
  `CODEC_TRIED` in `mobile/src/core/videoModes.ts`; it should follow these numbers.
- Letting more than one frame be in flight whatever its size (a policy of the sender, protocol section 8.4) was
  tried on the iPhone and measured (previous section): 1080p30 went from 16.8 to 30.1 fps and 1080p60 from 20.2
  to 38.7, at the price of some latency, which was not measured. `VIDEO_WINDOW` in `main` is now 3 frames and
  2 MB; nothing above 720p30 is offered yet, so the app behaves the same until the gate is raised. The next change would be handing the frame to
  the network from native code instead of through React Native's `WebSocket`, which would take the base64 work
  off the JavaScript thread; it was not tried. Neither helps the frame size itself: at 4K a JPEG is 630 to
  950 KB.
- The modes above 1080p, and any sustained run on a phone that heats up, are expected to need a hardware
  encoder (H.264 or HEVC) and a decoder on the PC (protocol section 8.7).
- H.264 in hardware, measured on the iPhone (previous section), takes the modes up to 2K60 to their requested rate at
  5 to 15 times fewer bits than JPEG and 4K to 25 fps. It is the way to offer the tiers that were planned. What it
  needs next is not measured: a decoder on the PC (which the virtual camera needs in any case), the same on Android
  (MediaCodec), the latency, and a run of an hour on a phone that warms up.
- On a phone of the OPPO's class, JPEG on Android can only be offered at 480p today, and the sizes it offers should
  be the sizes the module delivers. Whether a faster Android phone does much better was not measured.
