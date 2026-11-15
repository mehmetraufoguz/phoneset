import SwiftUI

@main
struct PhonesetCameraApp: App {
  @StateObject private var installer = CameraInstaller()

  var body: some Scene {
    WindowGroup {
      VStack(alignment: .leading, spacing: 14) {
        Text("phoneset Camera").font(.title2.bold())
        Text(installer.status)
          .foregroundStyle(installer.failed ? Color.red : Color.secondary)
          .fixedSize(horizontal: false, vertical: true)
        HStack {
          Button("Install camera") { installer.install() }
          Button("Remove camera") { installer.remove() }
        }
        Divider()
        Text(
          "After installing, macOS asks you to allow the camera in System Settings. Then pick "
            + "“phoneset Camera” as the camera in Zoom, Photo Booth or QuickTime Player."
        )
        .font(.footnote)
        .foregroundStyle(.secondary)
        .fixedSize(horizontal: false, vertical: true)
      }
      .padding(24)
      .frame(width: 460)
    }
    .windowResizability(.contentSize)
  }
}
