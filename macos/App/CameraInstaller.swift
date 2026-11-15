import Foundation
import SwiftUI
import SystemExtensions
import os

/// Asks macOS to turn the camera extension inside this app on or off, and says how that went.
final class CameraInstaller: NSObject, ObservableObject, OSSystemExtensionRequestDelegate {
  static let extensionID = "com.rapp.phoneset.camera.extension"

  private let log = Logger(subsystem: "com.rapp.phoneset.camera", category: "installer")

  @Published var status = "Not installed yet." {
    didSet { log.info("installer: \(self.status, privacy: .public)") }
  }
  @Published var failed = false
  /// Called when a request has come to an end (done, or failed).
  var onFinished: ((_ succeeded: Bool) -> Void)?

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

  /// Removing needs the extension of this app to be the one that is installed (macOS finds it by version), so a
  /// request to install it comes first: it changes nothing when it is already the one.
  private var removeWhenInstalled = false

  func remove() {
    failed = false
    removeWhenInstalled = true
    status = "Asking macOS to remove the camera…"
    let request = OSSystemExtensionRequest.activationRequest(
      forExtensionWithIdentifier: Self.extensionID, queue: .main)
    request.delegate = self
    OSSystemExtensionManager.shared.submitRequest(request)
  }

  private func submitDeactivation() {
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
    if removeWhenInstalled {
      removeWhenInstalled = false
      status = "Removing the camera…"
      submitDeactivation()
      return
    }
    switch result {
    case .completed: status = "Done. “phoneset Camera” is now in the list of cameras."
    case .willCompleteAfterReboot: status = "Done after you restart the Mac."
    @unknown default: status = "Done."
    }
    onFinished?(true)
  }

  func request(_ request: OSSystemExtensionRequest, didFailWithError error: Error) {
    failed = true
    removeWhenInstalled = false
    let ns = error as NSError
    status = "macOS refused: \(ns.localizedDescription) (\(ns.domain) \(ns.code))"
    onFinished?(false)
  }
}
