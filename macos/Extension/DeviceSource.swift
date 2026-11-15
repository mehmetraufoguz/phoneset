import CoreGraphics
import CoreMediaIO
import CoreText
import Foundation
import IOKit.audio
import os

/// The camera device. It has two streams: the source stream, which is what programs read, and a sink stream, where the
/// companion app writes the phone's picture. Pictures that come in at the sink go straight to the source. When
/// nothing has come in for a moment (no app, no phone) the device shows a slate of its own, so that a program that has
/// opened the camera always gets a live picture.
final class PhonesetDeviceSource: NSObject, CMIOExtensionDeviceSource {
  static let frameRate = 30
  static let width: Int32 = 1280
  static let height: Int32 = 720
  /// How long a picture from the app counts as "now": after that the slate is shown.
  private static let sinkFresh: TimeInterval = 0.5

  private(set) var device: CMIOExtensionDevice!
  private var sourceStream: PhonesetStreamSource!
  private var sinkStream: PhonesetSinkSource!

  private let lock = NSLock()
  private var streamingCounter = 0
  private var sinkStarted = false
  private var lastSinkFrame: Date = .distantPast
  private var timer: DispatchSourceTimer?
  private let timerQueue = DispatchQueue(label: "phoneset.camera.timer", qos: .userInteractive)

  private var videoDescription: CMFormatDescription!
  private var bufferPool: CVPixelBufferPool!
  private let bufferAuxAttributes: NSDictionary = [kCVPixelBufferPoolAllocationThresholdKey: 5]
  private var frameNumber = 0
  private let log = Logger(subsystem: "com.rapp.phoneset.camera.extension", category: "device")
  private var consumed = 0

  // Fixed identifiers, so that programs keep remembering the camera between runs. The app finds the camera by
  // the device one (its "unique ID"), so it is also in macos/App/SinkWriter.swift.
  private static let deviceID = UUID(uuidString: "5B6C0E2A-8A3F-4D3B-9E0A-7C1D2F4A6B11")!
  private static let sourceID = UUID(uuidString: "C0A4F1D6-3E52-4B8E-A6A1-2D9B7F0E8C34")!
  private static let sinkID = UUID(uuidString: "7E21B9A5-6C48-4F0D-8B13-A5D07C92E4F8")!

  init(localizedName: String) {
    super.init()
    device = CMIOExtensionDevice(
      localizedName: localizedName, deviceID: Self.deviceID, legacyDeviceID: nil, source: self)

    var description: CMFormatDescription?
    CMVideoFormatDescriptionCreate(
      allocator: kCFAllocatorDefault, codecType: kCVPixelFormatType_32BGRA, width: Self.width, height: Self.height,
      extensions: nil, formatDescriptionOut: &description)
    videoDescription = description

    let attributes: NSDictionary = [
      kCVPixelBufferWidthKey: Self.width,
      kCVPixelBufferHeightKey: Self.height,
      kCVPixelBufferPixelFormatTypeKey: kCVPixelFormatType_32BGRA,
      kCVPixelBufferIOSurfacePropertiesKey: [:] as NSDictionary,
    ]
    CVPixelBufferPoolCreate(kCFAllocatorDefault, nil, attributes, &bufferPool)

    let frameDuration = CMTime(value: 1, timescale: Int32(Self.frameRate))
    let format = CMIOExtensionStreamFormat(
      formatDescription: videoDescription, maxFrameDuration: frameDuration, minFrameDuration: frameDuration,
      validFrameDurations: nil)
    sourceStream = PhonesetStreamSource(
      localizedName: "phoneset Camera.Video", streamID: Self.sourceID, streamFormat: format, device: device)
    sinkStream = PhonesetSinkSource(
      localizedName: "phoneset Camera.Video.Sink", streamID: Self.sinkID, streamFormat: format, device: device)
    do {
      // The order matters: the app takes the second stream of the device for the sink.
      try device.addStream(sourceStream.stream)
      try device.addStream(sinkStream.stream)
    } catch {
      fatalError("Failed to add the streams: \(error.localizedDescription)")
    }
  }

  var availableProperties: Set<CMIOExtensionProperty> { [.deviceTransportType, .deviceModel] }

  func deviceProperties(forProperties properties: Set<CMIOExtensionProperty>) throws -> CMIOExtensionDeviceProperties {
    let deviceProperties = CMIOExtensionDeviceProperties(dictionary: [:])
    if properties.contains(.deviceTransportType) { deviceProperties.transportType = kIOAudioDeviceTransportTypeVirtual }
    if properties.contains(.deviceModel) { deviceProperties.model = "phoneset Camera" }
    return deviceProperties
  }

  func setDeviceProperties(_ deviceProperties: CMIOExtensionDeviceProperties) throws {}

  // MARK: the source: programs reading the camera

  func startStreaming() {
    lock.lock()
    defer { lock.unlock() }
    streamingCounter += 1
    guard timer == nil else { return }
    let timer = DispatchSource.makeTimerSource(flags: .strict, queue: timerQueue)
    timer.schedule(deadline: .now(), repeating: 1.0 / Double(Self.frameRate), leeway: .seconds(0))
    timer.setEventHandler { [weak self] in self?.tick() }
    timer.resume()
    self.timer = timer
  }

  func stopStreaming() {
    lock.lock()
    defer { lock.unlock() }
    if streamingCounter > 0 { streamingCounter -= 1 }
    if streamingCounter == 0 {
      timer?.cancel()
      timer = nil
    }
  }

  private var isReading: Bool {
    lock.lock()
    defer { lock.unlock() }
    return streamingCounter > 0
  }

