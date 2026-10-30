# App Store submission: phoneset

Everything App Store Connect asks for, ready to paste. Limits were read from Apple's App Store Connect
reference on 2026-10-29 (screenshot specifications, platform version information, app information).
Items marked **decide** or **unverified** need a person before submitting.

## 1. The app record

| Field                     | Value                                                                                                                              |
| ------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| Platform                  | iOS                                                                                                                                |
| Name (2 to 30 characters) | `phoneset` (check that it is free; fallback: `phoneset: Phone as PC Mic`)                                                          |
| Primary language          | English (U.S.)                                                                                                                     |
| Bundle ID                 | `com.rapp.phoneset` (register it under Identifiers first, or the New App list will not offer it)                                   |
| SKU                       | `phoneset-ios`                                                                                                                     |
| Version                   | `0.2.0`, the same as the PC program's latest release (`app.json`, `mobile/package.json`)                                           |
| Price                     | one price tier, the lowest Apple offers is US$0.99 (the plan was "$1")                                                             |
| Availability              | all countries; **turn off** "Make this app available on Apple silicon Macs" (a phone-to-PC bridge makes no sense running on a Mac) |

`private/setup.sh` applies everything below that `asc` can set (texts from `private/metadata/`, categories, age rating,
content rights, price, availability, copyright, release type, review contact and notes, the build); it is a dry run
unless `CONFIRM=1`. The `private/` folder is git-ignored because the repository is public, so it exists on the
owner's Mac only. App Privacy, the "Regulations and Permits" declarations and the Apple silicon Mac checkbox
are website-only. Creating the record cannot be done with the App Store Connect API. Either use the website (My Apps, +, New App)
or `asc web apps create --name "phoneset" --bundle-id "com.rapp.phoneset" --sku "phoneset-ios"`, which signs in
with a web session.

Already done on the account and not repeated per app: Paid Applications Agreement, tax and banking, the
EU trader status.

## 2. Product page

| Field              | Limit                                                          | Text                                                                                                                                                        |
| ------------------ | -------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Subtitle           | 30 characters                                                  | `Mic and headphones for a PC` (27)                                                                                                                          |
| Promotional text   | 170 characters                                                 | `Use your iPhone as a wireless microphone and headphones for your Windows PC or Mac. Pair once with a QR code, then it all runs over your own Wi-Fi.` (147) |
| Keywords           | 100 bytes, each longer than 2 characters, no other apps' names | `microphone,wireless,windows,mac,audio,wifi,stream,voice,virtual,cable,speaker,gaming,computer` (93)                                                        |
| Support URL        | required                                                       | `https://rauf.app/phoneset/support/`                                                                                                                        |
| Marketing URL      | optional                                                       | `https://rauf.app/phoneset/`                                                                                                                                |
| Privacy Policy URL | required                                                       | `https://rauf.app/phoneset/privacy/`                                                                                                                        |
| Copyright          | required                                                       | **decide**: `2026 <name or brand>` (Apple adds the symbol)                                                                                                  |
| Categories         |                                                                | Utilities (primary), Productivity (secondary)                                                                                                               |
| What's New         | not available for the first version                            |                                                                                                                                                             |

Description (4000 characters, plain text, 1677 used). It names no price: guideline 2.3.14 keeps prices out of
creative assets, and a price in the text goes stale.

```
phoneset turns your iPhone into your computer's microphone and headphones, over your own Wi-Fi.

Use the phone's microphone as the microphone of a Windows PC or a Mac: for calls, streams, recordings and games. Send the sound the computer is playing to the phone, so it comes out of the phone's headphones or earphones. Switch either one on or off whenever you like.

HOW IT WORKS
1. Install the free phoneset program on your computer (one command; the app's How it works screen links to it).
2. Start it. It shows a QR code and a 6-digit code.
3. Tap + in the app and scan the QR code, or type the code. You only pair once.
4. Open your computer in the list and switch on Microphone, Headphones or both.

PRIVATE BY DESIGN
- No account, no sign-in, no ads, no analytics, no tracking.
- Audio goes directly between your phone and your computer. There is no phoneset server in the middle.
- Everything is encrypted between the two devices, using the iPhone's own cryptography.
- The microphone is only used while you have it switched on, and the app stores nothing you say.

GOOD TO KNOW
- You need the free phoneset program on a Windows 10/11 PC or a Mac (macOS 12 or later; sending the computer's sound to the phone needs macOS 14.2 or later).
- The computer sees your phone's voice through a virtual audio cable (VB-CABLE on Windows, BlackHole on macOS); the installer can set it up.
- The phone and the computer have to be on the same Wi-Fi network.
- An echo cancellation switch is there for when the phone plays sound out loud. Earphones avoid echo altogether.

OPEN SOURCE
The app and the computer program are open source (MIT license): github.com/mehmetraufoguz/phoneset
```

## 3. Screenshots

Required: iPhone with Dynamic Island (medium display), **1206 x 2622 or 1179 x 2556**, portrait, PNG or JPEG,
**no alpha channel**, 1 to 10 images. Other sizes scale from that one, so a single set is enough. The iPhone
17 Pro simulator takes 1206 x 2622 screenshots natively.

