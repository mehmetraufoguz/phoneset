import AppKit

final class AppDelegate: NSObject, NSApplicationDelegate {
  private let installer = CameraInstaller()
  private let bridge = Bridge()
  private let reporter = AgentReporter()

  func applicationDidFinishLaunching(_ notification: Notification) {
    let arguments = CommandLine.arguments
    let install = arguments.contains("--install")
    let uninstall = arguments.contains("--uninstall")
    let background = arguments.contains("--background")

    // Opened by hand (Finder) rather than by the phoneset program: say where it is controlled from.
    if !install && !uninstall && !background {
      let alert = NSAlert()
      alert.messageText = "phoneset Camera runs with phoneset"
      alert.informativeText =
        "This is the camera that shows your phone's picture in Zoom, Meet and other programs. It is installed, "
        + "started and removed by the phoneset program on this Mac: use its menu bar icon, or run `phoneset camera`."
      alert.runModal()
      NSApp.terminate(nil)
      return
    }

    if uninstall {
      installer.onFinished = { _ in DispatchQueue.main.asyncAfter(deadline: .now() + 1) { NSApp.terminate(nil) } }
      installer.remove()
      return
    }
    if install {
      // macOS replaces the extension while this app runs, and an app that was open at that moment keeps a view of the
      // cameras that is out of date: so once it is installed, the app starts again.
      installer.onFinished = { [weak self] succeeded in
        if succeeded { self?.relaunchInBackground() }
      }
      installer.install()
    }

    bridge.start()
    reporter.start { [weak self] in
      guard let self else { return ("", false) }
      return (self.bridge.status, self.bridge.flowing)
    }
  }

  private func relaunchInBackground() {
    let task = Process()
    task.executableURL = URL(fileURLWithPath: "/bin/sh")
    task.arguments = ["-c", "sleep 1; /usr/bin/open -g \"\(Bundle.main.bundlePath)\" --args --background"]
    try? task.run()
    NSApp.terminate(nil)
  }
}
