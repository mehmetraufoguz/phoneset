import CoreMedia
import CoreMediaIO
import Foundation
import os

/// Writes the camera's pictures to the sink stream of the camera extension, through the Core Media I/O C functions.
/// Not thread safe: use it from one queue.
final class SinkWriter {
  /// The device's unique ID, set in macos/Extension/DeviceSource.swift.
  static let deviceUID = "5B6C0E2A-8A3F-4D3B-9E0A-7C1D2F4A6B11"

  private var deviceID = CMIODeviceID(0)
  private var streamID = CMIOStreamID(0)
  private var queue: CMSimpleQueue?
  private var format: CMFormatDescription?
  private(set) var isOpen = false
  private let log = Logger(subsystem: "com.rapp.phoneset.camera", category: "sink")

  enum Written { case written, full, failed }

  /// Finds the camera and opens its sink stream. False while the camera is not installed (or not ready yet).
  func open() -> Bool {
    if isOpen { return true }
    guard let device = Self.findDevice(uid: Self.deviceUID) else {
      log.debug("the camera is not in the list of devices")
      return false
    }
    let streams = Self.streams(of: device)
    // The sink stream is the one the host writes to: an output stream (direction 0). The extension adds the source
    // stream first and the sink stream second, which is the way to go when the direction cannot be read.
    let directions = streams.map { Self.direction(of: $0) }
    log.info("device \(device, privacy: .public) streams \(streams, privacy: .public) directions \(directions.map { $0.map(String.init) ?? "?" }, privacy: .public)")
    let sink: CMIOStreamID
    if let at = directions.firstIndex(where: { $0 == 0 }) {
      sink = streams[at]
    } else if streams.count >= 2 {
      sink = streams[1]
    } else {
      return false
    }

    var unmanaged: Unmanaged<CMSimpleQueue>?
    let copied = CMIOStreamCopyBufferQueue(sink, { _, _, _ in }, nil, &unmanaged)
    guard copied == noErr, let unmanaged else {
      log.error("CMIOStreamCopyBufferQueue failed: \(copied)")
      return false
    }
    let started = CMIODeviceStartStream(device, sink)
    guard started == noErr else {
      log.error("CMIODeviceStartStream failed: \(started)")
      return false
    }
    deviceID = device
    streamID = sink
    queue = unmanaged.takeRetainedValue()
    format = nil
    isOpen = true
    log.info("sink open: stream \(sink), queue capacity \(CMSimpleQueueGetCapacity(self.queue!))")
    return true
  }

  /// Whether the stream that is open is still the camera's: false once the extension was replaced or removed (the
  /// camera then has new stream numbers, or is gone, and what is open leads nowhere).
  func isCurrent() -> Bool {
    guard isOpen, let device = Self.findDevice(uid: Self.deviceUID), device == deviceID else { return false }
    return Self.streams(of: device).contains(streamID)
  }

  func close() {
    if isOpen { CMIODeviceStopStream(deviceID, streamID) }
    queue = nil
    isOpen = false
  }

  /// Puts one picture in the sink stream's queue. `.full` when the extension has not taken the earlier ones yet (the
  /// picture is dropped), `.failed` when the stream is gone or the picture could not be made.
  func write(_ pixels: CVPixelBuffer) -> Written {
    guard isOpen, let queue else { return .failed }
    guard CMSimpleQueueGetCount(queue) < CMSimpleQueueGetCapacity(queue) else { return .full }
    if format == nil {
      CMVideoFormatDescriptionCreateForImageBuffer(allocator: nil, imageBuffer: pixels, formatDescriptionOut: &format)
    }
    guard let format else { return .failed }
    var timing = CMSampleTimingInfo(
      duration: CMTime(value: 1, timescale: 30), presentationTimeStamp: CMClockGetTime(CMClockGetHostTimeClock()),
      decodeTimeStamp: .invalid)
    var sample: CMSampleBuffer?
    guard
      CMSampleBufferCreateForImageBuffer(
        allocator: nil, imageBuffer: pixels, dataReady: true, makeDataReadyCallback: nil, refcon: nil,
        formatDescription: format, sampleTiming: &timing, sampleBufferOut: &sample) == noErr,
      let sample
    else { return .failed }
    // The queue keeps the sample: it gets the reference this retain makes.
    let status = CMSimpleQueueEnqueue(queue, element: Unmanaged.passRetained(sample).toOpaque())
    if status != noErr {
      Unmanaged.passUnretained(sample).release()
      log.error("CMSimpleQueueEnqueue failed: \(status)")
      return .failed
    }
    return .written
  }

  // MARK: Core Media I/O lookups

  private static func address(_ selector: Int) -> CMIOObjectPropertyAddress {
    CMIOObjectPropertyAddress(
      mSelector: CMIOObjectPropertySelector(selector), mScope: CMIOObjectPropertyScope(kCMIOObjectPropertyScopeGlobal),
      mElement: CMIOObjectPropertyElement(kCMIOObjectPropertyElementMain))
  }

  private static func ids<T>(of object: CMIOObjectID, selector: Int, as: T.Type) -> [T] {
    var address = address(selector)
    var size: UInt32 = 0
    guard CMIOObjectGetPropertyDataSize(object, &address, 0, nil, &size) == noErr, size > 0 else { return [] }
    let count = Int(size) / MemoryLayout<T>.size
    let buffer = UnsafeMutablePointer<T>.allocate(capacity: count)
    defer { buffer.deallocate() }
    var used: UInt32 = 0
    guard CMIOObjectGetPropertyData(object, &address, 0, nil, size, &used, buffer) == noErr else { return [] }
    return Array(UnsafeBufferPointer(start: buffer, count: Int(used) / MemoryLayout<T>.size))
  }

  private static func findDevice(uid: String) -> CMIODeviceID? {
    for device in ids(of: CMIOObjectID(kCMIOObjectSystemObject), selector: kCMIOHardwarePropertyDevices, as: CMIODeviceID.self) {
      var address = address(kCMIODevicePropertyDeviceUID)
      var value: Unmanaged<CFString>?
      var used: UInt32 = 0
      let size = UInt32(MemoryLayout<Unmanaged<CFString>?>.size)
      guard CMIOObjectGetPropertyData(device, &address, 0, nil, size, &used, &value) == noErr, let value else { continue }
      if value.takeRetainedValue() as String == uid { return device }
    }
    return nil
  }

  private static func direction(of stream: CMIOStreamID) -> UInt32? {
    var address = address(kCMIOStreamPropertyDirection)
    var value: UInt32 = 0
    var used: UInt32 = 0
    guard
      CMIOObjectGetPropertyData(stream, &address, 0, nil, UInt32(MemoryLayout<UInt32>.size), &used, &value) == noErr
    else { return nil }
    return value
  }

  private static func streams(of device: CMIODeviceID) -> [CMIOStreamID] {
    ids(of: device, selector: kCMIODevicePropertyStreams, as: CMIOStreamID.self)
  }
}
