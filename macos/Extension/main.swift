import CoreMediaIO
import Foundation

let providerSource = PhonesetProviderSource(clientQueue: nil)
CMIOExtensionProvider.startService(provider: providerSource.provider)

CFRunLoopRun()
