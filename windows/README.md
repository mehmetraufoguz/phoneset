# phoneset Camera (Windows)

The first step of the virtual camera of phoneset for Windows 10 (the plan is `docs/virtual-camera.md`, phase 3): a
DirectShow virtual camera "phoneset Camera" that shows a moving test picture. It does **not** carry the phone's
picture yet: nothing here reads `/video.frames`.

It is [softcam](https://github.com/tshino/softcam) (MIT, Copyright 2020 tshino), a small C++ DirectShow source
filter with a sender API (a program writes frames into shared memory, the filter hands them to the apps). The scripts
fetch softcam at the commit pinned in `build.ps1` and build it with a name, a CLSID and shared memory names of our own
(so it never collides with another softcam on the PC). softcam's source is not copied into this repository; if a
built DLL is ever shipped, softcam's licence text goes with it.

```powershell
windows\build.ps1       # fetches softcam, builds windows\dist\softcam.dll and windows\dist\sender.exe
windows\install.ps1     # administrator (UAC): copies the DLL to C:\Program Files\phoneset\camera and registers it
windows\dist\sender.exe # sends a moving test picture (320 x 240), the camera shows it while this runs
windows\uninstall.ps1   # administrator (UAC): unregisters the camera and removes its folder
```

Needs Git and Visual Studio 2022 (or the Build Tools) with the C++ desktop workload and a Windows SDK; no other
installation. `install.ps1` and `uninstall.ps1` ask for administrator rights themselves, registering writes to HKLM
(`CLSID\{43750394-889D-43A4-807F-5FB98F8FCA5D}` and the video input device category). Nothing else is changed.
`uninstall.ps1` leaves nothing behind except `windows\dist`, which git ignores.

## What was seen (Windows 10 22H2, build 19045, 2026-11-15)

| Where                                                 | Result                                                                                                                  |
| ----------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| ffmpeg (`-f dshow -i video="phoneset Camera"`)        | the camera is listed and delivers frames (the balls move)                                                               |
| OBS Studio 33.0.1, "Video Capture Device"             | shows the picture                                                                                                       |
| Chrome 157 and Edge 157 (a web camera test page)      | listed and shows the picture, **only when the DLL is in a folder every process may read** (see below)                   |
| Windows Camera app, Zoom, Teams                       | not tried: not installed on this PC                                                                                     |

**The folder matters.** With the DLL in a user folder (under `AppData`), `chrome://settings/content/camera` listed
"phoneset Camera" but the page's camera picker did not offer it ("Use available cameras (3)" with two names in the
list). Chrome and Edge read cameras in a sandboxed process that cannot load a DLL from there. From
`C:\Program Files\phoneset\camera`, which grants read to `ALL APPLICATION PACKAGES` and `ALL RESTRICTED APPLICATION
PACKAGES` by inheritance, both browsers list the camera and show the picture. A browser that was open when the camera
was registered has to be closed and opened again.

**Gotchas met on the way.** `regsvr32.exe` is a Windows program, not a console one: PowerShell does not wait for it and
leaves `$LASTEXITCODE` empty, so the scripts start it with `Start-Process -Wait -PassThru`. `softcam.vcxproj` built on
its own copies its header and library to `$(SolutionDir)`, so `build.ps1` sets `SolutionDir`; the solution's tests want
NuGet packages, so only the two projects are built.

## What is not done

- The helper that reads `/video.frames`, decodes (Media Foundation for H.264, WIC for JPEG), turns the picture upright,
  fits it into the camera's size and writes it with softcam's sender API.
- With no sender running the camera cannot be opened at all: it has no output pin (ffmpeg: "Could not find output pin
  from video capture device"), so an app lists it but fails to start it. The slate "Waiting for your phone" of the Mac
  camera is not there; a helper has to keep a sender alive (and send the slate) for as long as the camera is installed
  and an app may open it, or the filter has to be changed to make its pin without a sender.
- A fixed 320 x 240 size from the example sender; the sender chooses the size, so 1280 x 720 comes with the helper.
- Windows 11 (build 22000 and newer): `MFCreateVirtualCamera` would be the way there; a DirectShow camera may not show
  in the Windows Camera app.
- `phoneset camera install | uninstall | status` on Windows (`pc/internal/camera` still has the stub).
- 32-bit programs: only the x64 DLL is built, so a 32-bit camera program does not see the camera.
