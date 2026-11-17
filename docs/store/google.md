# Google Play: graphics

What the Play listing needs and what we made. Requirements were read from Google's "Add preview assets to
showcase your app" help page on 2026-10-30; they change, so check the page again before uploading.

## What is needed, and what is not

| Asset | Needed? | Spec (Play Console) | Ours |
| --- | --- | --- | --- |
| App icon | Required | 512 x 512, 32-bit PNG with alpha, up to 1024 KB, no ranking, price or category badges | `icon-512.png` (from `mobile/assets/icon.png`) |
| Feature graphic | Required | 1024 x 500, JPEG or 24-bit PNG without alpha; no device imagery, nothing that repeats the icon, key content centred | `feature-graphic-1024x500.png` |
| Phone screenshots | At least 2 required, up to 8 | JPEG or 24-bit PNG without alpha, each side 320 to 3840 px, long side at most twice the short side | 3 at 1080 x 1920 |
| Phone screenshots, for promotion | Recommended | At least 4, 9:16, at least 1080 x 1920; tagline at most 20% of the image; no device frame, price or call to action; clean status bar | 3 of 4 so far |
| Alt text per graphic | Recommended | 140 characters or fewer | below |
| 7-inch and 10-inch tablet, Chromebook | Not needed | Optional; 4 screenshots, 1080 to 7680 px | skipped |
| Promo video | Not needed | A YouTube URL, optional | skipped |
| TV banner, Wear OS, Automotive, XR | Not needed | Only for those form factors | skipped |

One screenshot size is enough: 1080 x 1920. No other resolution is worth making.

## Files

Made in the screenshot editor project `~/github/phoneset-store` (outside this repository) and exported to
`export/play/` there: `icon-512.png`, `feature-graphic-1024x500.png`, `screenshots/phone-01..03.png`. The
screenshots are real Android captures from an emulator (Pixel, Android 16) with the status bar cropped off, in the
Signal Blue style. The feature graphic is drawn by `scripts/feature-graphic.mjs` there, because the editor's own
feature-graphic canvas always uses a dark icon-and-name template.

## Alt text

1. The phoneset screen with the microphone and headphones switched on, connected to a PC named Studio PC.
2. The How it works screen: put phoneset on your PC, start it, pair this phone, then use it.
3. The phoneset Settings screen with help links, source code and the MIT license.
4. Feature graphic: Your phone is your PC's mic. Mic and headphones for Windows and Mac.

Short description (80 characters at most): `Use your phone as your PC's microphone and headphones over Wi-Fi.` (65)

## Not done, and why

- A fourth screenshot (for promotion eligibility). The natural one is the home screen with a connected PC, but on
  Android the Disconnect button there is squeezed into a narrow column and reads "Disc / onne / ct". That is an app
  layout bug, not a capture problem, so the screen is left out until it is fixed.
- The Settings capture is cut above "About" so the version row of the older build (0.1.0) does not show.
- The emulator has no SIM, so there are no cell bars; the status bar is cropped off instead.
- Audio on a real Android phone is still unverified; the screenshots show the app's state, not sound.

---

# Google Play: internal test

Prepared on 2026-11-11. Facts about the Play Console come from Google's help pages (the pages show no
date, so check them again before acting) and are marked **unverified** where a page did not say. Facts about
the app were checked on the repository and on an OPPO A9 2020 (Android 11).

## What Google's pages say

- **Internal testing**: up to 100 testers, added as an email list under Testing > Internal testing > Testers.
  Testers install from the Play Store link or the shareable link; they cannot find the app by searching. The
  page says "You can start an internal test before completing app setup" and that an internal release can be made
  "if your app is not fully configured". The opt-in link appears only once the app's status is "Published"
  (**unverified**: what that means for a draft app with a first internal release). A first upload shows a
  temporary name and listing for up to 48 hours.
