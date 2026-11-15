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

## Releasing (signed and notarized)

`phoneset camera install` downloads `phoneset-camera-macos.zip` from the latest GitHub release when the app is not
next to the program or in /Applications. That file has to be signed with a Developer ID certificate and notarized by
Apple; `macos/release.sh` makes it (archive, export with `ExportOptions.plist`, notarize, staple, zip, checksum).

Only the owner can create what it needs:

1. A **Developer ID Application** certificate (Xcode, Settings, Accounts, Manage Certificates, +).
2. On developer.apple.com, the **System Extension** capability on the identifier `com.rapp.phoneset.camera`.
3. Notarization credentials: an App Store Connect API key (.p8, key id, issuer id), or a profile made once with
   `xcrun notarytool store-credentials` (then `NOTARY_PROFILE=<name>`).

Try it by hand first: `macos/release.sh --no-notarize` (signs only), then the full run. For CI set the repository
variable `MACOS_CAMERA=true` and the secrets `MACOS_CERT_P12_BASE64`, `MACOS_CERT_PASSWORD`, `ASC_KEY_ID`,
`ASC_ISSUER_ID`, `ASC_KEY_P8`; until then the release job skips the camera and the release has no camera app.

Not verified yet: a real signing and notarization run (no Developer ID certificate on the development Mac), and
the download path against a real release.
