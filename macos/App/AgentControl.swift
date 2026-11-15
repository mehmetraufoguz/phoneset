import Foundation

/// How to reach the phoneset program running on this Mac: its control API listens on a loopback port, and the port and
/// token are in control.json while it runs (readable by this user only).
struct AgentControl {
  let port: Int
  let token: String

  /// Where the program keeps control.json: the folder in the `agentConfigDir` setting (for a test program started
  /// with --config-dir), else the program's own folder.
  static func controlURL() -> URL {
    if let dir = UserDefaults.standard.string(forKey: "agentConfigDir"), !dir.isEmpty {
      return URL(fileURLWithPath: dir).appendingPathComponent("control.json")
    }
    let support = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
    return support.appendingPathComponent("phoneset").appendingPathComponent("control.json")
  }

  static func read() -> AgentControl? {
    guard
      let data = try? Data(contentsOf: controlURL()),
      let info = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
      let port = info["port"] as? Int, let token = info["token"] as? String
    else { return nil }
    return AgentControl(port: port, token: token)
  }

  func url(_ path: String) -> URL? {
    URL(string: "http://127.0.0.1:\(port)\(path)\(path.contains("?") ? "&" : "?")t=\(token)")
  }
}