- **The 12 testers for 14 days** rule is not on that page. It is the rule for personal developer accounts before
  they may apply for production access (Google's testing requirements page, read earlier); whether it touches
  internal testing is **unverified**.
- **Data safety**: apps active only on an internal track do not need the form. For production the form and the
  privacy policy link are required even if nothing is collected. "Collect" means sending data off the device;
  data only processed on the device need not be declared; data held only in memory for the time a real-time
  request needs ("ephemeral") must be in the form but is not shown on the store page. The page does not say how
  to treat data sent only to the user's own computer (**unverified**, see the draft below).
- **App signing**: new apps are enrolled in Play App Signing automatically. Google holds the app signing key;
  the developer keeps an upload key (a Java keystore, RSA 2048 bits or more) and signs the app bundle with it.
- **Foreground service declaration** (App content): for each type a description, the user impact if the task is
  stopped, a demo video link and a use case. For the microphone type the use case is "Background Audio Access" and
  the video must "show the user initiate the microphone actively recording". It applies to apps that target
  Android 14 or newer (ours targets 36). Whether it is needed before an internal release is **unverified**.

## The app, as built

| Item | Value | Where it was read |
| --- | --- | --- |
| Package | `com.rapp.phoneset` | `mobile/app.json` |
| Version | 0.2.0, versionCode 1 (EAS raises it on every production build) | `app.json`, `eas.json` |
| minSdk / targetSdk | 24 / 36 | a release APK built today, `aapt2` |
| Permissions | internet, Wi-Fi state, multicast (mDNS), camera, microphone, foreground service (microphone), notifications, audio settings, vibrate, network state, wake lock, boot completed (the last four come from libraries) | the APK's manifest |
| Foreground service | `CentralizedForegroundService`, type microphone, runs while connected, notification "phoneset - PC name" | `dumpsys` on the phone |
| Cleartext traffic | allowed (the PC talks plain HTTP and an encrypted WebSocket on the local network) | `app.json` |

Checked on the OPPO today with the test PC on the Mac: pairing by the discovered PC and the 6-digit code, the
connection, the microphone switched on (the permission prompt appears, the PC received real room audio, a foreground
service and its notification are running, the stream continued for 12 seconds with the app in the background), the
headphones switched on ("Playing ..." with no error and no crash), and both switched off. Not checked: sound quality,
the PC's side with a real virtual cable, a locked screen, Android 14 or newer, other makes (OPPO's battery manager
may stop background apps), a run of an hour.

## Build and upload (the owner's commands)

The first Android build needs the owner's Expo login, so it is not done here:

```bash
cd mobile
npx eas-cli login
npx eas-cli build --platform android --profile production
```

EAS asks once whether to generate an Android keystore: yes. That keystore is the upload key; keep a backup
(`npx eas-cli credentials` can download it) and never commit it (`.gitignore` covers `*.jks` and `*.keystore`).
Download the `.aab` from the build page and upload it by hand in Play Console (Testing > Internal testing >
Create new release). `eas submit` needs a Google service account key; `eas.json` already says internal track and
draft, and the key file would be kept outside the repository (`play-service-account*.json` is ignored). A local
release build also works (`expo prebuild`, JDK 17, `gradlew bundleRelease`) but would be signed with a debug key.

## Play Console, in order

1. Developer account: US$25, identity verification, and the device check with a real Android 10+ phone (the OPPO,
   Android 11, qualifies) through the Play Console app.
