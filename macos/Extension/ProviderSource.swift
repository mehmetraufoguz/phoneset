import CoreMediaIO
import Foundation

/// The extension as macOS sees it: one provider with one device, the camera "phoneset Camera".
final class PhonesetProviderSource: NSObject, CMIOExtensionProviderSource {
  private(set) var provider: CMIOExtensionProvider!
  private var deviceSource: PhonesetDeviceSource!

  init(clientQueue: DispatchQueue?) {
    super.init()
    provider = CMIOExtensionProvider(source: self, clientQueue: clientQueue)
    deviceSource = PhonesetDeviceSource(localizedName: "phoneset Camera")
    do {
      try provider.addDevice(deviceSource.device)
    } catch {
      fatalError("Failed to add the device: \(error.localizedDescription)")
    }
  }

  func connect(to client: CMIOExtensionClient) throws {}

  func disconnect(from client: CMIOExtensionClient) {}

  var availableProperties: Set<CMIOExtensionProperty> { [.providerManufacturer] }

  func providerProperties(forProperties properties: Set<CMIOExtensionProperty>) throws
    -> CMIOExtensionProviderProperties
  {
    let providerProperties = CMIOExtensionProviderProperties(dictionary: [:])
    if properties.contains(.providerManufacturer) { providerProperties.manufacturer = "phoneset" }
    return providerProperties
  }

  func setProviderProperties(_ providerProperties: CMIOExtensionProviderProperties) throws {}
}
