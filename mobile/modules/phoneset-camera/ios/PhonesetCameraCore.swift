import AVFoundation
import CoreImage

struct CameraFailure: Error {
  let message: String
}

/// The phone's camera as a source of JPEG frames, with no Expo types in it. The same design as the
/// Android FrameSource: a picture is only turned into a JPEG when something is waiting for one and
/// the frame rate allows it, so the work follows what is actually sent.
///
/// A frame is the message body of docs/protocol.md type 0x04: seq (u32), capture time in ms (u32),
/// codec (u8), flags (u8), width (u16), height (u16), then the JPEG. The picture is upright and not
/// mirrored.
final class PhonesetCameraCore: NSObject, AVCaptureVideoDataOutputSampleBufferDelegate {
  typealias Callback = (Data?, String?) -> Void

  private static let headerLength = 14
  private static let codecJPEG: UInt8 = 1
  private static let flagKey: UInt8 = 1

  private let session = AVCaptureSession()
  private let queue = DispatchQueue(label: "phoneset.camera")
  private let ciContext = CIContext()
  private let lock = NSLock()

  private var waiter: Callback?
  private var minInterval: CFAbsoluteTime = 1.0 / 30
  private var quality: CGFloat = 0.7
  private var lastEncode: CFAbsoluteTime = 0
  private var seq: UInt32 = 0

  // Counters for stats(); touched on the capture queue and read from anywhere, under the lock.
  private var startedAt: CFAbsoluteTime = 0
  private var analysed = 0
  private var encoded = 0
  private var encodeSeconds = 0.0
  private var jpegBytes = 0
  private var width = 0
  private var height = 0

  /// Opens the camera. Blocks until it runs, so call it off the main thread.
  func start(facing: String, width: Int, height: Int, fps: Int, quality: Int) throws {
    stop()
    let position: AVCaptureDevice.Position = facing == "front" ? .front : .back
    guard let device = AVCaptureDevice.default(.builtInWideAngleCamera, for: .video, position: position),
      let input = try? AVCaptureDeviceInput(device: device)
    else {
      throw CameraFailure(message: "This phone has no \(facing) camera.")
    }

    session.beginConfiguration()
    session.sessionPreset = width >= 1280 ? .hd1280x720 : .vga640x480
    guard session.canAddInput(input) else {
      session.commitConfiguration()
      throw CameraFailure(message: "The camera can't be used right now.")
    }
    session.addInput(input)

    let output = AVCaptureVideoDataOutput()
    // Pictures that arrive while the last is still being worked on are dropped by the system.
    output.alwaysDiscardsLateVideoFrames = true
    output.videoSettings = [kCVPixelBufferPixelFormatTypeKey as String: kCVPixelFormatType_32BGRA]
    output.setSampleBufferDelegate(self, queue: queue)
    guard session.canAddOutput(output) else {
      session.commitConfiguration()
      throw CameraFailure(message: "The camera can't be used right now.")
    }
    session.addOutput(output)
    if let connection = output.connection(with: .video) {
      // Upright for a phone held in portrait, and not mirrored, whichever camera it is.
      if #available(iOS 17.0, *) {
        if connection.isVideoRotationAngleSupported(90) { connection.videoRotationAngle = 90 }
      } else if connection.isVideoOrientationSupported {
        connection.videoOrientation = .portrait
      }
      if connection.isVideoMirroringSupported {
        connection.automaticallyAdjustsVideoMirroring = false
        connection.isVideoMirrored = false
      }
    }
    session.commitConfiguration()

    lock.lock()
    self.quality = CGFloat(min(95, max(30, quality))) / 100
    self.minInterval = 1.0 / Double(min(60, max(1, fps)))
    self.seq = 0
    self.lastEncode = 0
    startedAt = CFAbsoluteTimeGetCurrent()
    analysed = 0
    encoded = 0
    encodeSeconds = 0
    jpegBytes = 0
    lock.unlock()

