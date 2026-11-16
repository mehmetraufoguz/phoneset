# phoneset Camera (Windows)

The virtual camera of phoneset for Windows 10 (the plan is `docs/virtual-camera.md`, phase 3): a DirectShow camera
"phoneset Camera" with a slate of its own, and `phoneset-camera.exe`, the helper that carries the phone's picture to it.
**Built and tested against a stand-in for the program, not run with a phone yet**; the evening tests are in `TESTING.md`.
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
| the helper's cost                                                      | idle: 0.00 % CPU (below what could be measured), 8 to 11 MB; 1080p H.264: about 130 ms of work a second (13 % of one core), the decoder 3 to 5 ms and the turn and fit 1 to 2 ms a picture on this 16-thread PC |

Also against **the real program** built from `main` (no phone): the helper found its `control.json`, opened
`/video.frames` and reported; `GET /api/camera` showed `running: true` and the helper's line. The program's camera
line in `phoneset status` does not show it yet, because the program's Windows `Backend` is still the stub (phase 3c).

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

- The program's side: a Windows `Backend` in `pc/internal/camera` that installs and removes the camera, starts the helper
  when a phone begins to send video (and quits it with `--quit`), and shows the helper's line in `phoneset status` and
  the tray (phase 3c).
- A release asset (`phoneset-camera-windows-amd64.zip`) with `softcam.dll`, `phoneset-camera.exe`, the licence text, and
  its checksum.
- Windows 11 (build 22000 and newer): `MFCreateVirtualCamera` would be the way there; a DirectShow camera may not show
  in the Windows Camera app.
- A GPU decoder (a software decoder in its low latency mode is used), and a 4K picture was not tried.
- 32-bit programs: only the x64 DLL is built, so a 32-bit camera program does not see the camera.
