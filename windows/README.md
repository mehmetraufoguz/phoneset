# phoneset Camera (Windows)

The virtual camera of phoneset for Windows 10 (the plan is `docs/virtual-camera.md`, phase 3): a DirectShow camera
"phoneset Camera". This folder has the camera itself, with a slate of its own for when nothing is sending. It does
**not** carry the phone's picture yet: nothing here reads `/video.frames` (that is the helper, phase 3b).

It is [softcam](https://github.com/tshino/softcam) (MIT, Copyright 2020 tshino), a small C++ DirectShow source
filter with a sender API (a program writes frames into shared memory, the filter hands them to the apps). The scripts
fetch softcam at the commit pinned in `build.ps1`, change it and build it. softcam's source is not copied into this
repository; what we change is `filter/softcam-slate.patch` (a patch to that commit), `filter/Slate.cpp` and
`filter/Slate.h` (our code: the slate), and a few string replacements in `build.ps1` (the name "phoneset Camera", a
CLSID and shared memory names of our own, so it never collides with another softcam on the PC). softcam's licence text is
in `softcam-LICENSE.txt`; it has to go with any built DLL that is ever shipped.

```powershell
windows\build.ps1       # fetches softcam, builds windows\dist\softcam.dll and windows\dist\sender.exe
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
is ignored, the slate stays. Both are for the helper.

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
| the sender is deleted (3 times)                                    | the slate is back within about 55 ms                                                      |
| `scCreateCamera` right after `scDeleteCamera` (8 times)            | ok at once or after 4 to 36 ms: it fails while the camera still holds the old frame buffer    |
| a sender of 640 x 480                                              | ignored, the slate stays (30 a second)                                                        |
| `sender.exe` (another process) appears, then is killed             | live after about 80 to 130 ms; the slate is back after about 550 ms (the heartbeat timeout)         |
| stop and release                                                   | 10 to 27 ms and 2 ms, no hang in a run of about 20 s with all the switching above                                    |

**Not checked with the slate:** the registered camera in Chrome, Edge, OBS (the evening tests in `TESTING.md`); another
process holding the camera while a sender comes and goes; the Windows Camera app, Zoom and Teams; `install.ps1` and
`uninstall.ps1` with this build. The test program talks to the camera in its own process, which is not what an app does
through the registration.

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

**Gotchas met on the way.** `regsvr32.exe` is a Windows program, not a console one: PowerShell does not wait for it and
leaves `$LASTEXITCODE` empty, so the scripts start it with `Start-Process -Wait -PassThru`. `softcam.vcxproj` built on
its own copies its header and library to `$(SolutionDir)`, so `build.ps1` sets `SolutionDir`; the solution's tests want
NuGet packages, so only the two projects are built. softcam's BaseClasses here has no `CBaseRenderer` and no
`CBaseInputPin`, a test program for the filter has to use Windows' own Sample Grabber.

## What is not done

- The helper that reads `/video.frames`, decodes (Media Foundation for H.264, WIC for JPEG), turns the picture upright,
  fits it into 1280 x 720 and writes it with softcam's sender API (phase 3b).
- Windows 11 (build 22000 and newer): `MFCreateVirtualCamera` would be the way there; a DirectShow camera may not show
  in the Windows Camera app.
- `phoneset camera install | uninstall | status` on Windows (`pc/internal/camera` still has the stub).
- 32-bit programs: only the x64 DLL is built, so a 32-bit camera program does not see the camera.