    session.startRunning()
  }

  /// Stops the camera and fails whoever was waiting for a frame.
  func stop() {
    if session.isRunning { session.stopRunning() }
    session.beginConfiguration()
    session.inputs.forEach { session.removeInput($0) }
    session.outputs.forEach { session.removeOutput($0) }
    session.commitConfiguration()
    lock.lock()
    let pending = waiter
    waiter = nil
    lock.unlock()
    pending?(nil, "The camera was stopped.")
  }

  /// Waits for the next frame. A call replaces one that is still waiting.
  func next(_ callback: @escaping Callback) {
    lock.lock()
    let replaced = waiter
    waiter = callback
    lock.unlock()
    replaced?(nil, "Another frame was asked for.")
  }

  func captureOutput(
    _ output: AVCaptureOutput, didOutput sampleBuffer: CMSampleBuffer, from connection: AVCaptureConnection
  ) {
    let now = CFAbsoluteTimeGetCurrent()
    lock.lock()
    analysed += 1
    // Nobody is ready for a picture, or it is faster than asked for (a little early still counts, since the
    // camera's own rate is never exactly even): it is dropped, with no work done.
    guard let callback = waiter, now - lastEncode >= minInterval * 0.85 else {
      lock.unlock()
      return
    }
    waiter = nil
    lastEncode = now
    let quality = self.quality
    let number = seq
    seq &+= 1
    lock.unlock()

    guard let pixels = CMSampleBufferGetImageBuffer(sampleBuffer) else {
      callback(nil, "The camera gave no picture.")
      return
    }
    let w = CVPixelBufferGetWidth(pixels)
    let h = CVPixelBufferGetHeight(pixels)
    guard
      let jpeg = ciContext.jpegRepresentation(
        of: CIImage(cvPixelBuffer: pixels), colorSpace: CGColorSpace(name: CGColorSpace.sRGB)!,
        options: [kCGImageDestinationLossyCompressionQuality as CIImageRepresentationOption: quality])
    else {
      callback(nil, "Couldn't make a JPEG from the camera's picture.")
      return
    }

    // The presentation time is on the phone's steady clock.
    let captured = UInt32(truncatingIfNeeded: Int(CMSampleBufferGetPresentationTimeStamp(sampleBuffer).seconds * 1000))
    var frame = Data(capacity: Self.headerLength + jpeg.count)
    frame.append(contentsOf: Self.bigEndian(number))
    frame.append(contentsOf: Self.bigEndian(captured))
    frame.append(Self.codecJPEG)
    frame.append(Self.flagKey)  // every JPEG can be decoded on its own
    frame.append(contentsOf: Self.bigEndian(UInt16(truncatingIfNeeded: w)))
    frame.append(contentsOf: Self.bigEndian(UInt16(truncatingIfNeeded: h)))
    frame.append(jpeg)

    lock.lock()
    encoded += 1
    encodeSeconds += CFAbsoluteTimeGetCurrent() - now
    jpegBytes += jpeg.count
    width = w
    height = h
    lock.unlock()
    callback(frame, nil)
  }

  /// What the camera side has done since it started, for measuring.
  func stats() -> [String: Double] {
    lock.lock()
    defer { lock.unlock() }
    let seconds = max(1, CFAbsoluteTimeGetCurrent() - startedAt)
    return [
      "width": Double(width),
      "height": Double(height),
      "analysedFps": Double(analysed) / seconds,
      "encodedFps": Double(encoded) / seconds,
      "encodeMs": encoded == 0 ? 0 : encodeSeconds * 1000 / Double(encoded),
      "jpegKB": encoded == 0 ? 0 : Double(jpegBytes) / 1024 / Double(encoded),
      "captureLagMs": -1,
      "encoded": Double(encoded),
    ]
  }

  /// The sizes the camera reports it can deliver, each with the highest frame rate it lists for it.
  static func capabilities(facing: String) -> [String: Any] {
    let position: AVCaptureDevice.Position = facing == "front" ? .front : .back
    guard let device = AVCaptureDevice.default(.builtInWideAngleCamera, for: .video, position: position) else {
      return ["sizes": [[String: Int]](), "id": ""]
    }
    var best: [String: Int] = [:]  // "WxH" -> max fps
    var sizes: [String: (Int, Int)] = [:]
    for format in device.formats {
      let dimensions = CMVideoFormatDescriptionGetDimensions(format.formatDescription)
      let key = "\(dimensions.width)x\(dimensions.height)"
      let fps = Int(format.videoSupportedFrameRateRanges.map { $0.maxFrameRate }.max() ?? 0)
      best[key] = max(best[key] ?? 0, fps)
      sizes[key] = (Int(dimensions.width), Int(dimensions.height))
    }
    let list: [[String: Int]] = sizes.map { key, size in
      ["width": size.0, "height": size.1, "maxFps": best[key] ?? 0]
    }
    return ["id": device.uniqueID, "sizes": list]
  }

  private static func bigEndian<T: FixedWidthInteger>(_ value: T) -> [UInt8] {
    withUnsafeBytes(of: value.bigEndian) { Array($0) }
  }
}
