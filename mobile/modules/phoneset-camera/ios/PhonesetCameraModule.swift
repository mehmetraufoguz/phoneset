import AVFoundation
import ExpoModulesCore

private final class CameraException: GenericException<String> {
  override var reason: String {
    param
  }
}

/// Exposes PhonesetCameraCore to JavaScript: start the camera, ask for the next frame, stop. The
/// JavaScript side asks when it can send, so there is no stream of events to fall behind.
public class PhonesetCameraModule: Module {
  private let camera = PhonesetCameraCore()

  public func definition() -> ModuleDefinition {
    Name("PhonesetCamera")

    Function("isAvailable") { () -> Bool in
      AVCaptureDevice.default(for: .video) != nil
    }

    AsyncFunction("start") { (facing: String, width: Int, height: Int, fps: Int, quality: Int) throws in
      do {
        try self.camera.start(facing: facing, width: width, height: height, fps: fps, quality: quality)
      } catch let failure as CameraFailure {
        throw CameraException(failure.message)
      }
    }

    AsyncFunction("nextFrame") { (promise: Promise) in
      self.camera.next { frame, error in
        if let frame {
          promise.resolve(frame)
        } else {
          promise.reject("ERR_PHONESET_CAMERA", error ?? "No frame.")
        }
      }
    }

    Function("stop") {
      self.camera.stop()
    }

    Function("stats") { () -> [String: Double] in
      self.camera.stats()
    }

    OnDestroy {
      self.camera.stop()
    }
  }
}