Assets must not show a price (2.3.14). A first set is in `docs/store/screenshots/ios/` (JPEG, so no alpha
channel; 1206 x 2622, status bar set to 9:41 with `xcrun simctl status_bar`): the home screen with a connected
PC, Settings, and How it works.

Still to shoot, and why they are not here yet: the connected screen with the microphone on. The simulator has
no microphone unless one is picked in Simulator, I/O, Audio Input (the app then shows that error), and the
synthetic test PC names its devices "Dev Speakers", which would look wrong on a store page. Shoot it once
against the real Windows PC (the simulator can pair with it over the Wi-Fi; remove the simulator afterwards
with `phoneset forget`) or on an iPhone 16 or 17 class phone, which takes 1206 x 2622 natively. The
iPhone 13 Pro takes 1170 x 2532, which only counts as an optional size.

## 4. App Review information

Contact name, email and phone (international format, with the plus sign) are filled in on the website; they
are not kept in this repository. No sign-in is needed, so leave the demo account empty.

Notes (4000 bytes at most; the same text is `private/review-notes.txt`, which `setup.sh` sends):

```
phoneset is the phone half of a pair. It turns the iPhone into the microphone and headphones of the user's own Windows PC or Mac, over the local Wi-Fi. It needs a free companion program running on a computer. There is no account, no sign-in and no demo account.

To try it yourself with a Mac on the same Wi-Fi as the iPhone:
1. In Terminal on the Mac: curl -fsSL https://raw.githubusercontent.com/mehmetraufoguz/phoneset/main/scripts/install-mac.sh | sh
2. Open a new Terminal window and run: phoneset
   It prints a QR code and a 6-digit code.
3. In the app tap + (top right), then Scan QR code, or type the 6-digit code.
4. The Mac appears in the list. Open it. Pairing and the connection work as they are; switching Microphone on needs a virtual audio device on the Mac (BlackHole, which the installer adds with --with-blackhole), so without it the app says the microphone is not available on that PC. That is the only reason, not a fault of the app.

Permissions:
- Microphone: voice is sent only to the paired computer while the Microphone switch is on. The app records and stores nothing.
- Camera: scans the pairing QR code only.
- Local Network: finds the computer on the Wi-Fi. The app contacts no other server.
- Audio background mode: keeps the microphone and headphone streams going while the app is in the background (guideline 2.5.4).

The gear at the top left of the home screen opens Settings (support, privacy policy, source code, version). There are no hidden or dormant features and no in-app purchases; the app itself is paid. Source: https://github.com/mehmetraufoguz/phoneset
```

**Review risk (2.1).** A reviewer without a Windows PC will see "not available on this PC" for the microphone on
a Mac without BlackHole. A screen recording of the whole flow (real iPhone, Windows PC) would make the review
pass; it is not recorded yet and the owner chose to submit without it. If App Review asks for it, record it and
answer in the Resolution Center. Two things would lower the risk: a clearer message in the app (the PC knows why the microphone is unavailable) and a
review-friendly mode in the PC program.

## 5. App Privacy, age rating, export compliance

- **App Privacy: Data Not Collected.** Matches the privacy policy and the app's privacy manifest
  (`NSPrivacyCollectedDataTypes` empty, `NSPrivacyTracking` false). The pairing list and a random device id
  stay on the phone; audio only goes to the user's own computer.
- **Age rating:** the expected answers are all "None" or "No" (no user-generated content, no messaging, no
  unrestricted web access in the app, no ads). Answer the questionnaire in App Store Connect itself; it
  changed in 2026 and is not listed here.
- **Content rights:** the app does not contain, show or access third-party content.
- **Export compliance:** the only encryption is the operating system's own (CryptoKit through the local
  module `mobile/modules/phoneset-crypto`, Keychain, `URLSession`); no cryptography library is in the app.
  By Apple's table that needs no documentation and `ITSAppUsesNonExemptEncryption = NO`, the same reading
  RaBalance uses; the key is set in `app.json`. Checked in a Release build: no cipher code in the JavaScript
  bundle, no OpenSSL-style library in the binary or its frameworks, and only CryptoKit and Security linked.
  This is the owner's declaration to make; if the reading changes, remove the key and answer in App Store
  Connect instead.

## 6. Build and upload

1. `eas.json` has a `production` profile (local version source, auto increment) and the submit block with the
   team id; `ascAppId` goes in after the record exists.
2. `eas login` (interactive), `eas init`, then `eas build --platform ios --profile production`. EAS manages the
   distribution certificate and the App Store profile.
3. `eas submit --platform ios --profile production`, or upload the build with `asc`. Wait for processing, answer
   nothing new (export compliance comes from the key), attach the build to the version.
4. Apple may email warnings about required-reason APIs after the first upload; the app's privacy manifest
   declares UserDefaults, SystemBootTime and FileTimestamp.
5. TestFlight first: install the build and run the real flow before sending it to review.

## 7. Not verified

- The new Settings and How it works screens on iOS (typecheck and lint only; Android was run on an emulator).
- Background streaming with the screen locked on the iPhone.
- The macOS system-audio capture's actual audio content and the BlackHole microphone path on a fresh Mac.
