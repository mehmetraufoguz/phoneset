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
