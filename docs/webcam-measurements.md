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
- JPEG quality 70 (`VIDEO_QUALITY`). The phone holds the picture upright, so a 1920 x 1080 mode arrives
  as 1080 x 1920.

## What had to change first

The iOS camera module ignored the size it was asked for: it chose the 1280 x 720 or the 640 x 480 preset
from the width alone, so nothing above 720p had ever been captured, whatever the mode said. It now picks the
camera format that has exactly the requested size and can run at the requested rate, and sets the frame
duration (`PhonesetCameraCore.format(of:width:height:fps:)`). With that, frames arrive at the sizes asked
for. The Android module already asks CameraX for a size but sets no frame rate; it was not measured here.

## Results

Frame rate is the average over 25 s (the worst single second in brackets). Every run had 0 missing
sequence numbers.

| Mode             | fps (worst second) | Mbit/s | KB per frame | Notes                                   |
| ---------------- | ------------------ | ------ | ------------ | --------------------------------------- |
| 480p30           | 30.1 (30)          | 8.5    | 34           |                                         |
| 480p60           | 55.7 (22)          | 14.1   | 31           | 2 gaps over 100 ms                      |
| 720p30           | 30.0 (29)          | 18.1   | 74           |                                         |
| 720p60           | 59.4 (45)          | 35.3   | 73           | one gap of 259 ms                       |
| 1080p30          | 28.8 (23)          | 37.6   | 159          |                                         |
| 1080p60          | 20.7 (20)          | 27.4   | 161          | steady at about a third of the request  |
| 2K30 (1920x1440) | 15.1 (14)          | 28.1   | 228          |                                         |
| 2K60             | 20.0 (19)          | 38.6   | 235          |                                         |
| 4K30             | 7.7 (6)            | 39.3   | 626          | 188 gaps over 100 ms                    |
| 4K60             | 4.7 (4)            | 30.6   | 788          | 118 gaps over 100 ms                    |

A longer run at 720p30 (120 s) held 30.0 fps, 16.8 Mbit/s, with a longest gap of 70 ms.

## Reading them

Shown by the numbers:

- With JPEG, 480p to 720p60 and 1080p30 run at or close to the requested rate on this link.
- 1080p60, 2K and 4K do not reach the requested rate, and the gap grows with the size of the frame.
- Four modes (720p60, 1080p30, 2K60, 4K30) land at 35 to 39 Mbit/s. Several different modes ending at about the
  same bit rate looks like a limit of the whole path, not of one mode.

Not shown, only guessed:

- Where that limit is. Three modes (1080p60, 2K30, 4K60) are below it and sit at 15 to 21 fps, and the rates
  look like whole camera frame periods (every second frame at 30 fps, every third at 60), which would fit a
  per-frame cycle that runs one frame at a time: make the JPEG, hand it to JavaScript, encrypt, send, wait for
  the PC's acknowledgement. The phone's own counters were not read, so the encoder, that cycle and the Wi-Fi are
  not told apart yet.
- Anything about the link in a worse spot, at a distance, on 2.4 GHz or with other traffic.

## Not tested

Wi-Fi capacity as such (the link test of protocol section 8.8 is not built), distance, 2.4 GHz, the front camera
at the higher modes, a locked screen, runs longer than two minutes, battery and heat, the Android phone, and
the PC side beyond receiving the frames (there is no virtual camera yet).

## What this means for the product

The modes planned (480p to 4K, 30 and 60) are what the camera can report. With JPEG only the lower ones are
worth offering today; the gate for that is `CODEC_TRIED` in `mobile/src/core/videoModes.ts`, which should
follow these numbers once the cause above is known. The higher modes are expected to need a hardware encoder
(H.264 or HEVC) and a decoder on the PC (protocol section 8.7).
