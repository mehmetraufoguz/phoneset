import SwiftUI

@main
struct PhonesetCameraApp: App {
  @StateObject private var installer = CameraInstaller()
  @StateObject private var bridge = Bridge()

  var body: some Scene {
    WindowGroup {
      VStack(alignment: .leading, spacing: 14) {
        Text("phoneset Camera").font(.title2.bold())

        VStack(alignment: .leading, spacing: 4) {
          Text("Camera").font(.headline)
          Text(installer.status)
            .foregroundStyle(installer.failed ? Color.red : Color.secondary)
            .fixedSize(horizontal: false, vertical: true)
          HStack {
            Button("Install camera") { installer.install() }
            Button("Remove camera") { installer.remove() }
          }
        }

        VStack(alignment: .leading, spacing: 4) {
          Text("Your phone").font(.headline)
          Text(bridge.status)
            .foregroundStyle(bridge.flowing ? Color.green : Color.secondary)
            .fixedSize(horizontal: false, vertical: true)
        }

        Divider()
        Text(
          "Pick “phoneset Camera” as the camera in Zoom, Photo Booth or QuickTime Player. On your phone, open "
            + "phoneset, connect to this Mac and turn on “Use as a webcam”."
        )
        .font(.footnote)
        .foregroundStyle(.secondary)
        .fixedSize(horizontal: false, vertical: true)
      }
      .padding(24)
      .frame(width: 480)
      .onAppear {
        bridge.start()
        // `open -a "phoneset Camera" --args --install` installs the camera at once, for trying a new build.
        if CommandLine.arguments.contains("--install") { installer.install() }
      }
    }
    .windowResizability(.contentSize)
  }
}
