# A virtual camera of our own: plan

Status: phases 0, 1 and 2 are built and work on the developer's Mac (section 4); on Windows 10 the camera with its own slate and the helper are built, tested against a stand-in for the program, and were run with an Android phone on 2026-11-16 (section 4, phase 3: most checks passed; the helper's CPU at 1080p H.264 is high and the Android app's label does not match the stream's size); Linux is a plan. Written 2026-11-15, after the camera path from the phone to the PC was
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
| Windows | none of its own yet; `phoneset camera` says to use OBS Studio's Virtual Camera                                                                                                                                                                                                                            | OBS bridge works. Own camera: a DirectShow filter on Windows 10 with a slate of its own, and `phoneset-camera.exe` that carries the phone's picture to it (`windows/`; built, tested against a stand-in and run with an Android phone in Chrome, Edge and OBS on 2026-11-16, see phase 3; the program's `Backend` is still the stub), `MFCreateVirtualCamera` on Windows 11 (build 22000 and newer, not looked at); behind the same commands and the same tray section |
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
   picture through it. **Published 2026-11-16 with v0.3.0** (`macos/release.sh --version 0.3.0 --upload v0.3.0`: signed with the Developer ID certificate, notarized (Accepted), stapled, accepted by Gatekeeper as "Notarized Developer ID"; its line is in the release's `SHA256SUMS`): `phoneset camera install --force` on the developer's Mac downloaded it from the real release, checked it (`checksum ok`), put it in `/Applications` and macOS activated the new extension (0.3.0/1794854354, `[activated enabled]`) with no new approval; `phoneset camera status` said ready.
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
     with no sender running softcam's own filter could not be opened at all (no output pin): fixed, see the slate below;
     only an x64 DLL, so 32-bit camera programs do not see it. Not tried: the Windows Camera app, Zoom, Teams.
   - **The slate in the filter.** _Built 2026-11-16_ (`windows/filter`: a patch to softcam, applied by `windows/build.ps1`,
     and `Slate.cpp`), as the macOS extension has its own: the filter always offers its pin (one type, 1280 x 720 at 30
     frames a second, 24-bit RGB) and shows the slate of `DeviceSource.swift` (colours, the bar that moves 12 pixels a
     picture, the two lines of type) when no sender is there, so that an app that opens the camera sees a picture even when
     the program and the helper are not running. A sender of 1280 x 720 that appears while an app has the camera open takes
     over, and the slate returns when it goes. Checked with a throwaway test program that loads the DLL without
     registering it (one process, Sample Grabber; no browser, no elevation): 30 pictures a second, 0.5 % of a core idle,
     live 40 to 55 ms after a sender appears, the slate within about 55 ms of `scDeleteCamera` and about 550 ms after a
     killed sender process (softcam's heartbeat timeout), a sender of another size ignored (the slate stays), no hang
     over repeated switching; the slate's picture was looked at. **Not checked:** the registered camera in Chrome, Edge
     and OBS, another process holding the camera while senders come and go, the Windows Camera app, Zoom, Teams, and
     `install.ps1`/`uninstall.ps1` with this build (`windows/TESTING.md`). What softcam does that matters to the helper:
     `scCreateCamera` fails while the filter still holds the old frame buffer (after a `scDeleteCamera` it took 0 to 36 ms
     to work again: the helper tries at the next picture); a sender that is alive but sends nothing keeps the last picture
     for good, softcam notices only a sender that is gone; a frame wait blocks up to 0.5 s (read in its source, not measured);
     the camera's size is the size
     of the sender that was there when an app opened it, else 1280 x 720.
   - **3b. The phone's picture.** `windows/helper/phoneset-camera.exe` reads `/video.frames`, decodes (Media Foundation for
     H.264, in its low latency mode and in software; WIC for JPEG), turns it upright, fits it into 1280 x 720 and writes it
     through the sender API; after 1.5 s without a picture it lets the sender go, so that the camera shows its slate
     (this works while an app has the camera open); it reports to the program once a second and logs to `camera-helper.log`.
     _Built 2026-11-16; tested against a stand-in for the program, then run with an Android phone the same evening (results below)._ The stand-in (a throwaway
     server that speaks the stream) sent a picture of four colour quarters, coded as an Android phone does, and the
     camera's picture was compared with what it must be: all 8 rotation and mirror cases of H.264, JPEG, sizes 1920 x 1080,
     1280 x 720, 640 x 480 and full range colours were upright and right (colours within 1), 30 a second; after silence
     the slate came 2.0 s after the last picture and the picture 65 ms after the next record; bad records were survived,
     a closed connection was reconnected within about 1 s, a program restarted with another port was found; 0.00 % CPU
     idle. Against the real program (no phone) it connected and reported. **Cost, measured with the OPPO's real stream
     (2026-11-16):** the first build took 239 to 278 % of one core (15 to 17 % of a 16-thread PC). Per-thread CPU showed the
     cause: the stream was 2688 x 1512 (not 1080p) H.264, and about 14 spinning workers of the PPL pool (`parallel_for`)
     burned about 1.5 s of CPU a second on a few tens of milliseconds of work, on top of the software decoder's 0.5 to
     0.9 s. Fixed by dropping the pool and turning only the output's pixels from NV12 to BGR in one pass: 81 to 85 % of
     one core, 5.1 to 5.3 % of the PC; a decoder thread count did not help, the hardware decoder is not tried
     (`windows/README.md`, "The helper's cost on a real stream"). **After the phone-side size fix (2026-11-16, `59695e6`, `a90c8ea`)** the same 1080p ask arrives as 1920 x 1080 (High profile level 4.0, 5.9 to 6.5 Mbit/s, 31 a second written, none dropped) and the helper takes 55 % of one core, 3.4 % of the 16-thread PC, 51 MB; decode wall 214 to 219 ms a second. **Not
     checked:** a real phone's stream (colours, timing, bursts), the registered camera in Chrome, Edge and OBS with the
     helper, the program's own status and tray lines (they need the Backend, 3c), 4K, latency, a GPU decoder (`windows/README.md`,
     `windows/TESTING.md`).
     **The evening run (2026-11-16, the owner at the Windows 10 PC with the OPPO A9 2020, Chrome, Edge and OBS; `windows/TESTING.md`).** Slate, no phone: ok in Chrome, Edge and OBS (CPU 2.4 %, 2.3 % and 1.9 % of the PC for the browsers and OBS, the PC 11 % in all), the sender's picture appeared without the app opening the camera again, the slate came back after each stop (three times) and about half a second after a killed sender, uninstall then install worked and left no folder and no registry entry. With the phone: H.264 landscape and across, right way round, 1920 x 1080 at 30 a second, and JPEG; the slate within about 2 s of the camera going off and the picture back; Wi-Fi off and on, the program killed and started, the helper killed and started (the slate at once and the picture back without reopening the camera) all ok; Chrome, Edge and OBS at the same time ok; the owner felt no lag next to the program's own page (no measurement in milliseconds). **The front camera** looked mirrored in the browser's test page ("aynalıydı"), but not in OBS: the page mirrors its own self-view, so the picture from the phone and the helper is right (the owner's correction, after the first report). **Different from the expectation:** (1) the helper's CPU for H.264 1080p at 30 a second with the phone across was 239 % of one core, 15 % of the 16-core PC, 52 MB, `busy` about 300 to 340 ms a second (the stand-in showed about 130), 30 a second written, none dropped, so "a few percent" did not hold; suspected, not verified: the software decoder and the compose step's threads; JPEG took 41 % of one core, 2.6 % of the PC; (2) the Android app said 720p30 JPEG while the stream was 1920 x 1080 at 11 to 12 a second (H.264 1080p gave 30). **Still not checked:** the Windows Camera app, Zoom, Teams, 4K, the latency in milliseconds. Found on the way, and fixed: `WinHttpReadData` waits for its whole buffer (a stream of
     some tens of KB a second reached the helper seconds late, in a burst), `scSendFrame`'s sleep on the decoder's thread
     piled pictures up until they were dropped. Before it, a throwaway bridge
     outside the repository (agent, ffmpeg for turning and fitting, a small C++ sender that repeats the last frame at 30 frames a
     second) showed the OPPO's JPEG picture (back and front camera) in Chrome (the owner saw it), and an H.264 frame (front
     camera, rotation 2) upright through the camera by ffmpeg. Learned: `/video.mjpg` gives nothing for an H.264 camera, so the
     helper has to read `/video.frames` (rotation per record, no restart when it changes); ffmpeg's `-fflags nobuffer -flags
     low_delay` stall a raw H.264 input after the first frame (a fault of the bridge, not of the agent or the phone); when the
     phone or the network goes the last frame stays frozen, so the helper must let its sender go after a while without frames (it does).
   - **3c. A product.** _Written 2026-11-16, cross-compiled and vetted for Windows (amd64 and arm64), macOS and Linux, its
     zip handling tested by unit tests that stay local (the repository does not commit test files). **Run on a Windows 10 PC
     (amd64, 2026-11-16, with an owner answering each UAC window):** `camera status` before and after; `camera install` from
     the files next to the program (one UAC window; folder, registry, ACL and `version.txt` checked, the slate in Chrome);
     the helper started by the program itself when a phone turned its camera on (path under Program Files, no window, the
     picture in Chrome) and left running when the program quit; `camera install --force` while Chrome held the camera (one
     UAC window, the old DLL renamed aside, a new tab saw the camera); a UAC window answered No (friendly message, nothing
     changed); `camera uninstall` while Chrome held the DLL (unregistered, `chrome.exe` named, exit 1); `windows\release.ps1`
     without `-Upload` (the zip has four flat entries); the download path against a **local stand-in for the GitHub API**
     (`PHONESET_UPDATE_API`): a good `SHA256SUMS` installed it (version.txt `v9.9.9`, both licence texts copied), a wrong line
     and a missing line were both refused with nothing changed. Found and fixed on the way: a second `uninstall` after a held
     DLL said "not installed" and left the folder with `softcam.dll.old-*`; `windows/release/` was not in `.gitignore`.
     **Published and run from the real release (2026-11-16):** `windows\release.ps1 -Upload v0.3.0` put the zip on the v0.3.0 release
     (150756 bytes, SHA-256 `5531d20e…925b`, its line in `SHA256SUMS`, unsigned); then on the Windows 10 PC `phoneset camera install --force`
     with no override of the update address downloaded it from GitHub (`checksum ok`, `release v0.3.0`), one UAC window, `version.txt`
     `v0.3.0`, `camera status` ready and the slate in Chrome; `camera uninstall` (Chrome closed) removed the registration and the folder
     (`C:\Program Files\phoneset` gone). **Not run:** the tray's "Install the camera…" and "Remove the camera", `phoneset update`, arm64, a user
     who answers the UAC window with another account's credentials, Windows 11._ `pc/internal/camera/backend_windows.go` is the Windows
     `Backend`: `Probe` reads the camera's class id in HKLM (the 64-bit view) and the helper's named mutex, `Install` takes
     the files from next to the program (a build from `windows\build.ps1`), else keeps an installed camera, else downloads
     `phoneset-camera-windows.zip` from the latest release, checks it against `SHA256SUMS` (`update.Client.Fetch`) and unpacks
     it with limits (no paths out of the folder, no links, few and small files, `winpkg.go`), then starts the program again
     with administrator rights (`ShellExecute` with the verb "runas": the UAC window) as `phoneset camera _elevated install`,
     which copies to `C:\Program Files\phoneset\camera`, registers with `regsvr32` and checks the registry. A DLL a browser
     still has loaded cannot be overwritten or deleted but can be renamed, so an update moves the old copy aside instead of
     waiting for the browser; `uninstall` unregisters (programs stop listing the camera) and, when a program still holds the
     file, says which one. `Start` starts the helper (hidden, detached) when a phone begins to send video, with the program's
     `--config-dir`; the helper is not stopped when the program quits (it costs nothing idle). `phoneset update` refreshes it
     (`camera install --force`) and the tray's camera section works through the same `Backend`. The zip is made by
     `windows\release.ps1` (`-Upload TAG` adds it to a release and to its `SHA256SUMS`, like `macos/release.sh`); **nothing in
     it is signed** (a DirectShow filter needs no signature; an antivirus may still look at it). The elevated step is a second
     copy of the program run after a UAC prompt that names `phoneset.exe`; like any program that elevates itself it does what
     its arguments say, so it is only started by the program itself. Not done: Windows 11 with `MFCreateVirtualCamera` (looked at
     separately), a code-signing certificate.

## 5. Not decided, and not part of the first version

- Opening the phone's camera when an app on the PC opens the virtual camera. The protocol has no request from the PC
  to the phone to start the camera; the extension can tell the app when a client connects (forum thread 751413).
  This is the natural next step and needs a protocol change.
- A "fill" option instead of "fit" for a portrait picture, a mirror option for the front camera, 60 frames a second.
- Sound: a microphone already exists (the virtual microphone, BlackHole / VB-CABLE); video does not change that.
