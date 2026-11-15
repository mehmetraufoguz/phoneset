import Foundation
import SwiftUI
import SystemExtensions

/// Asks macOS to turn the camera extension inside this app on or off, and says how that went.
final class CameraInstaller: NSObject, ObservableObject, OSSystemExtensionRequestDelegate {
  static let extensionID = "com.rapp.phoneset.camera.extension"

  @Published var status = "Not installed yet."
  @Published var failed = false

  override init() {
    super.init()
    if !Bundle.main.bundlePath.hasPrefix("/Applications/") {
      status = "Move phoneset Camera to the Applications folder first: macOS only installs a camera from there."
      failed = true
    }
  }

  func install() {
    failed = false
    status = "Asking macOS to install the camera…"
    let request = OSSystemExtensionRequest.activationRequest(
      forExtensionWithIdentifier: Self.extensionID, queue: .main)
    request.delegate = self
    OSSystemExtensionManager.shared.submitRequest(request)
  }

  func remove() {
    failed = false
    status = "Asking macOS to remove the camera…"
    let request = OSSystemExtensionRequest.deactivationRequest(
      forExtensionWithIdentifier: Self.extensionID, queue: .main)
    request.delegate = self
    OSSystemExtensionManager.shared.submitRequest(request)
  }

  // MARK: OSSystemExtensionRequestDelegate

  func request(
    _ request: OSSystemExtensionRequest, actionForReplacingExtension existing: OSSystemExtensionProperties,
    withExtension ext: OSSystemExtensionProperties
  ) -> OSSystemExtensionRequest.ReplacementAction {
    status = "Replacing version \(existing.bundleShortVersion) with \(ext.bundleShortVersion)…"
    return .replace
  }

  func requestNeedsUserApproval(_ request: OSSystemExtensionRequest) {
    status = "Waiting for you: allow “phoneset Camera” in System Settings, under Privacy & Security."
  }

  func request(_ request: OSSystemExtensionRequest, didFinishWithResult result: OSSystemExtensionRequest.Result) {
    failed = false
    switch result {
    case .completed: status = "Done. “phoneset Camera” is now in the list of cameras."
    case .willCompleteAfterReboot: status = "Done after you restart the Mac."
    @unknown default: status = "Done."
    }
  }

  func request(_ request: OSSystemExtensionRequest, didFailWithError error: Error) {
    failed = true
    let ns = error as NSError
    status = "macOS refused: \(ns.localizedDescription) (\(ns.domain) \(ns.code))"
  }
}
