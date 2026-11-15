# phoneset Camera (macOS)

The virtual camera of phoneset for macOS: a companion app and a Core Media I/O camera extension. The plan and
the state are in `docs/virtual-camera.md`.

```sh
macos/build.sh            # generates the Xcode project, builds, signs, puts the app in /Applications
macos/build.sh --no-install
```

Needs Xcode, `brew install xcodegen`, and the Apple Developer team in `project.yml` (Xcode's automatic signing
makes the provisioning profile). macOS only installs a camera extension from an app in `/Applications`: open it, press
"Install camera", and, if asked, allow it in System Settings, General, Login Items & Extensions, Camera Extensions.

`App/` is the companion app, `Extension/` the camera extension (it publishes one device, "phoneset Camera").
The Xcode project and the plists are generated and not in git.
