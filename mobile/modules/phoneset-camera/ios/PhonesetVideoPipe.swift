import CryptoKit
import Foundation

/// The hot path of the video connection, in native code: it pulls frames from the camera, seals them
/// (AES-256-GCM as in docs/protocol.md, section 4), sends them on the socket, reads the PC's acknowledgements and
/// keeps the window of how much may be in flight (section 8.4). JavaScript opens the connection, authenticates
/// and negotiates the mode, then hands the keys and the counters over; from then on it only gets the control
/// messages it has to look at (not the acknowledgements) and, once a second, the numbers.
///
/// All state is touched on one serial queue.
final class PhonesetVideoPipe {
  struct Window {
    let frames: Int
    let bytes: Int
  }

  private static let frameType: UInt8 = 0x04
  private static let controlType: UInt8 = 0x01
  private static let tagLength = 16

  private let queue = DispatchQueue(label: "phoneset.video.pipe")
  private let socket: PhonesetSocket
  private let sendKey: SymmetricKey
  private let receiveKey: SymmetricKey
  private let transcript: Data
  private let window: Window
  private let depth: Int
  private let source: PhonesetCameraCore

  private var sent: UInt64
  private var received: UInt64
  private var pulls = 0
  private var inflight: [UInt32: (at: CFAbsoluteTime, size: Int)] = [:]
  private var inflightBytes = 0
  private var lastSize = 0
  private var ended: String?

  // What JavaScript is told, taken and reset by [numbers].
  private var sentFrames = 0
  private var sentBytes = 0
  private var ackSeconds = 0.0
  private var acks = 0
  private var sendSeconds = 0.0
  private var sends = 0

  // Control messages JavaScript has to see, and the one that is waiting for the next.
  private var controls: [String] = []
  private var controlReader: ((String?, String?) -> Void)?

  init(
    socket: PhonesetSocket, sendKey: Data, receiveKey: Data, transcript: Data, sent: UInt64, received: UInt64,
    window: Window, depth: Int, source: PhonesetCameraCore
  ) {
    self.socket = socket
    self.sendKey = SymmetricKey(data: sendKey)
    self.receiveKey = SymmetricKey(data: receiveKey)
    self.transcript = transcript
    self.sent = sent
    self.received = received
    self.window = window
    self.depth = max(1, depth)
    self.source = source
  }

  func start() {
    queue.async {
      self.receiveLoop()
      self.request()
    }
  }

  /// Ends the pipe (and the socket) with a reason. Safe to call more than once.
  func stop(_ reason: String) {
    queue.async { self.end(reason) }
  }

  // ---------------------------------------------------------------- sending

  private var windowOpen: Bool {
    inflight.count < window.frames && (inflight.isEmpty || inflightBytes + lastSize <= window.bytes)
  }

  /// Asks the camera for pictures while the window has room and fewer than [depth] requests are out.
  private func request() {
    guard ended == nil else { return }
    while pulls < depth && windowOpen {
      pulls += 1
      source.next { [weak self] frame, error in
        guard let self else { return }
        self.queue.async {
          self.pulls -= 1
          guard self.ended == nil else { return }
          if let frame {
            self.send(frame)
          } else {
            self.end("The camera stopped: \(error ?? "no frame")")
          }
        }
      }
    }
  }

  private func send(_ frame: Data) {
    guard frame.count >= 4 else { return }
    let seq = frame.prefix(4).reduce(UInt32(0)) { ($0 << 8) | UInt32($1) }
    inflight[seq] = (CFAbsoluteTimeGetCurrent(), frame.count)
    inflightBytes += frame.count
    lastSize = frame.count
    sentFrames += 1
    sentBytes += frame.count
    // A frame counts as in flight from the moment it exists, so the window decides when the next is asked for.
    request()

    let before = CFAbsoluteTimeGetCurrent()
    var plain = Data(capacity: 1 + frame.count)
    plain.append(Self.frameType)
    plain.append(frame)
    do {
      let sealed = try seal(plain)
      socket.send(sealed)
    } catch {
      end("Couldn't seal a video frame.")
      return
    }
    sendSeconds += CFAbsoluteTimeGetCurrent() - before
    sends += 1
  }

