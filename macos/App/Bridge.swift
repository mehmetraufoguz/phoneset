import CoreImage
import Foundation
import os

/// Carries the phone's picture from the phoneset program on this Mac to the camera extension: reads the frames,
/// decodes them, turns them upright, fits them into the camera's frame and writes them to the extension's sink stream.
/// It also says, in a line, what is going on.
final class Bridge: ObservableObject {
  @Published var status = "Starting…"
  @Published var flowing = false

  private let link = AgentLink()
  private let decoder = FrameDecoder()
  private let compositor = Compositor()
  private let sink = SinkWriter()
  /// Everything that touches the decoder, the compositor and the sink runs on this queue.
  private let work = DispatchQueue(label: "phoneset.bridge", qos: .userInteractive)

  private let countLock = NSLock()
  private var inFlight = 0
  private static let maxInFlight = 3

  private var agentUp = false
  private var lastFrameAt = Date.distantPast
  private var frames = 0
  private var failedWrites = 0
  /// How many pictures in a row found the sink's queue full: the extension is not taking them (it was replaced).
  private var fullStreak = 0
  private var windowStart = Date()
  private var lastDescription = ""
  private var timer: DispatchSourceTimer?

  // For the log line of every second: how far a frame gets (read with `log stream --predicate 'subsystem == "com.rapp.phoneset.camera"'`).
  private let log = Logger(subsystem: "com.rapp.phoneset.camera", category: "bridge")
  private let counterLock = NSLock()
  private var received = 0, dropped = 0, decoded = 0, composed = 0, written = 0, full = 0

  private func count(_ keyPath: ReferenceWritableKeyPath<Bridge, Int>) {
    counterLock.lock()
    self[keyPath: keyPath] += 1
    counterLock.unlock()
  }

  func start() {
    link.onState = { [weak self] state in
      self?.work.async { self?.agentUp = state == .connected }
    }
    link.onFrame = { [weak self] record in self?.receive(record) }
    decoder.onImage = { [weak self] image, record in
      self?.count(\.decoded)
      self?.work.async { self?.show(image, record) }
    }
    link.start()

    let timer = DispatchSource.makeTimerSource(queue: work)
    timer.schedule(deadline: .now(), repeating: 1.0)
    timer.setEventHandler { [weak self] in self?.tick() }
    timer.resume()
    self.timer = timer
  }

  func stop() {
    link.stop()
    timer?.cancel()
    work.async { self.sink.close() }
  }

  /// A record from the program (on the link's queue). When the decoder is behind, the record is dropped and the decoder
  /// waits for the next key frame, so that it never decodes a picture that depends on one that was dropped.
  private func receive(_ record: FrameRecord) {
    count(\.received)
    countLock.lock()
    let busy = inFlight >= Self.maxInFlight
    if !busy { inFlight += 1 }
    countLock.unlock()
    if busy {
      count(\.dropped)
      work.async { self.decoder.lostFrame() }
      return
    }
    work.async {
      self.decoder.decode(record)
      self.countLock.lock()
      self.inFlight -= 1
      self.countLock.unlock()
    }
  }

  private func show(_ image: CIImage, _ record: FrameRecord) {
    if !sink.isOpen { _ = sink.open() }
    guard sink.isOpen else { return }
    guard let pixels = compositor.frame(from: image, rotation: record.rotation, mirror: record.mirror) else { return }
    count(\.composed)
    switch sink.write(pixels) {
    case .written:
      count(\.written)
      failedWrites = 0
      fullStreak = 0
    case .full:
      count(\.full)
      fullStreak += 1
      if fullStreak > 60 {  // about two seconds: open the stream again
        sink.close()
        fullStreak = 0
      }
    case .failed:
      failedWrites += 1
      if failedWrites > 90 { sink.close() }  // the extension went away: find it again
    }
    frames += 1
    lastFrameAt = Date()
    let turned = record.rotation % 2 == 1
    let size = turned ? "\(record.height)×\(record.width)" : "\(record.width)×\(record.height)"
    lastDescription = "\(size) \(record.codec == FrameRecord.h264 ? "H.264" : "JPEG")"
  }

  /// Once a second: opens the sink if it is not open, and says what is going on.
  private func tick() {
    if sink.isOpen && !sink.isCurrent() { sink.close() }  // the extension was replaced or removed
    if !sink.isOpen { _ = sink.open() }
    let seconds = max(0.001, Date().timeIntervalSince(windowStart))
    let fps = Double(frames) / seconds
    frames = 0
    windowStart = Date()

    counterLock.lock()
    log.info(
      "agent=\(self.agentUp) sink=\(self.sink.isOpen) received=\(self.received) dropped=\(self.dropped) decoded=\(self.decoded) composed=\(self.composed) written=\(self.written) full=\(self.full) failedWrites=\(self.failedWrites)"
    )
    received = 0; dropped = 0; decoded = 0; composed = 0; written = 0; full = 0
    counterLock.unlock()

    let text: String
    var flowing = false
    if !sink.isOpen {
      text = "The camera isn't installed yet: press Install camera."
    } else if !agentUp {
      text = "The phoneset program isn't running on this Mac."
    } else if Date().timeIntervalSince(lastFrameAt) > 1.5 {
      text = "Waiting for your phone to turn its camera on."
    } else {
      text = "Sending your phone's picture: \(lastDescription), \(Int(fps.rounded())) frames a second."
      flowing = true
    }
    DispatchQueue.main.async {
      self.status = text
      self.flowing = flowing
    }
  }
}
