import Foundation

/// One record of the agent's `/video.frames` stream (docs/protocol.md, section 9).
struct FrameRecord {
  static let jpeg: UInt8 = 1
  static let h264: UInt8 = 2

  let seq: UInt32
  let capturedMs: UInt32
  let codec: UInt8
  let key: Bool
  /// Quarter turns clockwise that make the decoded picture upright.
  let rotation: Int
  let mirror: Bool
  let width: Int
  let height: Int
  let payload: Data
}

/// Reads the phone's frames from the phoneset program running on this Mac: finds its port and token in control.json
/// (the program writes it while it runs, readable by this user only), opens `/video.frames` and hands over each
/// record. When the program is not running or the stream ends it tries again every second.
final class AgentLink: NSObject, URLSessionDataDelegate {
  enum State { case noAgent, connected }

  /// Both are called on the link's own queue.
  var onState: ((State) -> Void)?
  var onFrame: ((FrameRecord) -> Void)?

  private let queue = DispatchQueue(label: "phoneset.agentlink")
  private var session: URLSession!
  private var task: URLSessionDataTask?
  private var buffer = Data()
  private var running = false
  private static let headerLength = 18

  override init() {
    super.init()
    let operations = OperationQueue()
    operations.maxConcurrentOperationCount = 1
    operations.underlyingQueue = queue
    let configuration = URLSessionConfiguration.ephemeral
    // The stream is quiet while no phone sends: it must not time out.
    configuration.timeoutIntervalForRequest = 24 * 3600
    configuration.timeoutIntervalForResource = 24 * 3600
    session = URLSession(configuration: configuration, delegate: self, delegateQueue: operations)
  }

  func start() { queue.async { self.running = true; self.connect() } }

  func stop() {
    queue.async {
      self.running = false
      self.task?.cancel()
      self.task = nil
    }
  }

  /// Where the program keeps control.json: the folder in the `agentConfigDir` setting (for a test program started
  /// with --config-dir), else the program's own folder.
  private static func controlURL() -> URL {
    if let dir = UserDefaults.standard.string(forKey: "agentConfigDir"), !dir.isEmpty {
      return URL(fileURLWithPath: dir).appendingPathComponent("control.json")
    }
    let support = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
    return support.appendingPathComponent("phoneset").appendingPathComponent("control.json")
  }

  private func connect() {
    guard running else { return }
    guard
      let data = try? Data(contentsOf: Self.controlURL()),
      let info = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
      let port = info["port"] as? Int, let token = info["token"] as? String,
      let url = URL(string: "http://127.0.0.1:\(port)/video.frames?t=\(token)")
    else {
      onState?(.noAgent)
      retry()
      return
    }
    buffer = Data()
    task = session.dataTask(with: URLRequest(url: url))
    task?.resume()
  }

  private func retry() {
    queue.asyncAfter(deadline: .now() + 1) { self.connect() }
  }

  // MARK: URLSessionDataDelegate

  func urlSession(
    _ session: URLSession, dataTask: URLSessionDataTask, didReceive response: URLResponse,
    completionHandler: @escaping (URLSession.ResponseDisposition) -> Void
  ) {
    let ok = (response as? HTTPURLResponse)?.statusCode == 200
    if ok { onState?(.connected) }
    completionHandler(ok ? .allow : .cancel)
  }

  func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive data: Data) {
    buffer.append(data)
    while buffer.count >= Self.headerLength {
      let header = [UInt8](buffer.prefix(Self.headerLength))
      let length = Int(UInt32(header[14]) << 24 | UInt32(header[15]) << 16 | UInt32(header[16]) << 8 | UInt32(header[17]))
      guard buffer.count >= Self.headerLength + length else { break }
      let payload = buffer.subdata(in: Self.headerLength..<(Self.headerLength + length))
      buffer = Data(buffer.dropFirst(Self.headerLength + length))
      let flags = header[9]
      onFrame?(
        FrameRecord(
          seq: UInt32(header[0]) << 24 | UInt32(header[1]) << 16 | UInt32(header[2]) << 8 | UInt32(header[3]),
          capturedMs: UInt32(header[4]) << 24 | UInt32(header[5]) << 16 | UInt32(header[6]) << 8 | UInt32(header[7]),
          codec: header[8], key: flags & 1 != 0, rotation: Int((flags >> 1) & 3), mirror: flags & 8 != 0,
          width: Int(header[10]) << 8 | Int(header[11]), height: Int(header[12]) << 8 | Int(header[13]),
          payload: payload))
    }
  }

  func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
    guard running else { return }
    onState?(.noAgent)
    retry()
  }
}
