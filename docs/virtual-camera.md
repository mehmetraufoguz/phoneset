# A virtual camera of our own: plan

Status: phases 0, 1 and 2 are built and work on the developer's Mac (section 4); Windows and Linux are plans. Written 2026-11-15, after the camera path from the phone to the PC was
measured (docs/webcam-measurements.md) and shown to work through OBS Studio's Browser source and Virtual Camera on
Windows 10. The point of a virtual camera of our own is that Zoom, Meet, Teams, FaceTime and the like list "phoneset
Camera" without OBS in between.

What is already there, and stays as it is: the PC agent keeps the phone's frames and serves them on the loopback
interface, among others as `/video.frames` (docs/protocol.md, section 9): records with codec, size, key frame flag,
rotation and mirror, for JPEG and H.264, a reader that joins gets a key frame within about 0.1 s. The agent is a plain
Go program (no cgo) and should stay one.

## 1. Decisions

| Question                     | Choice                                                                                                         | Why                                                                                                                                                 |
| ---------------------------- | -------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| Which system first           | macOS                                                                                                          | Can be built and tried end to end on the developer's Mac; a Windows 10 PC is only reachable through another session.                                |
| Where decoding happens       | in a native piece of each system (VideoToolbox on macOS, Media Foundation on Windows), reading `/video.frames` | The Go agent stays free of cgo and of a decoder; the system decoders use the hardware.                                                              |
| How it reaches the apps      | a Core Media I/O camera extension on macOS (a system extension, macOS 12.3 and newer) with a companion app     | The DAL plug-ins it replaced are not recommended any more.                                                                                          |
| Output the apps see          | one device "phoneset Camera", 1920 x 1080 at 30 frames a second (1280 x 720 as a second format), landscape     | A virtual camera has fixed formats. A portrait picture from the phone is shown with bars at the sides ("fit"); filling the frame is a later option. |
| When the phone sends nothing | the device shows a slate: "Waiting for your phone"                                                             | An app that has opened the camera must not freeze or fail.                                                                                          |

## 2. How it fits together on macOS

Two bundles, both signed by us, shipped as one app, **phoneset Camera.app**, which has to be in `/Applications` for the
extension to be activated (the public sample this plan follows says so; the activation is requested by the app):

- **The companion app** (a menu bar app, no Dock icon). It activates the extension (`OSSystemExtensionRequest`),
  reads the agent's `control.json` (port and token) from the same configuration directory the agent uses, opens
  `/video.frames`, decodes each record (H.264 with a VideoToolbox decompression session, JPEG with ImageIO), turns it
  upright by the record's rotation and mirror, fits it into the output frame, and pushes it as a `CMSampleBuffer`
  into the extension's **sink stream** through the Core Media I/O C API. It shows whether the agent runs, whether a
  phone sends, and whether the camera extension is on.