2. Create the app: name `phoneset`, English (United States), App, **free or paid** (decide first: the App Store
   price is US$0.99; Google's price page, read 2026-11-16: a paid app can be made free later, but "once your app has
   been offered for free, the app can't be changed to paid"), accept the declarations.
3. App content (answers below). Internal testing may start before all of it is done; production needs all of it.
4. Testing > Internal testing > Testers: make an email list, add the testers' Google accounts, create a release,
   upload the `.aab`, roll it out, copy the link and open it on the phone.
5. Later, for production: a closed test, then "Apply for production access" on the Dashboard. Google's page for
   personal accounts created after 2023-11-13 (read 2026-11-16): at least 12 testers opted in continuously for 14
   days when you apply; internal testing is optional and can start before app setup is complete; a closed test can
   start after setup is complete; Production stays disabled until then. The page is about personal accounts and does
   not say whether organisation accounts are exempt; which kind the account is decides this.

## App content answers (draft, the owner decides)

| Item | Answer | Why |
| --- | --- | --- |
| Privacy policy | `https://rauf.app/phoneset/privacy/` | Updated and published 2026-11-17: covers the camera as a webcam, the camera subscription and the camera choices kept per PC |
| Ads | No | no ads |
| App access | All functionality is available without special access | no account; but a PC with the phoneset program is needed to see anything, say so in the notes |
| Content rating | Utility, no violence, sexual content, drugs, gambling, user-generated content or location sharing | the IARC questionnaire |
| Target audience | 18 and over, not designed for children | a utility for computers (**decide**) |
| News, COVID-19, government, financial, health | No | none applies |
| Data safety | see below | |
| Foreground service (microphone) | see below | |

## Data safety (draft)

The app sends audio, and with the camera switch the camera picture, only to the user's own computer on the local
network, encrypted (AES-256-GCM), and nothing to the developer or to anyone else. Two ways to answer; Google's
page does not say which one is meant for data that goes to the user's own device (**unverified**):

- **No data collected, none shared**, the same as the App Store's "Data Not Collected" and the privacy policy.
- **The conservative reading**: audio (voice or sound recordings) and video declared as collected, not shared,
  processed ephemerally, required for the app's function, encrypted in transit. Ephemeral data is not shown on
  the store page.

Either way: encryption in transit yes, a way to request deletion not applicable (no account, nothing is held).

## Foreground service declaration (microphone), draft

- **Description**: "phoneset turns the phone into the microphone of the user's own computer. While the user has the
  microphone switched on, a foreground service keeps the microphone streaming to the paired computer over the local
  Wi-Fi when the screen turns off or another app is in front. It stops when the user switches the microphone off
  or ends the connection."
- **User impact if interrupted**: "The computer would lose its microphone in the middle of a call or a recording,
  and the user would not notice until the other side complained."
- **Use case**: Background Audio Access.
- **Demo video** (a link; not recorded yet): open the app, connect to a PC, switch the microphone on, show the
  notification, go to the home screen, show that the PC still receives sound, switch the microphone off.

## Store text

Short description (80 characters at most): `Use your phone as your PC's microphone and headphones over Wi-Fi.` (65)

Full description (plain text, under 4000 characters):

```
phoneset turns your Android phone into your computer's microphone and headphones, over your own Wi-Fi.

Use the phone's microphone as the microphone of a Windows PC or a Mac: for calls, streams, recordings and games. Send the sound the computer is playing to the phone, so it comes out of the phone's headphones or earphones. Switch either one on or off whenever you like.

HOW IT WORKS
1. Install the free phoneset program on your computer (one command; the app's How it works screen links to it).
2. Start it. It shows a QR code and a 6-digit code.
3. Tap + in the app and scan the QR code, or type the code. You only pair once.
4. Open your computer in the list and switch on Microphone, Headphones or both.

PRIVATE BY DESIGN
- No account, no sign-in, no ads, no analytics, no tracking.
- Audio goes directly between your phone and your computer. There is no phoneset server in the middle.
- Everything is encrypted between the two devices.
- The microphone is only used while you have it switched on, and the app stores nothing you say.

GOOD TO KNOW
- You need the free phoneset program on a Windows 10/11 PC or a Mac (macOS 12 or later; sending the computer's sound to the phone needs macOS 14.2 or later).
- The computer sees your phone's voice through a virtual audio cable (VB-CABLE on Windows, BlackHole on macOS); the installer can set it up.
- The phone and the computer have to be on the same Wi-Fi network.
- While the microphone or the headphones are on, Android shows a notification so that the sound keeps going when the app is in the background.
- Echo cancellation is not available on Android yet. Earphones avoid echo altogether.
- Android 7.0 or later.

OPEN SOURCE
The app and the computer program are open source (MIT license): github.com/mehmetraufoguz/phoneset
```

## Tell the testers

- The "Camera (experimental)" section is a prototype: on this Android phone only a small size worked (about 24 fps at
  480p); it needs the PC program from the same release to be seen (`phoneset video`).
- Echo cancellation is not available on Android.
- If the phone is an OPPO, Xiaomi or Samsung, switch off battery optimisation for phoneset if the sound stops in
  the background (**unverified** that it is needed).
- If "Pair with <PC>" says "Couldn't reach the PC", type the PC's address under "Can't see your PC?". The PC program
  from the commit that fixes this advertises only reachable addresses.

---

# Google Play: the account exists (2026-11-16)

The developer account was bought. What is left, who does it, and what Google's pages said when read today (they
change; the numbers are in the pages named, not in this file's memory).

## The owner, in Play Console and on the phone

1. Finish the account: identity verification, then the device check with the OPPO through the Play Console mobile app
   (a real, non-rooted Android 10+ phone). The account is **personal**, so the 12-tester rule below applies.
2. Payments profile (merchant account) in Settings: needed for a paid app and for subscriptions. Turkey supports
   merchant registration (Google's supported-locations page); tax information is asked there.
3. Create the app (step 2 above) as **paid, US$0.99** (decided 2026-11-16: the Play prices follow the App Store's;
   any later in-app price is in the owner's private pricing note). Free can never become paid, so the app is not
   switched to free, not even for testing.
4. `cd mobile && npx eas-cli login`, then `npx eas-cli build --platform android --profile production`; accept the
   generated keystore (it is the upload key; back it up with `npx eas-cli credentials`, never commit it). Upload the
   `.aab` to Testing > Internal testing by hand. Internal testing needs no finished app setup.
5. App content answers: from the drafts above, after the owner has read them. The privacy policy on the website already
   covers the camera and the subscription (published 2026-11-17).
6. Closed test for production access: 12 testers opted in for 14 days in a row (see step 5 of "Play Console, in
   order"). Start the 14 days as soon as the setup allows, it is the longest wait. Finding 12 people with a Google
   account who stay opted in is the owner's part. Because the app is paid, Google's test page (read 2026-11-16) says
   "testers must purchase paid apps when participating in open or closed tests. For internal tests, testers can
   install paid apps for free." So give the closed-test testers promo codes (Play Console, Promotions; the price page
   says a paid app can be given free with a promo code; the number of codes, and that a code covers a closed-test
   install, were not read) or refund what they paid.

## Service fees (Google's page, read 2026-11-16)

Subscriptions: 10% plus a 5% billing fee (15%), new and existing installs, in the US, UK, EEA, Australia and Japan,
and 15% in the other markets. One-time sales and the up-front price of a paid app: 20% (new installs) or 25%
(existing installs) plus the 5% billing fee in those regions, 15% on the first US$1M a year elsewhere; reduced by the
Apps Experience programme (not checked whether it applies). Apple's 30% / 15% is in `apple.md` and its sources.

## What stays open in the repository

- The camera permission text in `mobile/app.json` (`expo-camera`) now says the camera also sends the picture to the PC
  (changed 2026-11-17; it reaches the iOS `Info.plist` with the next prebuild). The App Review notes for the camera
  release still say "scans the pairing QR code only" and must be rewritten before submission.
- Whether the "Camera (experimental)" section stays visible on Android (asked 2026-11-11, unanswered). What the
  OPPO measured is in `docs/webcam-measurements.md` (H.264 reached 4K at 30 fps; front-camera JPEG at 720p only
  about 6 fps).
- The foreground service demo video (microphone) is not recorded; Google wants a link to it on the declaration form.
- Subscriptions: no billing code exists, so there is nothing for a Play Billing product to attach to yet. In Play
  Console a subscription holds base plans, each with a billing period and a price (Google's page, read 2026-11-16).
  RevenueCat's guide lists "every six months" among the periods, the excerpt of Google's page did not show the list:
  check that the period needed exists when the products are created.
- Refunds (Apple's and Google's own pages, read 2026-11-16): on Apple a refund is a request at
  reportaproblem.apple.com for a purchase that "might be eligible", answered in 24 to 48 hours; on Google the store
  "may give refunds ... based on the refund policies", which differ by what was bought, when and where, and the
  developer can refund too. It is a request the store may refuse, not a trial period.
