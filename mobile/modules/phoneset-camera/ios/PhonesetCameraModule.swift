import AVFoundation
import ExpoModulesCore

private final class CameraException: GenericException<String> {
  override var reason: String {
    param
  }
}

/// Exposes PhonesetCameraCore to JavaScript: start the camera, ask for the next frame, stop. The
/// JavaScript side asks when it can send, so there is no stream of events to fall behind.
///
/// It also owns the sockets of the video connection and the pipe that runs it once it is authenticated
/// (PhonesetSocket, PhonesetVideoPipe), so that frames do not have to pass through JavaScript.
public class PhonesetCameraModule: Module {
  private let camera = PhonesetCameraCore()
  private let lock = NSLock()
  private var sockets: [Int: PhonesetSocket] = [:]
  private var nextSocketId = 1
  private var pipe: PhonesetVideoPipe?

  private func socket(_ id: Int) -> PhonesetSocket? {
    lock.lock()
    defer { lock.unlock() }
    return sockets[id]
  }

  public func definition() -> ModuleDefinition {
    Name("PhonesetCamera")

    Function("isAvailable") { () -> Bool in
      AVCaptureDevice.default(for: .video) != nil
    }

    AsyncFunction("start") { (facing: String, width: Int, height: Int, fps: Int, quality: Int, codec: String) throws in
      do {
        try self.camera.start(
          facing: facing, width: width, height: height, fps: fps, quality: quality, codec: codec)
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

    // What this phone's camera reports it can deliver.
    Function("capabilities") { (facing: String) -> [String: Any] in
      PhonesetCameraCore.capabilities(facing: facing)
    }

    Function("stop") {
      self.camera.stop()
    }

    Function("stats") { () -> [String: Double] in
      self.camera.stats()
    }

    // ---- the video connection, run natively

    Function("nativePipe") { () -> Bool in true }

    AsyncFunction("socketConnect") { (url: String, promise: Promise) in
      guard let target = URL(string: url) else {
        promise.reject("ERR_PHONESET_SOCKET", "That address is not valid.")
        return
      }
      let socket = PhonesetSocket()
      socket.connect(url: target) { error in
        if let error {
          promise.reject("ERR_PHONESET_SOCKET", error)
          return
        }
        self.lock.lock()
        let id = self.nextSocketId
        self.nextSocketId += 1
        self.sockets[id] = socket
        self.lock.unlock()
        promise.resolve(id)
      }
    }

    AsyncFunction("socketReceive") { (id: Int, promise: Promise) in
      guard let socket = self.socket(id) else {
        promise.reject("ERR_PHONESET_SOCKET", "The connection is gone.")
        return
      }
      socket.next { data, error in
        if let data {
          promise.resolve(data)
        } else {
          promise.reject("ERR_PHONESET_SOCKET", error ?? "The connection closed.")
        }
      }
    }

    Function("socketSend") { (id: Int, data: Data) in
      self.socket(id)?.send(data)
    }

    Function("socketClose") { (id: Int) in
      self.lock.lock()
      let socket = self.sockets.removeValue(forKey: id)
      self.lock.unlock()
      socket?.close()
    }

    // Takes the connection over: from here the pipe seals and sends the frames and reads the acknowledgements.
    // The arguments are packed, the same on both platforms (Expo's Kotlin functions take at most eight): `material`
    // is the send key, the receive key (32 bytes each) and the transcript; `settings` is sent, received, window
    // frames, window bytes, depth.
    AsyncFunction("pipeStart") { (id: Int, material: Data, settings: [Double]) throws in
      guard let socket = self.socket(id) else { throw CameraException("The connection is gone.") }
      guard material.count > 64, settings.count >= 5 else {
        throw CameraException("The video connection was handed over wrongly.")
      }
      let bytes = [UInt8](material)
      self.pipe?.stop("A new video connection started.")
      let pipe = PhonesetVideoPipe(
        socket: socket, sendKey: Data(bytes[0..<32]), receiveKey: Data(bytes[32..<64]),
        transcript: Data(bytes[64...]), sent: UInt64(settings[0]), received: UInt64(settings[1]),
        window: .init(frames: Int(settings[2]), bytes: Int(settings[3])), depth: Int(settings[4]),
        source: self.camera)
      self.pipe = pipe
      pipe.start()
    }

    // The next control message JavaScript has to look at; rejects with why when the pipe has ended.
    AsyncFunction("pipeControl") { (promise: Promise) in
      guard let pipe = self.pipe else {
        promise.reject("ERR_PHONESET_PIPE", "The video is not running.")
        return
      }
      pipe.nextControl { text, error in
        if let text {
          promise.resolve(text)
        } else {
          promise.reject("ERR_PHONESET_PIPE", error ?? "The video ended.")
        }
      }
    }

    Function("pipeNumbers") { () -> [String: Double] in
      self.pipe?.numbers() ?? [:]
    }

    Function("pipeStop") {
      self.pipe?.stop("The video stopped.")
      self.pipe = nil
    }

    OnDestroy {
      self.pipe?.stop("The app is closing.")
      self.camera.stop()
    }
  }
}
