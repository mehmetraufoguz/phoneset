# The webcam: "phoneset Camera"

The phone's camera as a camera on the PC. Source of truth: `docs/virtual-camera.md` (plan and what was run) and
`docs/webcam-measurements.md` (numbers). This file is what an agent needs to help someone with it.

```
phone camera ──Wi-Fi──▶ phoneset (PC) ──▶ /video.frames on 127.0.0.1 ──▶ camera helper ──▶ "phoneset Camera" ──▶ any video app
```

The PC program keeps the phone's frames (JPEG or H.264, each with its size, rotation and mirror) and serves them on
the loopback interface only. A small native helper per system reads them, decodes and turns the picture upright, and
feeds the camera. With no picture the camera shows a **slate** ("Waiting for your phone") so an app that opened it
never freezes.

## Put it on the PC

```sh
phoneset camera status           # not installed / waiting for approval / switched off / ready / being removed / unsupported
phoneset camera install          # --force fetches the newest camera again
phoneset camera uninstall        # also: remove
```

`install` uses the camera files next to the program if there are any (a build from the repository), else it
downloads `phoneset-camera-macos.zip` or `phoneset-camera-windows.zip` from the **latest release** and checks it
against the release's `SHA256SUMS`; a release without those files (before v0.3.0) cannot install it. The tray
menu has **Install the camera…** and **Remove the camera** (shown depending on the state). `phoneset update` is meant
to refresh the camera too (`camera install --force`); on Windows that path was not run.

### macOS (12.3 or newer)

- Installs **phoneset Camera.app** into `/Applications` (a camera extension and a helper without a window, signed with
  a Developer ID and notarized) and asks macOS to turn the extension on.
- If `camera status` says *waiting for approval* or *switched off*: System Settings → General → Login Items &
  Extensions → **Camera Extensions** → switch on phoneset Camera. On the developer's Mac no approval step appeared; how it
  reads on a fresh Mac and on macOS 26 is **not verified**.
- The program starts the helper by itself when a phone begins to send video.

### Windows (10; Windows 11 not looked at separately)

- A DirectShow camera (built from softcam, MIT) plus `phoneset-camera.exe`, which carries the picture to it.
  Installed to `C:\Program Files\phoneset\camera` and registered with `regsvr32`, so **one UAC window** (the program
  starts a second copy of itself with administrator rights; answering No changes nothing).
- Chrome, Edge and OBS were seen to list it (the camera is 64-bit only, so 32-bit camera programs do not see it).
  Restart a browser that was open. The program starts the helper (no window) when a phone begins to send video and
  leaves it running when the program quits (it costs nothing idle).
- `uninstall` unregisters the camera. A program that still has the DLL loaded (Chrome's capture service does)
  blocks deleting the file: the command says which program, close it and run it again.
- Nothing is code-signed (a DirectShow filter needs no signature); an antivirus may look at it.

## On the phone

Camera section of the session screen: **Use as a webcam** (switch), **Back / Front**, **Quality** (480p to 4K as the
phone's camera and the PC allow), **Format** (Automatic, H.264, JPEG), **Smoothness** (30 or 60 frames a second),
**Reset to defaults**. The line under the switch says what is really sent (codec, size, frames a second, Mbit/s, and
whether a program on the PC has the camera open). Choices are remembered per PC.

The camera is the app's **subscription** (App Store or Google Play; the store shows the price). The switch is
checked when video starts. **Restore purchases** and **Redeem a code** are in the Camera subscription section.
Audio needs no subscription.

## What was seen, and what was not

| | Seen | Not verified |
| --- | --- | --- |
| macOS | the iPhone's picture, upright, in Photo Booth; install from the real release; the extension taken by macOS | Zoom, FaceTime, Chrome with it; portrait (shown with bars); a fresh Mac's approval wording |
| Windows 10 | the slate and the phone's picture in Chrome, Edge and OBS (an Android phone); install, update and uninstall from the release; a UAC answered No | the Windows Camera app, Zoom, Teams; an iPhone on Windows in the docs' runs; Windows 11; arm64; the tray items |

The program's own message after `camera install` names Zoom, Meet, FaceTime, QuickTime and Teams as places to pick the
camera. Treat that as intent, not as verified; say what was seen if asked.

## Numbers worth knowing

- Output: macOS 1920 x 1080 at 30 (1280 x 720 as a second format); Windows 1280 x 720 at 30 as built. A portrait
  picture is shown with bars at the sides.
- Windows helper cost (software H.264 decoding): with a phone's 1920 x 1080 stream, about 55 % of one core (3.4 % of a
  16-thread PC) after a fix; an earlier run with another phone and a bigger stream took 239 % of one core. If a
  PC is slow with the camera on, lowering **Quality** in the app should help (not measured by itself).
- The front camera looks mirrored in a browser's camera test page (the page mirrors its own self-view), not in OBS or
  the camera's own path.
