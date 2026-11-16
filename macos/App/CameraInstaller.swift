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

  /// What the request in flight is for, so that its answers (done, waiting for the user, failed) can be told apart.
  private enum Goal {
    case install
    /// Remove the extension that is installed.
    case remove
    /// Remove it, but the installed one is another version than this app's: make it this one first.
    case replaceThenRemove
  }
  private var goal = Goal.install

  func install() {
    failed = false
    goal = .install
    status = "Asking macOS to install the camera…"
    let request = OSSystemExtensionRequest.activationRequest(
      forExtensionWithIdentifier: Self.extensionID, queue: .main)
    request.delegate = self
    OSSystemExtensionManager.shared.submitRequest(request)
  }

  /// Removing asks macOS to remove the extension straight away. macOS finds the extension by its version, so when the
  /// one that is installed is another version than this app's the request fails with "extension not found"; only
  /// then is the installed one made this app's first (an install request, which can wait for the user), and removed.
  /// (It used to start with the install request every time, and a removal then waited on an install the person had
  /// not asked for.)
  func remove() {
    failed = false
    goal = .remove
    status = "Asking macOS to remove the camera…"
    submitDeactivation()
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
    switch goal {
    case .remove:
      status = "Waiting for you: macOS asks you to confirm removing “phoneset Camera” (a password or Touch ID prompt, "
        + "or System Settings, General, Login Items & Extensions, Camera Extensions)."
    case .replaceThenRemove:
      status = "Waiting for you: macOS asks you to allow the version of “phoneset Camera” that is being removed."
    case .install:
      status = "Waiting for you: allow “phoneset Camera” in System Settings, under Privacy & Security."
    }
  }

  func request(_ request: OSSystemExtensionRequest, didFinishWithResult result: OSSystemExtensionRequest.Result) {
    failed = false
    switch goal {
    case .replaceThenRemove:
      goal = .remove
      status = "Removing the camera…"
      submitDeactivation()
      return
    case .remove:
      status = "The camera is removed."
      onFinished?(true)
      return
    case .install:
      break
    }
    switch result {
    case .completed: status = "Done. “phoneset Camera” is now in the list of cameras."
    case .willCompleteAfterReboot: status = "Done after you restart the Mac."
    @unknown default: status = "Done."
    }
    onFinished?(true)
  }

  func request(_ request: OSSystemExtensionRequest, didFailWithError error: Error) {
    let ns = error as NSError
    if goal == .remove, ns.domain == OSSystemExtensionErrorDomain,
      ns.code == OSSystemExtensionError.Code.extensionNotFound.rawValue
    {
      // Another version is installed (or none): macOS cannot remove the one this app carries. The program checks that
      // the camera is installed before it asks, so this is another version; make it this app's, then remove it.
      goal = .replaceThenRemove
      status = "The installed camera is another version than this app: replacing it first, then removing it…"
      let activate = OSSystemExtensionRequest.activationRequest(
        forExtensionWithIdentifier: Self.extensionID, queue: .main)
      activate.delegate = self
      OSSystemExtensionManager.shared.submitRequest(activate)
      return
    }
    failed = true
    goal = .install
    status = "macOS refused: \(ns.localizedDescription) (\(ns.domain) \(ns.code))"
    onFinished?(false)
  }
}