  /// Every frame time: the slate, unless the app has just sent a picture.
  private func tick() {
    lock.lock()
    let fresh = sinkStarted && Date().timeIntervalSince(lastSinkFrame) < Self.sinkFresh
    lock.unlock()
    if fresh { return }

    var pixels: CVPixelBuffer?
    guard
      CVPixelBufferPoolCreatePixelBufferWithAuxAttributes(
        kCFAllocatorDefault, bufferPool, bufferAuxAttributes, &pixels) == kCVReturnSuccess,
      let pixels
    else { return }
    CVPixelBufferLockBaseAddress(pixels, [])
    drawSlate(into: pixels)
    CVPixelBufferUnlockBaseAddress(pixels, [])
    frameNumber += 1
    send(pixels, at: CMClockGetTime(CMClockGetHostTimeClock()))
  }

  private func send(_ pixels: CVPixelBuffer, at time: CMTime) {
    var timing = CMSampleTimingInfo(
      duration: CMTime(value: 1, timescale: Int32(Self.frameRate)), presentationTimeStamp: time,
      decodeTimeStamp: .invalid)
    var sample: CMSampleBuffer?
    guard
      CMSampleBufferCreateForImageBuffer(
        allocator: kCFAllocatorDefault, imageBuffer: pixels, dataReady: true, makeDataReadyCallback: nil, refcon: nil,
        formatDescription: videoDescription, sampleTiming: &timing, sampleBufferOut: &sample) == noErr,
      let sample
    else { return }
    sourceStream.stream.send(sample, discontinuity: [], hostTimeInNanoseconds: UInt64(time.seconds * Double(NSEC_PER_SEC)))
  }

  // MARK: the sink: the companion app writing the phone's picture

  func startStreamingSink(client: CMIOExtensionClient) {
    log.info("sink streaming starts")
    lock.lock()
    sinkStarted = true
    lock.unlock()
    consumeBuffer(client)
  }

  func stopStreamingSink() {
    lock.lock()
    sinkStarted = false
    lock.unlock()
  }

  /// Takes the next picture the app has written and passes it on to the source; asks for the next one when done.
  private func consumeBuffer(_ client: CMIOExtensionClient) {
    lock.lock()
    let started = sinkStarted
    lock.unlock()
    if !started { return }

    sinkStream.stream.consumeSampleBuffer(from: client) { [weak self] sample, sequence, _, _, _ in
      guard let self else { return }
      if let sample {
        self.consumed += 1
        if self.consumed % 30 == 1 { self.log.info("sink frames taken: \(self.consumed, privacy: .public)") }
        let now = CMClockGetTime(CMClockGetHostTimeClock())
        self.lock.lock()
        self.lastSinkFrame = Date()
        self.lock.unlock()
        if self.isReading {
          self.sourceStream.stream.send(
            sample, discontinuity: [],
            hostTimeInNanoseconds: UInt64(sample.presentationTimeStamp.seconds * Double(NSEC_PER_SEC)))
        }
        let output = CMIOExtensionScheduledOutput(
          sequenceNumber: sequence, hostTimeInNanoseconds: UInt64(now.seconds * Double(NSEC_PER_SEC)))
        self.sinkStream.stream.notifyScheduledOutputChanged(output)
      }
      self.consumeBuffer(client)
    }
  }

  // MARK: the slate

  private func drawSlate(into pixels: CVPixelBuffer) {
    guard
      let context = CGContext(
        data: CVPixelBufferGetBaseAddress(pixels), width: CVPixelBufferGetWidth(pixels),
        height: CVPixelBufferGetHeight(pixels), bitsPerComponent: 8, bytesPerRow: CVPixelBufferGetBytesPerRow(pixels),
        space: CGColorSpace(name: CGColorSpace.sRGB) ?? CGColorSpaceCreateDeviceRGB(),
        bitmapInfo: CGImageAlphaInfo.premultipliedFirst.rawValue | CGBitmapInfo.byteOrder32Little.rawValue)
    else { return }
    let width = CGFloat(CVPixelBufferGetWidth(pixels))
    let height = CGFloat(CVPixelBufferGetHeight(pixels))

    context.setFillColor(CGColor(red: 0.07, green: 0.08, blue: 0.11, alpha: 1))
    context.fill(CGRect(x: 0, y: 0, width: width, height: height))

    // A thin bar at the bottom that runs across, so that a frozen picture is easy to tell from a live one.
    let barX = CGFloat((frameNumber * 12) % Int(width))
    context.setFillColor(CGColor(red: 0.23, green: 0.42, blue: 1, alpha: 1))
    context.fill(CGRect(x: barX, y: 0, width: 80, height: 8))

    drawText("phoneset Camera", size: 84, y: height * 0.55, width: width, in: context)
    drawText("Waiting for your phone…", size: 36, y: height * 0.42, width: width, in: context)
  }

  private func drawText(_ text: String, size: CGFloat, y: CGFloat, width: CGFloat, in context: CGContext) {
    let font = CTFontCreateWithName("Helvetica-Bold" as CFString, size, nil)
    let attributes: [NSAttributedString.Key: Any] = [
      NSAttributedString.Key(kCTFontAttributeName as String): font,
      NSAttributedString.Key(kCTForegroundColorAttributeName as String): CGColor(red: 1, green: 1, blue: 1, alpha: 1),
    ]
    let line = CTLineCreateWithAttributedString(NSAttributedString(string: text, attributes: attributes))
    let bounds = CTLineGetBoundsWithOptions(line, [])
    context.textPosition = CGPoint(x: (width - bounds.width) / 2, y: y)
    CTLineDraw(line, context)
  }
}
