# phoneset Camera (Windows)

The virtual camera of phoneset for Windows 10 (the plan is `docs/virtual-camera.md`, phase 3): a DirectShow camera
"phoneset Camera" with a slate of its own, and `phoneset-camera.exe`, the helper that carries the phone's picture to it.
**Built, tested against a stand-in, and run with an Android phone (OPPO A9 2020) on 2026-11-16**: most checks of `TESTING.md` passed (the front camera looked mirrored only in the browser's test page, not in OBS); the helper's CPU at 1080p H.264 is high (239 % of one core); see `docs/virtual-camera.md`, phase 3b.
What exists and what was seen is written below, and where nothing was checked, it says so.

The camera is [softcam](https://github.com/tshino/softcam) (MIT, Copyright 2020 tshino), a small C++ DirectShow source
filter with a sender API (a program writes frames into shared memory, the filter hands them to the apps). The scripts
fetch softcam at the commit pinned in `build.ps1`, change it and build it. softcam's source is not copied into this
repository; what we change is `filter/softcam-slate.patch` (a patch to that commit), `filter/Slate.cpp` and
`filter/Slate.h` (our code: the slate), and a few string replacements in `build.ps1` (the name "phoneset Camera", a
CLSID and shared memory names of our own, so it never collides with another softcam on the PC). softcam's licence text is
in `softcam-LICENSE.txt`; it has to go with any built DLL that is ever shipped.

```powershell
windows\build.ps1       # fetches softcam, builds windows\dist\softcam.dll, phoneset-camera.exe and sender.exe
windows\install.ps1     # administrator (UAC): copies the DLL to C:\Program Files\phoneset\camera and registers it
windows\dist\sender.exe # sends a moving test picture (1280 x 720); the camera shows it while this runs
windows\uninstall.ps1   # administrator (UAC): unregisters the camera and removes its folder
```

Needs Git and Visual Studio 2022 (or the Build Tools) with the C++ desktop workload and a Windows SDK; no other
installation. `install.ps1` and `uninstall.ps1` ask for administrator rights themselves, registering writes to HKLM
(`CLSID\{43750394-889D-43A4-807F-5FB98F8FCA5D}` and the video input device category). Nothing else is changed.
`uninstall.ps1` leaves nothing behind except `windows\dist`, which git ignores.

## The slate

With no sender the camera always offers its output pin: one type, 1280 x 720 at 30 frames a second, 24-bit RGB (what
softcam uses). It shows the same slate as the macOS camera (`macos/Extension/DeviceSource.swift`): a dark background, a
blue bar on the bottom edge that moves 12 pixels to the right with each picture (so that a frozen picture is easy to
tell from a live one), "phoneset Camera" and "Waiting for your phone…" in white bold type (Segoe UI Bold, else Arial
Bold), drawn once with GDI; only the bar is drawn per picture. The pictures are paced at the frame rate. A sender of
1280 x 720 that appears while an app has the camera open takes over; when it goes away (`scDeleteCamera`, a killed
process) the slate comes back. A sender that is there first sets the camera's size, as before.

What it does **not** do: a sender that is alive but sends nothing keeps the last picture on screen for good (softcam only
notices a sender that is gone: its heartbeat stops for half a second), and a sender of another size than the camera's
is ignored, the slate stays. The helper takes care of the first by letting its sender go.

## The helper: phoneset-camera.exe

The Windows twin of the macOS companion app (`macos/App`). It runs in the background (no window, no console) and
carries the phone's picture from the phoneset program to the camera:

1. It finds the program through `control.json` in the program's config folder (port and token; `%AppData%\phoneset`, or
   the folder given with `--config-dir`, like the program's own flag), opens `/video.frames` with WinHTTP and reads the
   records (`docs/protocol.md`, section 9). It tries again every second when the program is not there, ends a
   connection that gives a record it does not understand (and makes a new one), skips a codec it does not decode.
2. It decodes: H.264 with Media Foundation's decoder (a software decoder in its low latency mode: one picture out for
   each one in, taking 3 to 5 ms for 1080p; no GPU path), JPEG with WIC. A key frame is needed first; when a record is
   dropped or does not decode, it waits for the next key frame again (the macOS helper's `lostFrame`).
3. It turns the picture upright by the record's rotation (quarter turns clockwise) and mirror, fits it into 1280 x 720
   with black bars (no cropping), and writes it with softcam's sender API, 30 frames a second at most.
4. It tells the program once a second what it is doing (`POST /api/camera/report`, the same line the macOS app sends)
   and writes what it does to `camera-helper.log` in the config folder: counters of what was received, dropped,
   decoded and written, how long the work took, the last decoder error. The token is never logged.
5. When no picture came for 1.5 s (the phone turned its camera off, the network or the program is gone) it **lets the
   sender go** (`scDeleteCamera`), so that the camera shows its slate; it makes the sender again when pictures
   return. This works while an app has the camera open (see below); the alternative, sending a slate picture itself, was
   not needed.

```powershell
windows\dist\phoneset-camera.exe --config-dir C:\phoneset-test   # starts it (silently; no output anywhere)
windows\dist\phoneset-camera.exe --quit                          # asks the running one to quit; waits up to 5 s
```

One instance per user session (a named mutex; a second one exits with code 3 at once). `--quit` signals the event
`Local\phoneset-camera-helper-quit` and returns 0 when the helper is gone, 1 when none was running, 2 when it was still
there after 5 s: that is what the program's Windows `Backend` will use. It never quits by itself: not when the program
restarts, not when the phone goes away. It needs `softcam.dll` next to it (`build.ps1` puts both in `windows\dist`; the
installer will put both in `C:\Program Files\phoneset\camera`).

How the helper's threads divide the work, because it matters: the link reads, the decoder thread decodes and turns, a
thread of its own writes with `scSendFrame` (which sleeps to keep to 30 a second, so it must not be the decoder's), a
thread of its own reports (a program that is slow to answer must not hold anything up), and once a second the main thread
lets the sender go when it is time and writes the log line.

## What was seen (Windows 10 22H2, build 19045, 2026-11-15 and 16)

Before the slate, with softcam's own behaviour:

| Where                                                 | Result                                                                                                                  |
| ----------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| ffmpeg (`-f dshow -i video="phoneset Camera"`)        | the camera is listed and delivers frames (the balls move)                                                               |
| OBS Studio 33.0.1, "Video Capture Device"             | shows the picture                                                                                                       |
| Chrome 157 and Edge 157 (a web camera test page)      | listed and shows the picture once the DLL was in `C:\Program Files\phoneset\camera` and the browsers were restarted    |
| Windows Camera app, Zoom, Teams                       | not tried: not installed on this PC                                                                                     |
| no sender                                             | the camera could not be opened (no output pin: ffmpeg "Could not find output pin from video capture device")            |

The slate, checked with a throwaway test program (not in the repository) that loads `softcam.dll` directly, without
registering it, builds a graph "camera, Sample Grabber, Null Renderer" in one process and watches what comes out
(a sender in the same process and `sender.exe` in another):

| Check                                                              | Result                                                                                        |
| ------------------------------------------------------------------ | --------------------------------------------------------------------------------------------- |
| no sender: the camera connects                                     | yes, 1280 x 720, 24 bit, 30 frames a second                                                   |
| the pictures delivered                                             | 30 a second (60 in 2 s, twice), the slate: the text, the moving bar and the colours as above  |
| cost while idle on the slate                                       | 0.5 % of one core (that process, 3 s)                                                         |
| a 1280 x 720 sender appears, camera open (3 times)                 | live after about 40 to 55 ms, 30 a second                                                     |
| the sender is deleted (3 times)                                    | the slate is back within about 55 ms                                                          |
| `scCreateCamera` right after `scDeleteCamera` (8 times)            | ok at once or after 4 to 36 ms: it fails while the camera still holds the old frame buffer    |
| a sender of 640 x 480                                              | ignored, the slate stays (30 a second)                                                        |
| `sender.exe` (another process) appears, then is killed             | live after about 80 to 130 ms; the slate is back after about 550 ms (the heartbeat timeout)   |
| stop and release                                                   | 10 to 27 ms and 2 ms, no hang in a run of about 20 s with all the switching above             |

The helper, checked the same way (the camera in that test program, the helper as its own process) against **a stand-in
for the phoneset program**: a throwaway Python server (not in the repository) that writes `control.json` and serves
`/video.frames` as the real program does (chunked, flushed at once, 30 records a second) and takes the reports. It sends
a picture of four colour quarters (upright: red, green, blue, yellow), coded as an Android phone sends it, H.264 as
1920 x 1080 in the camera's own orientation with the rotation and mirror flags that turn it upright, and JPEG upright.
The camera's picture was compared with what it must be (colour at the middle of each quarter, black where the bars are):

| Check                                                                  | Result                                                                                         |
| ---------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| H.264, rotation 0 to 3, with and without mirror (8 cases)              | all upright and the right way round, colours within 1 of what was sent; portrait 405 x 720 centred with bars |
| JPEG landscape, portrait, with the mirror flag                         | the same                                                                                       |
| H.264 sizes 1920 x 1080, 1280 x 720, 640 x 480 (4:3: 960 x 720), full range colours | the same (the decoder is made new when the size changes)                         |
| the frame rate of the camera while it ran                              | 30 a second, steadily (a dip for about 2 s where the sender was let go)                        |
| silence for 5 s, then pictures again                                   | the slate 2.0 s after the last picture (1.5 s, the beat of one second, the filter); the picture is back 65 ms after the first record |
| records the helper must survive: an unknown codec, JPEG and H.264 that are not, a record claiming 2 GB | no crash; each counted; the huge one ended the connection and it came back about 1 s later |
| the connection closed by the program (3 times)                         | reconnected within about 1 s each time                                                         |
| the program restarted with another port and token                      | found the new `control.json`, connected                                                        |
| a second helper; `--quit`; `--quit` with none running                  | exit code 3; helper gone, 0; 1                                                                 |
| the helper's cost                                                      | idle: 0.00 % CPU (below what could be measured), 8 to 11 MB; the stand-in's 1080p H.264 picture: 133 ms of work a second (upright) to 233 ms (across), on this 16-thread PC. A real phone's stream costs more: see the next section |

**The helper's cost on a real stream (2026-11-16, the OPPO's back camera, H.264, phone across).** The first build used
239 to 278 % of one core (15 to 17 % of this 16-thread PC), well above what the stand-in showed. Measured with
`phoneset-camera.exe --profile` (a line a second in the log: wall time of each stage, the stream's bit rate and SPS, and the
CPU time of every thread of the process):

- The stream was **2688 x 1512** (4.06 megapixels, twice 1080p) although the app was set to 1080p: High profile, level
  5.0, 11 to 13.6 Mbit/s, 31 to 33 pictures a second, a key frame about every second. The stand-in's pictures are 1080p
  and static, which is why it looked cheaper. (Fixed on the phone the same evening, see below: the app asked for the widest size at the height, a 20:9 one, and CameraX jumped to the next 16:9 picture.)
- About 14 worker threads of the exe's own PPL pool (`parallel_for`, in the NV12 conversion and in compose) burned 60 to
  170 ms each every second, about 1.5 s of CPU in total, although the work they were given was 73 ms (convert) plus
  53 ms (compose) of wall time: the pool's workers spin while they wait for the next picture. The Media Foundation
  software decoder (`msmpeg2vdec.dll`, about 10 threads) was 0.5 to 0.9 s of CPU a second. The sender thread costs nothing
  (`scSendFrame` sleeps; its 600 to 1000 ms of wall time is waiting).
- With plain loops instead of the pool: 127 % of one core (7.9 % of the PC), but the whole-picture NV12 to BGRA
  conversion (374 ms a second) and the fit (282 ms) took the thread to 28 pictures a second.

What was changed: no pool, and H.264 stays NV12 (the visible part is copied out of the decoder's buffer) until compose
turns **only the pixels of the 1280 x 720 picture** into BGR in one pass with the turn, the mirror and the scaling
(integer tables per picture; Y is interpolated, the chroma is the nearest sample). Result on the same stream: **81 to 85 %
of one core, 5.1 to 5.3 % of the PC**, 63 MB, 30 pictures written a second, none dropped; about 165 ms of compose, 28 ms of
copying and about 450 ms of decoder wall time a second. What did **not** help: `CODECAPI_AVDecNumWorkerThreads` of 1, 2
or 4 (81 to 85 % in each case, the decoder seems to ignore it). What is not done: the Direct3D 11 hardware decoder, which
would take most of the decoder's 0.5 s (this PC has an NVIDIA GPU; not tried). The decoder and the pass cost about in
proportion to the pixels, so at 1080p they should be about half (not measured on a phone). The stand-in's pixel check of
all rotation and mirror cases, JPEG and the size changes passes with the new code (colours within 1); a real phone's
picture after this change was **not looked at** beyond one frame whose colours were right (the owner chose not to test
more).

Also against **the real program** built from `main` (no phone): the helper found its `control.json`, opened
`/video.frames` and reported; `GET /api/camera` showed `running: true` and the helper's line. The program's camera
line in `phoneset status` did not show it then, because the program's Windows `Backend` was still the stub; with the
Backend of phase 3c `phoneset camera status` shows "picture running" while the helper runs.

**Not checked, anything that needs a phone or a person:** a real phone's stream (the stand-in is one coded picture
repeated every second; a real stream is a moving picture, in bursts, with the phone's own SPS/PPS and VUI), so the
colours of a real H.264 stream (the decoder says what the stream says: limited or full range, BT.601 or 709; it is
written to the log) and the real timing are open; that the program reaches the helper in `phoneset status` and the tray;
the registered camera in Chrome, Edge, OBS with the helper (the evening tests in `TESTING.md`); another process holding
the camera while the sender is let go and made again, in a real app; the Windows Camera app, Zoom and Teams; 4K; the
latency from the camera to the picture; `install.ps1` and `uninstall.ps1` with this build.

**What the stand-in does not prove:** it speaks the stream as the program's source says it does, but the real program
is Go and its frames come from a phone. The stand-in showed two faults of the helper that the real program would also
have shown (see "Gotchas": the WinHTTP read, and the drop rule); for the first, at the 100 to 800 KB a second of an H.264
stream from a phone a 64 KB buffer is 0.1 to 0.6 s of video held back.

**Folder or restart: not told apart.** With the DLL in a user folder (under `AppData`) and Chrome and Edge already
open, `chrome://settings/content/camera` listed "phoneset Camera" but the page's camera picker did not offer it ("Use
available cameras (3)" with two names in the list). After the DLL was moved to `C:\Program Files\phoneset\camera`,
registered again, and both browsers were closed and opened again, both listed the camera and showed the picture. Two
things changed at once (the folder and the restart), so which of them made the difference was **not tested**. A
sandbox that cannot read a user folder is only a guess, and a weak one: Chrome's camera process here runs with
`--service-sandbox-type=none`. Installing under Program Files is the right place for a product anyway, and a browser
that was open when the camera was registered has to be closed and opened again to be safe.

**A DLL in use.** Chrome's camera process (`video_capture.mojom.VideoCaptureService`) keeps `softcam.dll` loaded after
a test page used the camera, so removing the file fails ("access denied") even after the registration is gone.
`uninstall.ps1` says which program holds it; closing that program (or the browser) and running it again finishes.

## Gotchas

- `regsvr32.exe` is a Windows program, not a console one: PowerShell does not wait for it and leaves `$LASTEXITCODE`
  empty, so the scripts start it with `Start-Process -Wait -PassThru`.
- `softcam.vcxproj` built on its own copies its header and library to `$(SolutionDir)`, so `build.ps1` sets `SolutionDir`;
  the solution's tests want NuGet packages, so only the projects we need are built. softcam's BaseClasses here has no
  `CBaseRenderer` and no `CBaseInputPin`: a test program for the filter has to use Windows' own Sample Grabber.
- **`WinHttpReadData` waits until its whole buffer is full.** On a stream of some tens of KB a second, a 64 KB buffer
  held the first pictures back for seconds, then gave them all at once. The helper asks `WinHttpQueryDataAvailable` first
  and reads only that. The same would hold for any other WinHTTP client of `/video.frames` (the Go side is not affected).
- **`scSendFrame` sleeps** to keep to the frame rate. On the decoder's thread it held up the decoder: pictures
  piled up, "too many in flight" dropped them, and every drop costs a wait for the next key frame (about a second), so 3
  pictures a second got through. It is on a thread of its own, and a limit of 8 records (skip ahead to the newest key
  frame in the queue; everything is dropped beyond 30) replaced the macOS helper's 3 in flight, because one read of the
  stream can hold a burst of records.
- Media Foundation's H.264 decoder fails (`E_FAIL`) on the second `ProcessOutput` when the output sample's buffer still
  has the length of the first picture: the length is set back to 0 before each call.
- `scCreateCamera` fails while the filter still holds the old frame buffer: it does not take over a name that exists. The
  helper tries again at the next picture, and says "the camera cannot be started: is another program sending to it?" only
  after 3 seconds of it.
- ffmpeg's `-fflags nobuffer -flags low_delay` stall a raw H.264 input after the first frame (found with a throwaway
  bridge, before the helper; not a fault of the program or the phone).
- A source file that is not ASCII is read in the machine's code page by the compiler: the ellipsis of the slate is `\x2026`.

## What is not done

- The program's side is written and was run on this PC (`phoneset camera install | uninstall | status`, the helper started
  by the program; docs/virtual-camera.md, phase 3c, says exactly what was run). Not run: the tray's camera entries,
  `phoneset update`, a real release.
- A release asset: `windows\release.ps1` builds `phoneset-camera-windows.zip` (+ `.sha256`; `-Upload TAG` puts it on a
  release and in its `SHA256SUMS`); no release has it yet, and the upload itself was not run.
- Windows 11 (build 22000 and newer): `MFCreateVirtualCamera` would be the way there; a DirectShow camera may not show
  in the Windows Camera app.
- A GPU decoder (a software decoder in its low latency mode is used), and a 4K picture was not tried.
- 32-bit programs: only the x64 DLL is built, so a 32-bit camera program does not see the camera.
