import CoreGraphics
import CoreMediaIO
import CoreText
import Foundation
import IOKit.audio

/// The camera device. Phase 0: it makes a test picture of its own (a moving bar and the name) so that it can be
/// seen that macOS has installed the extension and that programs can open the camera.
final class PhonesetDeviceSource: NSObject, CMIOExtensionDeviceSource {
  static let frameRate = 30
  static let width: Int32 = 1280
  static let height: Int32 = 720

  private(set) var device: CMIOExtensionDevice!
  private var streamSource: PhonesetStreamSource!
  private var streamingCounter = 0
  private let lock = NSLock()
  private var timer: DispatchSourceTimer?
  private let timerQueue = DispatchQueue(label: "phoneset.camera.timer", qos: .userInteractive)
  private var videoDescription: CMFormatDescription!
  private var bufferPool: CVPixelBufferPool!
  private let bufferAuxAttributes: NSDictionary = [kCVPixelBufferPoolAllocationThresholdKey: 5]
  private var frameNumber = 0

  // Fixed identifiers, so that programs keep remembering the camera between runs.
  private static let deviceID = UUID(uuidString: "5B6C0E2A-8A3F-4D3B-9E0A-7C1D2F4A6B11")!
  private static let streamID = UUID(uuidString: "C0A4F1D6-3E52-4B8E-A6A1-2D9B7F0E8C34")!

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
    streamSource = PhonesetStreamSource(
      localizedName: "phoneset Camera.Video", streamID: Self.streamID, streamFormat: format, device: device)
    do {
      try device.addStream(streamSource.stream)
    } catch {
      fatalError("Failed to add the stream: \(error.localizedDescription)")
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

  // MARK: streaming

  func startStreaming() {
    lock.lock()
    defer { lock.unlock() }
    streamingCounter += 1
    guard timer == nil else { return }
    let timer = DispatchSource.makeTimerSource(flags: .strict, queue: timerQueue)
    timer.schedule(deadline: .now(), repeating: 1.0 / Double(Self.frameRate), leeway: .seconds(0))
    timer.setEventHandler { [weak self] in self?.makeFrame() }
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

  private func makeFrame() {
    var pixels: CVPixelBuffer?
    guard
      CVPixelBufferPoolCreatePixelBufferWithAuxAttributes(
        kCFAllocatorDefault, bufferPool, bufferAuxAttributes, &pixels) == kCVReturnSuccess,
      let pixels
    else { return }

    CVPixelBufferLockBaseAddress(pixels, [])
    drawTestPicture(into: pixels)
    CVPixelBufferUnlockBaseAddress(pixels, [])
    frameNumber += 1

    let now = CMClockGetTime(CMClockGetHostTimeClock())
    var timing = CMSampleTimingInfo(
      duration: CMTime(value: 1, timescale: Int32(Self.frameRate)), presentationTimeStamp: now,
      decodeTimeStamp: .invalid)
    var sample: CMSampleBuffer?
    guard
      CMSampleBufferCreateForImageBuffer(
        allocator: kCFAllocatorDefault, imageBuffer: pixels, dataReady: true, makeDataReadyCallback: nil, refcon: nil,
        formatDescription: videoDescription, sampleTiming: &timing, sampleBufferOut: &sample) == noErr,
      let sample
    else { return }
    streamSource.stream.send(
      sample, discontinuity: [], hostTimeInNanoseconds: UInt64(now.seconds * Double(NSEC_PER_SEC)))
  }

  // MARK: the test picture

  private func drawTestPicture(into pixels: CVPixelBuffer) {
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

    // A bar that runs across, so that a frozen picture is easy to tell from a live one.
    let barX = CGFloat((frameNumber * 10) % Int(width))
    context.setFillColor(CGColor(red: 0.23, green: 0.42, blue: 1, alpha: 1))
    context.fill(CGRect(x: barX, y: 0, width: 40, height: height))

    drawText("phoneset Camera", size: 84, y: height * 0.55, width: width, in: context)
    drawText("test picture, frame \(frameNumber)", size: 32, y: height * 0.40, width: width, in: context)
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