  /// ciphertext || tag, with the counter as the nonce (4 zero bytes, then the counter as a big-endian u64).
  private func seal(_ plain: Data) throws -> Data {
    let box = try AES.GCM.seal(
      plain, using: sendKey, nonce: try AES.GCM.Nonce(data: nonce(sent)), authenticating: transcript)
    sent += 1
    return box.ciphertext + box.tag
  }

  private func open(_ message: Data) throws -> Data {
    guard message.count >= Self.tagLength else { throw PipeFailure() }
    let box = try AES.GCM.SealedBox(
      nonce: try AES.GCM.Nonce(data: nonce(received)),
      ciphertext: message.prefix(message.count - Self.tagLength),
      tag: message.suffix(Self.tagLength))
    let plain = try AES.GCM.open(box, using: receiveKey, authenticating: transcript)
    received += 1
    return plain
  }

  private func nonce(_ counter: UInt64) -> Data {
    var data = Data(count: 4)
    withUnsafeBytes(of: counter.bigEndian) { data.append(contentsOf: $0) }
    return data
  }

  // -------------------------------------------------------------- receiving

  private func receiveLoop() {
    socket.next { [weak self] data, error in
      guard let self else { return }
      self.queue.async {
        guard self.ended == nil else { return }
        guard let data else {
          self.end(error ?? "The connection closed.")
          return
        }
        do {
          self.handle(try self.open(data))
        } catch {
          self.end("A message from the PC failed to authenticate.")
          return
        }
        self.receiveLoop()
      }
    }
  }

  private func handle(_ plain: Data) {
    guard plain.first == Self.controlType, plain.count > 1 else { return }  // other kinds are ignored
    let body = plain.dropFirst()
    guard let object = try? JSONSerialization.jsonObject(with: body) as? [String: Any],
      let tag = object["t"] as? String
    else { return }
    switch tag {
    case "vack":
      guard let number = object["seq"] as? NSNumber else { return }
      if let entry = inflight.removeValue(forKey: number.uint32Value) {
        inflightBytes -= entry.size
        ackSeconds += CFAbsoluteTimeGetCurrent() - entry.at
        acks += 1
      }
      request()
    case "vkey":
      source.requestKeyFrame()
    default:
      // vstats, an error, the PC's answer to the mode: for JavaScript.
      if let text = String(data: body, encoding: .utf8) { give(text) }
    }
  }

  // ------------------------------------------------------------- for JavaScript

  private func give(_ text: String) {
    if let reader = controlReader {
      controlReader = nil
      reader(text, nil)
    } else {
      controls.append(text)
    }
  }

  /// The next control message JavaScript has to look at, or why the pipe ended. One reader at a time.
  func nextControl(_ completion: @escaping (String?, String?) -> Void) {
    queue.async {
      if !self.controls.isEmpty {
        completion(self.controls.removeFirst(), nil)
      } else if let why = self.ended {
        completion(nil, why)
      } else {
        self.controlReader = completion
      }
    }
  }

  /// What was done since the last call, for the once-a-second report.
  func numbers() -> [String: Double] {
    queue.sync {
      let out: [String: Double] = [
        "sentFrames": Double(sentFrames),
        "sentBytes": Double(sentBytes),
        "ackSeconds": ackSeconds,
        "acks": Double(acks),
        "sendSeconds": sendSeconds,
        "sends": Double(sends),
        "inflight": Double(inflight.count),
      ]
      sentFrames = 0
      sentBytes = 0
      ackSeconds = 0
      acks = 0
      sendSeconds = 0
      sends = 0
      return out
    }
  }

  private func end(_ reason: String) {
    guard ended == nil else { return }
    ended = reason
    socket.close()
    if let reader = controlReader {
      controlReader = nil
      reader(nil, reason)
    }
  }
}

private struct PipeFailure: Error {}