- **The camera extension** (`.systemextension` inside the app's `Contents/Library/SystemExtensions`). It publishes one
  device with two streams: a **source** stream, which is what other programs read, and a **sink** stream, where the
  app writes; the extension forwards sink frames to the source. With nothing written for about a second it produces
  the slate itself.

Frames travel app to extension through the sink stream, not through XPC or shared memory: Apple's engineer advises
avoiding XPC for this (forum thread 706184), and the public sample that does it this way works on 12.3. The app must
not be sandboxed (same thread); the extension is.

The agent does not change. Nothing but the companion app reads `/video.frames` for this, and the token stays in the
user's own configuration directory.

## 2a. One place to control it, on every system

The phoneset program is where the camera is controlled from, whatever the system: `phoneset camera status | install |
uninstall`, the camera line of `phoneset status`, and a camera section in the tray menu (status line, install and
remove) all go through the package `pc/internal/camera` and the control API (`GET /api/camera`, `POST
/api/camera/install`, `/uninstall`, and `POST /api/camera/report`, where the part that carries the picture says what it
is doing). A system provides a `Backend` (probe, install, uninstall, start) and nothing else is system specific.

| System  | Backend                                                                                                                                                                                                                                                                                                   | State                                                                                                                                                                                                                                  |
| ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| macOS   | `phoneset Camera.app` in `/Applications` (a camera extension and a helper without a window); the program starts it when a phone begins to send video                                                                                                                                                      | works; `phoneset camera install` puts it in place and asks macOS to turn it on, then the user allows it once in System Settings                                                                                                        |
| Windows | none of its own yet; `phoneset camera` says to use OBS Studio's Virtual Camera                                                                                                                                                                                                                            | OBS bridge works. Own camera: a DirectShow filter on Windows 10 (a test picture works, `windows/`; not the phone's picture yet), `MFCreateVirtualCamera` on Windows 11 (build 22000 and newer), each a native component that reads `/video.frames`, behind the same commands and the same tray section |
| Linux   | none yet; the plan is v4l2loopback (a kernel module that makes `/dev/videoN`) fed by the program through `ffmpeg` (reading `/video.h264` or `/video.mjpg`, turning it upright with the record's rotation, writing to the device); `install` would check for the module and ffmpeg and say what is missing | not built and not tried: no Linux machine to test on                                                                                                                                                                                   |

The macOS app is only a helper: its window is gone (it has none), it is started with `--background`, `--install` or
`--uninstall`, and opened by hand it says where the camera is controlled from. After an install it starts itself again,
because an app that was open while macOS replaced the extension keeps an out-of-date view of the cameras.

## 3. What has to be true to build and ship it

| Need                                                                                                                                                                                          | State                                                                                                                                                                                                                                                  |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Xcode 26 and an Apple Development certificate for team `JY83GWM55C`                                                                                                                           | present on the developer's Mac (the iOS app is built the same way)                                                                                                                                                                                     |
| A tool to generate the Xcode project from a text file (XcodeGen), so that the project is reproducible and reviewable                                                                          | not installed; `brew install xcodegen`                                                                                                                                                                                                                 |
| The app's entitlement `com.apple.developer.system-extension.install`, a restricted entitlement: with Apple Development signing Xcode's automatic signing provides it                          | to be checked in the first build                                                                                                                                                                                                                       |
| An App Group shared by the app and the extension (a forum poster reports that the extension's Mach service name must start with one of its groups, which Apple's engineer did not contradict) | to be set up in the project                                                                                                                                                                                                                            |
| Developer ID signing and notarization for use outside the Mac App Store (system extensions have to be notarized)                                                                              | done 2026-11-15: a Developer ID Application certificate is in the owner's keychain and the System Extension capability is on for `com.rapp.phoneset.camera`; the signed build needs nothing more (the extension has no profile of its own); notarization by a stored profile, by hand (`macos/release.sh`, see macos/README.md), not in CI |
| First install: the user approves the extension in System Settings, and may need an administrator's password                                                                                   | where exactly and how it reads on macOS 26 is **not verified**; found in the first run                                                                                                                                                                 |

Sources read for this plan: Apple's Core Media I/O overview (developer.apple.com/documentation/coremediaio), the
forum threads 706184 and 751413 (frame transport, client counting), 793731 (the missing system extension entitlement,
which OBS Studio met as well), 840312 (an activation failure reported on macOS 26.5 with a bundle ID that did not
match, answered by Apple as a naming problem, not an operating system regression), the sample
github.com/ldenoue/cameraextension (a source and a sink stream), and Apple's notarization page. Apple's own camera
extension article and the system extension installation page did not load for the reader that fetched them, so
what they say about signing and approval is not in this plan.

## 4. Phases

Each phase ends with something that can be seen, and the next one starts only when that is seen.

0. **Does it activate here?** _Done 2026-11-15:_ `macos/` builds with `macos/build.sh`; on macOS 26.7.1, signed with the
   Apple Development team (Xcode's automatic signing made the provisioning profile, including the system extension
   entitlement), the extension went to `[activated enabled]` in the category `com.apple.system_extension.cmio` from the
   app in `/Applications`, "phoneset Camera" is in the list of cameras (`system_profiler SPCameraDataType`) and the
   user saw the test picture in Photo Booth. The first try failed with `OSSystemExtensionErrorDomain 9` ("does not
   appear to belong to any extension categories"): the extension's Info.plist needs `CFBundlePackageType` (`SYSX`) and
   `NSSystemExtensionUsageDescription` besides the `CMIOExtension` key. macOS names the place to manage it: System
   Settings, General, Login Items & Extensions, Camera Extensions. No separate approval step was seen on this Mac.
   The plan for the phase, as written before it was done: An Xcode project (from `macos/project.yml`) with the companion app and the extension.
   The extension publishes "phoneset Camera" with a moving test pattern; the app has one button that activates it.
   Done when Photo Booth (or QuickTime Player's camera list) on the developer's Mac shows "phoneset Camera" and the
   pattern, after the user has approved the extension in System Settings. This is the gate: if the extension does not
   activate on macOS 26.7 with the Apple Development team, the rest waits for that to be understood.
1. **The phone's picture.** _Done 2026-11-15, checked by the user in Photo Booth:_ the iPhone's picture (H.264) arrives
   upright, smooth, and turns to landscape and fills the frame when the phone is turned. The companion app reads
   `/video.frames`, decodes (VideoToolbox for H.264, ImageIO for JPEG), turns the picture upright, fits it into
   1280 x 720 with black bars and writes it to the extension's sink stream; with nothing written the extension shows
   its own slate. What was learned: the sink stream is the one with stream direction 0 (the source has 1; the order
   of the two in the device's list also matched); the extension's version number (`CFBundleVersion`) has to grow for
   macOS to replace an installed extension, so `build.sh` uses the time; a sink opened while the extension is being
   replaced stays attached to the old one and its queue stays full, so the app now closes and opens the sink again when
   the queue is full for about two seconds or the camera's stream numbers change; removing the camera and installing
   it again (or switching it off in System Settings, General, Login Items & Extensions, Camera Extensions) makes
   programs such as Photo Booth lose the camera and needs the switch to be turned on again there. Not checked: a JPEG
   camera through the virtual camera, an Android phone, the load on the Mac and the delay from camera to picture, other
   programs (Zoom, FaceTime, Chrome), the camera in portrait where the frame is not filled (it is shown with bars).
   The plan for the phase, as written before it was done: The app reads `/video.frames`, decodes, turns upright, fits and writes to the sink stream;
   the slate when nothing comes. Done when the OPPO's and the iPhone's picture, upright, in both orientations and in
   H.264 and JPEG, shows in Photo Booth, and the slate shows when the phone turns the camera off.
2. **A product.** _Done 2026-11-15:_ the control is in the PC program, not in a menu bar item of the app (the app has
   no window): `phoneset camera install | uninstall | status`, the camera line of `phoneset status`, the tray section,
   and the program starts the helper when a phone begins to send video. `install` takes the app from next to the
   program, else downloads `phoneset-camera-macos.zip` from the latest release (checked against `SHA256SUMS`; a release
   that lists `SHA256SUMS` but can't be checked is refused), `--force` and `phoneset update` fetch the latest again.
   The zip is made by `macos/release.sh` (archive, Developer ID export, notarize, staple, zip; `--upload TAG` adds it to
   an existing release and to its `SHA256SUMS`), by hand on the owner's Mac. Checked: the notarized build installed from
   a download (a local fake release), macOS took its extension without a new approval, and the owner saw the iPhone's
   picture through it. Not checked: the download from a real GitHub release.
3. **Windows.** Windows 11 (build 22000 and newer) has `MFCreateVirtualCamera`; Windows 10 needs a DirectShow filter
   registered by an administrator; both need a native (C++) component that reads `/video.frames`. Until it is done
   OBS's Virtual Camera is the way on Windows (it works, see the measurements report).
   - **3a. Does a DirectShow camera of ours work on Windows 10? _Done 2026-11-15_ (build 19045, see `windows/README.md`):**
     yes. softcam (MIT, a DirectShow source filter with a sender API), built with our own name, CLSID and shared memory
     names by `windows/build.ps1`, registered by `windows/install.ps1`, showed a moving test picture in ffmpeg, OBS,
     Chrome and Edge (the owner saw it). Learned: with the DLL in a user folder and the browsers already open, the web page's camera picker in Chrome and Edge did not
     offer the camera (the browser's settings page listed it); from `C:\Program Files\phoneset\camera` and after the browsers were
     restarted it did. Which of the two fixed it was **not tested** (the first explanation, a sandbox that cannot load a DLL from a user
     folder, is probably wrong: Chrome's camera process ran with `--service-sandbox-type=none` on that PC); Program Files is the right
     place for a product anyway; Chrome's video capture service keeps the DLL loaded after it used the camera, so removing or replacing
     the file fails ("access denied") until that helper process is closed, and an uninstall or update has to say so; registering needs an administrator (UAC), so `phoneset camera install` on Windows has to ask for it;
     with no sender running the camera cannot be opened at all (no output pin), so the "Waiting for your phone" slate
     needs either a helper that keeps a sender alive or a change to the filter so that it makes its pin and the slate
     itself (the second matches macOS, where the extension has its own slate, and works while the program is not
     running); only an x64 DLL, so 32-bit camera programs do not see it. Not tried: the Windows Camera app, Zoom, Teams.
   - **3b. The phone's picture.** A helper reads `/video.frames`, decodes (Media Foundation for H.264, WIC for JPEG), turns
     it upright, fits it into 1280 x 720 and writes it through the sender API. _Not started as a product;_ a throwaway bridge
     outside the repository (agent, ffmpeg for turning and fitting, a small C++ sender that repeats the last frame at 30 frames a
     second) showed the OPPO's JPEG picture (back and front camera) in Chrome (the owner saw it), and an H.264 frame (front
     camera, rotation 2) upright through the camera by ffmpeg. Learned: `/video.mjpg` gives nothing for an H.264 camera, so the
     helper has to read `/video.frames` (rotation per record, no restart when it changes); ffmpeg's `-fflags nobuffer -flags
     low_delay` stall a raw H.264 input after the first frame (a fault of the bridge, not of the agent or the phone); when the
     phone or the network goes the last frame stays frozen, so the helper must send the slate itself after a while without frames.
   - **3c. A product.** A Windows `Backend` in `pc/internal/camera` (download the signed zip from the release, ask for
     administrator rights, register, remove), the tray section, `phoneset update`. Windows 11 with `MFCreateVirtualCamera`
     is looked at separately. _Not started._

## 5. Not decided, and not part of the first version

- Opening the phone's camera when an app on the PC opens the virtual camera. The protocol has no request from the PC
  to the phone to start the camera; the extension can tell the app when a client connects (forum thread 751413).
  This is the natural next step and needs a protocol change.
- A "fill" option instead of "fit" for a portrait picture, a mirror option for the front camera, 60 frames a second.
- Sound: a microphone already exists (the virtual microphone, BlackHole / VB-CABLE); video does not change that.
