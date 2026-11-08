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

  override fun definition() = ModuleDefinition {
    Name("PhonesetCamera")

    Function("isAvailable") {
      context.packageManager.hasSystemFeature(PackageManager.FEATURE_CAMERA_ANY)
    }

    AsyncFunction("start") { facing: String, width: Int, height: Int, fps: Int, quality: Int, promise: Promise ->
      // The camera follows the screen's lifecycle: it works while phoneset is in front.
      val owner = appContext.currentActivity as? LifecycleOwner
      if (owner == null) {
        promise.reject("ERR_PHONESET_CAMERA", "phoneset has to be in front to use the camera.", null)
      } else {
        source.start(owner, facing, width, height, fps, quality) { error ->
          if (error == null) promise.resolve()
          else promise.reject("ERR_PHONESET_CAMERA", error.message ?: "The camera didn't start.", error)
        }
      }
    }

    AsyncFunction("nextFrame") { promise: Promise ->
      source.next { frame, error ->
        if (frame != null) promise.resolve(frame)
        else promise.reject("ERR_PHONESET_CAMERA", error?.message ?: "No frame.", error)
      }
    }

    // What this phone's camera reports it can deliver, and which video encoders the system lists.
    Function("capabilities") { facing: String -> Capabilities.camera(context, facing) }

    Function("encoders") { Capabilities.encoders() }

    Function("stop") { source.stop() }

    Function("stats") { source.stats() }

    OnDestroy { source.stop() }
  }
}
