# Testing the Windows camera at the PC

About 40 minutes, the OPPO next to you, one administrator prompt (UAC) at the start and one at the end. Everything below
was built and tried by a program standing in for the phone (`README.md`, "What was seen"); what is left is what only a
person with a phone and an app can see. Write what you see next to each line of the table at the bottom, "ok" or what was
different, and send it back with the log (last section).

Chrome and Edge keep a camera they have used loaded: **close them completely before the install and the uninstall**, and
open them again after.

## 1. Set up

In a normal PowerShell, in the repository:

```powershell
cd $HOME\githubProjects\phoneset
git pull --ff-only
windows\build.ps1
cd pc
$env:CGO_ENABLED = '0'
go build -o ..\windows\dist\phoneset-dev.exe .\cmd\phoneset
cd ..
```

Close Chrome and Edge, then install the camera (the UAC window: it copies the DLL to `C:\Program Files\phoneset\camera` and
registers it, nothing else):

```powershell
windows\install.ps1
```

Open Chrome and a camera test page (for example `https://www.veed.io/tools/webcam-test`), allow the camera and choose
**phoneset Camera**. Keep the page open for the checks in part A.

## A. The slate, no phone

| # | Do | You should see |
| - | -- | -------------- |
| A1 | Nothing running but the page: the camera chosen on the page | A dark picture with "phoneset Camera" and "Waiting for your phone…" in white, and a thin blue bar on the bottom edge that moves to the right all the time. (Before, the camera could not be opened at all.) |
| A2 | The same in Edge, and in OBS (a "Video Capture Device" source, phoneset Camera) | The same slate. Bonus, if you have them: the Windows Camera app, Zoom, Teams: write what each shows. |
| A3 | Task Manager, Details, sort by CPU, with the slate on the screen | Nothing from `chrome.exe` or `msedge.exe` that stands out (a few percent at most). |
| A4 | `Start-Process windows\dist\sender.exe` (a console window opens), page still open | Bouncing balls on the page **without opening the camera again**. |
| A5 | Close the sender's window. Do A4 and A5 three times | The slate comes back each time, no frozen picture, no hang. |
| A6 | Start the sender again, then `Stop-Process -Name sender -Force` | The slate within a second. Closing the tab or the browser afterwards does not hang. |
| A7 | Close Chrome and Edge, `windows\uninstall.ps1`, then `windows\install.ps1`, open Chrome, the page, phoneset Camera | Uninstall prints what it removed (if it says a program holds the DLL, close that program and run it again); after the install the slate shows again. |

## B. With the phone

Start the program and the helper (the program has a port of its own, so your own phoneset can keep running; if the phone
cannot find "Windows Test", quit your own phoneset and leave out `'--port','47840'`):

```powershell
Start-Process windows\dist\phoneset-dev.exe -WindowStyle Hidden -ArgumentList 'serve','--config-dir','C:\phoneset-test','--name','"Windows Test"','--port','47840','--no-mic','--no-speaker','--no-tray','--verbose' -RedirectStandardOutput C:\phoneset-test\agent.log -RedirectStandardError C:\phoneset-test\agent.err
Start-Process windows\dist\phoneset-camera.exe -ArgumentList '--config-dir','C:\phoneset-test'
```

On the phone: open phoneset, tap "Windows Test", turn **Camera as a webcam** on (the app has the mode button under it, it
steps through h264 and jpeg modes). The page in Chrome, with phoneset Camera chosen, shows the phone's picture; the
program's own page (`windows\dist\phoneset-dev.exe video --config-dir C:\phoneset-test --open`) shows what the program
gets, to compare with.

| # | Do | You should see |
| - | -- | -------------- |
| B1 | Back camera, **h264**, phone upright (portrait) | The picture upright, filling the height, black bars at the sides. Smooth. |
| B2 | Turn the phone across (landscape), then back | Upright in both, the picture fills the width when across. Writing on a sheet reads the right way round. |
| B3 | The same with the mode button on **jpeg** | The same. |
| B4 | Front camera (the app's Front button), h264 and jpeg | Upright; writing held to the camera reads the right way round, not mirrored (the program does not mirror the front camera). |
| B5 | Turn the camera switch off in the app, wait, turn it on | The slate about 2 seconds after the camera goes off; the picture back within a second of it coming on. |
| B6 | Phone Wi-Fi off for 10 seconds, then on (reconnect in the app if it asks, camera on) | The slate within about 2 seconds, the picture back after the reconnect. |
| B7 | `Stop-Process -Name phoneset-dev`, wait 5 seconds, start the program again (the first command above), reconnect the phone | The slate while it is gone; the picture returns after the phone is connected again. Nothing needs to be started again by hand. |
| B8 | With a picture on the page: `Stop-Process -Name phoneset-camera -Force`, then `Start-Process windows\dist\phoneset-camera.exe -ArgumentList '--config-dir','C:\phoneset-test'` | The slate within about a second of the kill (not a frozen picture); the picture back within a few seconds of the start, **without opening the camera on the page again**. |
| B9 | The picture in Chrome, Edge and OBS (and, bonus, Camera app, Zoom, Teams) at the same time or one after the other | The picture in each. Write which ones were tried. |
| B10 | Latency: wave a hand in front of the phone; watch the program's own page next to the Chrome page | The virtual camera is not behind the program's own page by more than a few frames. Write how it feels (and, if you can, the delay in milliseconds from a video of the two screens). |
| B11 | CPU: Task Manager, Details, `phoneset-camera.exe` and the browser, in h264 1080p and in jpeg 720p | The helper: a few percent of the whole PC at most (the log's `busy=` is its milliseconds of work in a second: about 130 for 1080p on the stand-in). Write the numbers. |

If you take numbers, use the lines of `docs/webcam-measurements.md` (appendix A.8): for example
`back h264 requested 1920x1080@30 | 30 fps in Chrome | helper busy 130 ms a second, 1 % CPU | measured 20 s | feel: no lag`.

## C. Clean up

```powershell
windows\dist\phoneset-camera.exe --quit
Stop-Process -Name phoneset-dev
```

Close Chrome and Edge, then (leave the camera installed if you want to go on using it):

```powershell
windows\uninstall.ps1
```

## What to send back

The table above with what you saw in each line (ok, or what was different, and which phone mode), and the end of the
helper's log, which says what it received, dropped, decoded and wrote each second and what the decoder said about the
stream's colours (it contains no token):

```powershell
Get-Content C:\phoneset-test\camera-helper.log -Tail 80
```

A screenshot of anything that looked wrong, and the file `C:\phoneset-test\agent.err` if the program did not start.
