# phoneset Camera (macOS)

The virtual camera of phoneset for macOS: a companion app and a Core Media I/O camera extension. The plan and
the state are in `docs/virtual-camera.md`.

```sh
macos/build.sh            # generates the Xcode project, builds, signs, puts the app in /Applications
macos/build.sh --no-install
```

Needs Xcode, `brew install xcodegen`, and the Apple Developer team in `project.yml` (Xcode's automatic signing
makes the provisioning profile). macOS only installs a camera extension from an app in `/Applications`. The app has no window: the phoneset program
controls it (`phoneset camera install | uninstall | status`, the menu bar icon, `phoneset status`) and starts it when a
phone turns its camera on; after `phoneset camera install` the user allows the camera once in System Settings, General,
Login Items & Extensions, Camera Extensions. The app is started by hand with `--background`, `--install` or `--uninstall`.

`App/` is the companion app, `Extension/` the camera extension (it publishes one device, "phoneset Camera").
The Xcode project and the plists are generated and not in git.

## Trying a new build

`macos/build.sh` replaces the app and `phoneset camera install` (or `open -a "phoneset Camera" --args --install`) asks macOS to
replace the extension (its version is the build time; removing needs the installed version to be the app's, so the
app activates before it deactivates). The app finds the phoneset program through `control.json` in
`~/Library/Application Support/phoneset`, or in the folder set with
`defaults write com.rapp.phoneset.camera agentConfigDir <folder>` (for a program started with `--config-dir`).
What the app does is logged under the subsystem `com.rapp.phoneset.camera` (the extension's:
`com.rapp.phoneset.camera.extension`):

```sh
/usr/bin/log show --last 1m --info --debug --predicate 'subsystem BEGINSWITH "com.rapp.phoneset.camera"'
```

## Releasing (signed and notarized, from this Mac)

`phoneset camera install` downloads `phoneset-camera-macos.zip` from the latest GitHub release when the app is not
next to the program or in /Applications. That file has to be signed with a Developer ID certificate and notarized by
Apple; `macos/release.sh` makes it (archive, export with `ExportOptions.plist`, notarize, staple, zip, checksum).
It is done by hand on a Mac, not in CI: no certificate or key is ever in GitHub.

One-time setup, on Apple's side (only the account holder can do it):

1. **Developer ID Application certificate.** Xcode, Settings, Accounts, pick the team, Manage Certificates, +,
   Developer ID Application. (Needs the paid Apple Developer Program; the key stays in this Mac's keychain.) The
   account has none yet (`asc certificates list` shows only Development and iOS Distribution), and `asc` can't make
   it: the API answers "can only be performed by the Account Holder", so it has to be Xcode or the portal.
2. **System Extension capability** on `com.rapp.phoneset.camera`: already on (`asc bundle-ids capabilities list
   --bundle K833XU23GP` shows SYSTEM_EXTENSION_INSTALL). Nothing to do.
3. **Notarization credentials.** appleid.apple.com, Sign-In and Security, App-Specific Passwords, make one named
   "phoneset notary". Then in a terminal, once (it asks for the password; type it there):
   `xcrun notarytool store-credentials phoneset-notary --apple-id <your Apple ID> --team-id JY83GWM55C`
   (An App Store Connect API key works too: `ASC_KEY_PATH`, `ASC_KEY_ID`, `ASC_ISSUER_ID`, see release.sh.)
4. If notarization answers 403, accept the new agreements at developer.apple.com and appstoreconnect.apple.com.

Each release: push the tag and let the release job make the release, then on this Mac

    NOTARY_PROFILE=phoneset-notary macos/release.sh --version X.Y.Z --upload vX.Y.Z

(needs `gh auth login`). It builds, signs, notarizes, uploads `phoneset-camera-macos.zip` and adds its line to the
release's `SHA256SUMS`. Before the first real release, check the certificate and the profile with
`macos/release.sh --no-notarize` (signs only, uploads nothing).

Checked on the development Mac (2026-11-15): `NOTARY_PROFILE=phoneset-notary macos/release.sh --version 0.2.1` builds,
exports with the Developer ID certificate (hardened runtime, secure timestamp, `com.apple.developer.system-extension.install`
in the app, a "Mac Team Direct" provisioning profile), Apple accepts the notarization, the ticket is stapled, and the
unpacked zip passes `stapler validate` and `spctl` ("Notarized Developer ID"). If double-clicking the downloaded `.cer`
fails with error -25294, import it from a terminal: `security import <file>.cer -k ~/Library/Keychains/login.keychain-db`.

Installing it (2026-11-15): `phoneset camera install --force` downloaded the notarized zip from a local fake release
(checksum ok), replaced the development build in /Applications, and macOS took the Developer ID build's extension
(`[activated enabled]`, no new approval, no crash or launch rejection) although the extension has no provisioning profile
of its own.

Not verified yet: the phone's picture through the Developer ID build (no phone was connected during the test), and the
download from a real GitHub release.
