import CoreMediaIO
import Foundation
import os

/// The stream the companion app writes the phone's picture to. The extension reads from it (consumeSampleBuffer in
/// the device) and passes each picture on to the source stream.
final class PhonesetSinkSource: NSObject, CMIOExtensionStreamSource {
  private(set) var stream: CMIOExtensionStream!
  private let device: CMIOExtensionDevice
  private let streamFormat: CMIOExtensionStreamFormat
  private var client: CMIOExtensionClient?
  private let log = Logger(subsystem: "com.rapp.phoneset.camera.extension", category: "sink")

  init(localizedName: String, streamID: UUID, streamFormat: CMIOExtensionStreamFormat, device: CMIOExtensionDevice) {
    self.device = device
    self.streamFormat = streamFormat
    super.init()
    stream = CMIOExtensionStream(
      localizedName: localizedName, streamID: streamID, direction: .sink, clockType: .hostTime, source: self)
  }

  var formats: [CMIOExtensionStreamFormat] { [streamFormat] }

  var availableProperties: Set<CMIOExtensionProperty> {
    [
      .streamActiveFormatIndex, .streamFrameDuration, .streamSinkBufferQueueSize,
      .streamSinkBuffersRequiredForStartup, .streamSinkBufferUnderrunCount, .streamSinkEndOfData,
    ]
  }

  func streamProperties(forProperties properties: Set<CMIOExtensionProperty>) throws -> CMIOExtensionStreamProperties {
    let streamProperties = CMIOExtensionStreamProperties(dictionary: [:])
    if properties.contains(.streamActiveFormatIndex) { streamProperties.activeFormatIndex = 0 }
    if properties.contains(.streamFrameDuration) {
      streamProperties.frameDuration = CMTime(value: 1, timescale: Int32(PhonesetDeviceSource.frameRate))
    }
    if properties.contains(.streamSinkBufferQueueSize) { streamProperties.sinkBufferQueueSize = 1 }
    if properties.contains(.streamSinkBuffersRequiredForStartup) { streamProperties.sinkBuffersRequiredForStartup = 1 }
    return streamProperties
  }

  func setStreamProperties(_ streamProperties: CMIOExtensionStreamProperties) throws {}

  func authorizedToStartStream(for client: CMIOExtensionClient) -> Bool {
    log.info("sink: authorized to start for a client")
    self.client = client
    return true
  }

  func startStream() throws {
    log.info("sink: startStream, client \(self.client != nil, privacy: .public)")
    guard let source = device.source as? PhonesetDeviceSource, let client else { return }
    source.startStreamingSink(client: client)
  }

  func stopStream() throws {
    log.info("sink: stopStream")
    guard let source = device.source as? PhonesetDeviceSource else { return }
    source.stopStreamingSink()
  }
}
