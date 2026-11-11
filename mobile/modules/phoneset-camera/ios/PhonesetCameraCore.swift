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

  private static let codecH264: UInt8 = 2

  // Callbacks waiting for a picture, oldest first: JavaScript may ask for the next frame before it has the last
  // (more than one picture in the coder at once hides the coder's own delay).
  private var waiters: [Callback] = []
  private var minInterval: CFAbsoluteTime = 1.0 / 30

  // h264: a picture is handed to the coder (on the capture queue) and comes back on another thread, so
  // the callback that is waiting for it is kept here until then. There is one at most: JavaScript asks
  // for the next frame only when it has the last.
  private var codec = "jpeg"
  private var askedFps = 30
  private var encoder: PhonesetH264Encoder?
  private var inFlight: [(callback: Callback, number: UInt32, captured: UInt32, started: CFAbsoluteTime)] = []
  private var needKey = true
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
  func start(facing: String, width: Int, height: Int, fps: Int, quality: Int, codec: String) throws {
    stop()
    guard codec == "jpeg" || codec == "h264" else {
      throw CameraFailure(message: "This phone can't send \(codec) video.")
    }
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

    // The camera format that has exactly this size and can run at this rate. Without one, the preset
    // chosen above stays (640 x 480 or 1280 x 720), which is all this module could do before.
    if let format = Self.format(of: device, width: width, height: height, fps: fps) {
      do {
        try device.lockForConfiguration()
        device.activeFormat = format  // makes the session's preset "input priority"
        let frame = CMTime(value: 1, timescale: CMTimeScale(fps))
        device.activeVideoMinFrameDuration = frame
        device.activeVideoMaxFrameDuration = frame
        device.unlockForConfiguration()
      } catch {
        // Keep the preset's own format: a picture of the wrong size is better than none.
      }
    }

    let output = AVCaptureVideoDataOutput()
    // Pictures that arrive while the last is still being worked on are dropped by the system.
    output.alwaysDiscardsLateVideoFrames = true
    // JPEG is made from RGB; the coder takes the camera's own YUV pictures, with no conversion.
    output.videoSettings = [
      kCVPixelBufferPixelFormatTypeKey as String: codec == "h264"
        ? kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange : kCVPixelFormatType_32BGRA
    ]
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
    self.codec = codec
    self.askedFps = min(60, max(1, fps))
    self.needKey = true
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
    let pending = waiters
    waiters = []
    let coded = inFlight
    inFlight = []
    let coder = encoder
    encoder = nil
    lock.unlock()
    coder?.invalidate()
    pending.forEach { $0(nil, "The camera was stopped.") }
    coded.forEach { $0.callback(nil, "The camera was stopped.") }
  }

  /// Waits for the next frame. A call replaces one that is still waiting.
  func next(_ callback: @escaping Callback) {
    lock.lock()
    waiters.append(callback)
    let replaced: Callback? = waiters.count > 4 ? waiters.removeFirst() : nil
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
    guard !waiters.isEmpty, now - lastEncode >= minInterval * 0.85 else {
      lock.unlock()
      return
    }
    let callback = waiters.removeFirst()
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
    // The presentation time is on the phone's steady clock.
    let captured = UInt32(truncatingIfNeeded: Int(CMSampleBufferGetPresentationTimeStamp(sampleBuffer).seconds * 1000))
    if codec == "h264" {
      code(
        pixels, at: CMSampleBufferGetPresentationTimeStamp(sampleBuffer), callback: callback, number: number,
        captured: captured, width: w, height: h, started: now)
      return
    }
    guard
      let jpeg = ciContext.jpegRepresentation(
        of: CIImage(cvPixelBuffer: pixels), colorSpace: CGColorSpace(name: CGColorSpace.sRGB)!,
        options: [kCGImageDestinationLossyCompressionQuality as CIImageRepresentationOption: quality])
    else {
      callback(nil, "Couldn't make a JPEG from the camera's picture.")
      return
    }

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

  /// h264: hands one picture to the coder; `deliver` makes the frame when the coder has it.
  private func code(
    _ pixels: CVPixelBuffer, at time: CMTime, callback: @escaping Callback, number: UInt32, captured: UInt32,
    width w: Int, height h: Int, started: CFAbsoluteTime
  ) {
    lock.lock()
    var coder = encoder
    let forceKey = needKey
    needKey = false
    inFlight.append((callback, number, captured, started))
    width = w
    height = h
    lock.unlock()

    if coder == nil {
      // About 0.1 bit per pixel: 6 Mbit/s at 1080p30, 25 at 4K30, which the link and the phone carry.
      let bitrate = max(1_500_000, min(60_000_000, Int(Double(w * h) * Double(askedFps) * 0.1)))
      coder = PhonesetH264Encoder(width: w, height: h, fps: askedFps, bitrate: bitrate) { [weak self] unit, failure in
        self?.deliver(unit, failure)
      }
      lock.lock()
      encoder = coder
      lock.unlock()
    }
    guard let coder else {
      fail("This phone has no hardware H.264 coder for \(w) x \(h).")
      return
    }
    if !coder.encode(pixels, at: time, forceKey: forceKey) {
      fail("Couldn't hand the picture to the video coder.")
    }
  }

  /// Ends the picture that was just handed over (and was refused) with a message; the next one is a key frame.
  private func fail(_ message: String) {
    lock.lock()
    let failed = inFlight.popLast()
    needKey = true
    lock.unlock()
    failed?.callback(nil, message)
  }

  /// h264: the coder has (or has dropped) the picture that was handed to it.
  private func deliver(_ unit: PhonesetH264Encoder.Unit?, _ failure: String?) {
    lock.lock()
    let pending = inFlight.isEmpty ? nil : inFlight.removeFirst()  // the coder answers in the order it was given pictures
    if unit == nil { needKey = true }  // dropped or failed: the next picture starts a new sequence
    let w = width
    let h = height
    lock.unlock()
    guard let pending else { return }
    if let failure {
      pending.callback(nil, failure)
      return
    }
    guard let unit else {
      // The coder dropped it: wait for the next picture with the same request.
      lock.lock()
      waiters.insert(pending.callback, at: 0)
      lock.unlock()
      return
    }
    var frame = Data(capacity: Self.headerLength + unit.data.count)
    frame.append(contentsOf: Self.bigEndian(pending.number))
    frame.append(contentsOf: Self.bigEndian(pending.captured))
    frame.append(Self.codecH264)
    frame.append(unit.key ? Self.flagKey : 0)
    frame.append(contentsOf: Self.bigEndian(UInt16(truncatingIfNeeded: w)))
    frame.append(contentsOf: Self.bigEndian(UInt16(truncatingIfNeeded: h)))
    frame.append(unit.data)

    lock.lock()
    encoded += 1
    encodeSeconds += CFAbsoluteTimeGetCurrent() - pending.started  // from the picture to the coded unit
    jpegBytes += unit.data.count  // the size of what was coded, whichever codec
    lock.unlock()
    pending.callback(frame, nil)
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

  /// A format of this camera with exactly this size (as the camera lists it, before the picture is turned
  /// upright) that can deliver this rate; the video-range 4:2:0 ones the camera app itself uses come first.
  private static func format(of device: AVCaptureDevice, width: Int, height: Int, fps: Int)
    -> AVCaptureDevice.Format?
  {
    let matching = device.formats.filter { format in
      let size = CMVideoFormatDescriptionGetDimensions(format.formatDescription)
      return Int(size.width) == width && Int(size.height) == height
        && format.videoSupportedFrameRateRanges.contains {
          $0.minFrameRate <= Double(fps) && Double(fps) <= $0.maxFrameRate
        }
    }
    return matching.first {
      CMFormatDescriptionGetMediaSubType($0.formatDescription)
        == kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange
    } ?? matching.first
  }

  private static func bigEndian<T: FixedWidthInteger>(_ value: T) -> [UInt8] {
    withUnsafeBytes(of: value.bigEndian) { Array($0) }
  }
}
