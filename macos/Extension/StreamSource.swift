import CoreMediaIO
import Foundation

/// The stream other programs read from the camera.
final class PhonesetStreamSource: NSObject, CMIOExtensionStreamSource {
  private(set) var stream: CMIOExtensionStream!
  private let device: CMIOExtensionDevice
  private let streamFormat: CMIOExtensionStreamFormat

  init(localizedName: String, streamID: UUID, streamFormat: CMIOExtensionStreamFormat, device: CMIOExtensionDevice) {
    self.device = device
    self.streamFormat = streamFormat
    super.init()
    stream = CMIOExtensionStream(
      localizedName: localizedName, streamID: streamID, direction: .source, clockType: .hostTime, source: self)
  }

  var formats: [CMIOExtensionStreamFormat] { [streamFormat] }

  var activeFormatIndex: Int = 0

  var availableProperties: Set<CMIOExtensionProperty> { [.streamActiveFormatIndex, .streamFrameDuration] }

  func streamProperties(forProperties properties: Set<CMIOExtensionProperty>) throws -> CMIOExtensionStreamProperties {
    let streamProperties = CMIOExtensionStreamProperties(dictionary: [:])
    if properties.contains(.streamActiveFormatIndex) { streamProperties.activeFormatIndex = 0 }
    if properties.contains(.streamFrameDuration) {
      streamProperties.frameDuration = CMTime(value: 1, timescale: Int32(PhonesetDeviceSource.frameRate))
    }
    return streamProperties
  }

  func setStreamProperties(_ streamProperties: CMIOExtensionStreamProperties) throws {
    if let index = streamProperties.activeFormatIndex { activeFormatIndex = index }
  }

  func authorizedToStartStream(for client: CMIOExtensionClient) -> Bool { true }

  func startStream() throws {
    guard let source = device.source as? PhonesetDeviceSource else { fatalError("The device has the wrong source.") }
    source.startStreaming()
  }

  func stopStream() throws {
    guard let source = device.source as? PhonesetDeviceSource else { fatalError("The device has the wrong source.") }
    source.stopStreaming()
  }
}
