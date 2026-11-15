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
