import Foundation

/// Tells the phoneset program, once a second, what the camera app is doing, so that `phoneset status`, `phoneset camera`
/// and the tray can show it (the program is the one place everything is controlled from).
final class AgentReporter {
  private let session = URLSession(configuration: .ephemeral)
  private var timer: DispatchSourceTimer?

  /// `line` is a function so that each report is made from the state of that moment.
  func start(_ line: @escaping () -> (text: String, flowing: Bool)) {
    let timer = DispatchSource.makeTimerSource(queue: .global(qos: .utility))
    timer.schedule(deadline: .now() + 1, repeating: 1.0)
    timer.setEventHandler { [weak self] in
      guard let self, let control = AgentControl.read(), let url = control.url("/api/camera/report") else { return }
      let state = line()
      var request = URLRequest(url: url, timeoutInterval: 2)
      request.httpMethod = "POST"
      request.setValue("application/json", forHTTPHeaderField: "Content-Type")
      request.httpBody = try? JSONSerialization.data(withJSONObject: ["line": state.text, "flowing": state.flowing])
      self.session.dataTask(with: request).resume()
    }
    timer.resume()
    self.timer = timer
  }
}
