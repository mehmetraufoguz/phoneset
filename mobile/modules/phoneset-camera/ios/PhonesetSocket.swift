import Foundation

/// A WebSocket that the video connection runs on, owned by native code. JavaScript uses it for the plaintext
/// handshake and the authentication (a few small messages); once the connection is authenticated the video pipe
/// ([PhonesetVideoPipe]) takes it over, so that frames are sealed and sent without passing through JavaScript,
/// where React Native would turn every message into base64.
///
/// Messages that arrive are queued; one reader at a time takes the next with [next].
final class PhonesetSocket: NSObject, URLSessionWebSocketDelegate {
  typealias Reader = (Data?, String?) -> Void

  private var session: URLSession?
  private var task: URLSessionWebSocketTask?
  private let lock = NSLock()
  private var inbox: [Data] = []
  private var failure: String?
  private var reader: Reader?
  private var opened: ((String?) -> Void)?
  private var closed = false

  /// Connects; the completion gets nil when the socket is open, or why it could not be.
  func connect(url: URL, completion: @escaping (String?) -> Void) {
    let session = URLSession(configuration: .ephemeral, delegate: self, delegateQueue: nil)
    let task = session.webSocketTask(with: url)
    // A video frame can be a few megabytes; the default limit is 1 MiB.
    task.maximumMessageSize = 8 * 1024 * 1024
    lock.lock()
    self.session = session
    self.task = task
    opened = completion
    lock.unlock()
    task.resume()
  }

  func urlSession(
    _ session: URLSession, webSocketTask: URLSessionWebSocketTask, didOpenWithProtocol protocol: String?
  ) {
    lock.lock()
    let done = opened
    opened = nil
    lock.unlock()
    done?(nil)
    readNext()
  }

  func urlSession(
    _ session: URLSession, webSocketTask: URLSessionWebSocketTask, didCloseWith closeCode: URLSessionWebSocketTask.CloseCode,
    reason: Data?
  ) {
    fail("The PC closed the connection.")
  }

  func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
    // A connection that never opened ends here with the reason.
    fail(error == nil ? "The connection closed." : "Couldn't reach the PC.")
  }

  private func readNext() {
    lock.lock()
    let task = self.task
    lock.unlock()
    task?.receive { [weak self] result in
      guard let self else { return }
      switch result {
      case .success(let message):
        if case .data(let data) = message { self.push(data) }
        self.readNext()
      case .failure:
        self.fail("The connection failed.")
      }
    }
  }

  private func push(_ data: Data) {
    lock.lock()
    if let reader {
      self.reader = nil
      lock.unlock()
      reader(data, nil)
      return
    }
    inbox.append(data)
    lock.unlock()
  }

  private func fail(_ message: String) {
    lock.lock()
    if failure == nil { failure = message }
    let why = failure
    let waiting = reader
    reader = nil
    let opening = opened
    opened = nil
    lock.unlock()
    opening?(why)
    waiting?(nil, why)
  }

  /// The next message, or why there will be none. One reader at a time.
  func next(_ completion: @escaping Reader) {
    lock.lock()
    if !inbox.isEmpty {
      let data = inbox.removeFirst()
      lock.unlock()
      completion(data, nil)
      return
    }
    if let why = failure {
      lock.unlock()
      completion(nil, why)
      return
    }
    reader = completion
    lock.unlock()
  }

  func send(_ data: Data, completion: ((String?) -> Void)? = nil) {
    lock.lock()
    let task = self.task
    lock.unlock()
    task?.send(.data(data)) { [weak self] error in
      if error != nil { self?.fail("The connection dropped.") }
      completion?(error == nil ? nil : "The connection dropped.")
    }
  }

  func close() {
    lock.lock()
    let task = self.task
    let session = self.session
    closed = true
    lock.unlock()
    task?.cancel(with: .normalClosure, reason: nil)
    session?.invalidateAndCancel()
    fail("The connection closed.")
  }
}
