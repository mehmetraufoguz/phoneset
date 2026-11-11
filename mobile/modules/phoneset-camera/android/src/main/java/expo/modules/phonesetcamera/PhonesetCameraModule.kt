package expo.modules.phonesetcamera

import android.content.pm.PackageManager
import androidx.lifecycle.LifecycleOwner
import expo.modules.kotlin.Promise
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition

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

    OnDestroy {
      source.stop()
      h264.stop()
    }
  }
}
