package expo.modules.phonesetcamera

import android.content.pm.PackageManager
import androidx.lifecycle.LifecycleOwner
import expo.modules.kotlin.Promise
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicInteger

/**
 * Exposes FrameSource to JavaScript: start the camera, ask for the next frame, stop. The JavaScript side
 * asks when it can send, so there is no stream of events to fall behind (see FrameSource).
 */
class PhonesetCameraModule : Module() {
  private val context
    get() = appContext.reactContext ?: throw IllegalStateException("React context is gone")

  private val source by lazy { FrameSource(context.applicationContext) }
  private val h264 by lazy { H264Source(context.applicationContext) }

  /** The source the camera was last started with; frames are pulled from it. */
  @Volatile private var current: FrameProvider? = null

  // The native video connection: sockets JavaScript opened by number, and the pipe that took one over.
  private val sockets = ConcurrentHashMap<Int, PhonesetSocket>()
  private val nextSocketId = AtomicInteger(1)
  @Volatile private var pipe: PhonesetVideoPipe? = null

  override fun definition() = ModuleDefinition {
    Name("PhonesetCamera")

    Function("isAvailable") {
      context.packageManager.hasSystemFeature(PackageManager.FEATURE_CAMERA_ANY)
    }

    AsyncFunction("start") { facing: String, width: Int, height: Int, fps: Int, quality: Int, codec: String, promise: Promise ->
      // The camera follows the screen's lifecycle: it works while phoneset is in front.
      val owner = appContext.currentActivity as? LifecycleOwner
      val finished = { error: Throwable? ->
        if (error == null) promise.resolve()
        else promise.reject("ERR_PHONESET_CAMERA", error.message ?: "The camera didn't start.", error)
      }
      if (owner == null) {
        promise.reject("ERR_PHONESET_CAMERA", "phoneset has to be in front to use the camera.", null)
      } else if (codec == "jpeg") {
        h264.stop()
        current = source
        source.start(owner, facing, width, height, fps, quality, finished)
      } else if (codec == "h264") {
        source.stop()
        current = h264
        h264.start(owner, facing, width, height, fps, finished)
      } else {
        promise.reject("ERR_PHONESET_CAMERA", "This phone can't send $codec video.", null)
      }
    }

    AsyncFunction("nextFrame") { promise: Promise ->
      val from = current
      if (from == null) {
        promise.reject("ERR_PHONESET_CAMERA", "The camera is not running.", null)
      } else {
        from.next { frame, error ->
          if (frame != null) promise.resolve(frame)
          else promise.reject("ERR_PHONESET_CAMERA", error?.message ?: "No frame.", error)
        }
      }
    }

    // What this phone's camera reports it can deliver, and which video encoders the system lists.
    Function("capabilities") { facing: String -> Capabilities.camera(context, facing) }

    Function("encoders") { Capabilities.encoders() }

    Function("stop") {
      source.stop()
      h264.stop()
      current = null
    }

    Function("stats") { (current ?: source).stats() }

    // ---- the video connection, run natively

    Function("nativePipe") { true }

    AsyncFunction("socketConnect") { url: String, promise: Promise ->
      val socket = PhonesetSocket()
      socket.connect(url) { error ->
        if (error != null) {
          promise.reject("ERR_PHONESET_SOCKET", error, null)
        } else {
          val id = nextSocketId.getAndIncrement()
          sockets[id] = socket
          promise.resolve(id)
        }
      }
    }

    AsyncFunction("socketReceive") { id: Int, promise: Promise ->
      val socket = sockets[id]
      if (socket == null) {
        promise.reject("ERR_PHONESET_SOCKET", "The connection is gone.", null)
      } else {
        socket.next { data, error ->
          if (data != null) promise.resolve(data)
          else promise.reject("ERR_PHONESET_SOCKET", error ?: "The connection closed.", null)
        }
      }
    }

    Function("socketSend") { id: Int, data: ByteArray ->
      val socket = sockets[id]
      if (socket != null) socket.send(data)
    }

    Function("socketClose") { id: Int ->
      val socket = sockets.remove(id)
      if (socket != null) socket.close()
    }

    // Takes the connection over: from here the pipe seals and sends the frames and reads the acknowledgements.
    // The arguments are packed (Expo's Kotlin functions take at most eight): `material` is the send key, the receive
    // key (32 bytes each) and the transcript; `settings` is sent, received, window frames, window bytes, depth.
    AsyncFunction("pipeStart") { id: Int, material: ByteArray, settings: DoubleArray, promise: Promise ->
      val socket = sockets[id]
      val from = current
      when {
        socket == null -> promise.reject("ERR_PHONESET_PIPE", "The connection is gone.", null)
        from == null -> promise.reject("ERR_PHONESET_PIPE", "The camera is not running.", null)
        material.size <= 64 || settings.size < 5 ->
          promise.reject("ERR_PHONESET_PIPE", "The video connection was handed over wrongly.", null)
        else -> {
          pipe?.stop("A new video connection started.")
          val made = PhonesetVideoPipe(
            socket = socket,
            sendKey = material.copyOfRange(0, 32),
            receiveKey = material.copyOfRange(32, 64),
            transcript = material.copyOfRange(64, material.size),
            sent = settings[0].toLong(),
            received = settings[1].toLong(),
            windowFrames = settings[2].toInt(),
            windowBytes = settings[3].toInt(),
            depth = settings[4].toInt(),
            source = from
          )
          pipe = made
          made.start()
          promise.resolve()
        }
      }
    }

    // The next control message JavaScript has to look at; rejects with why when the pipe has ended.
    AsyncFunction("pipeControl") { promise: Promise ->
      val running = pipe
      if (running == null) {
        promise.reject("ERR_PHONESET_PIPE", "The video is not running.", null)
      } else {
        running.nextControl { text, error ->
          if (text != null) promise.resolve(text)
          else promise.reject("ERR_PHONESET_PIPE", error ?: "The video ended.", null)
        }
      }
    }

    Function("pipeNumbers") { pipe?.numbers() ?: emptyMap<String, Double>() }

    Function("pipeStop") {
      pipe?.stop("The video stopped.")
      pipe = null
    }

    OnDestroy {
      pipe?.stop("The app is closing.")
      source.stop()
      h264.stop()
    }
  }
}
