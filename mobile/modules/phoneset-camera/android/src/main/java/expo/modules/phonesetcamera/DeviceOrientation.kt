package expo.modules.phonesetcamera

import android.content.Context
import android.view.OrientationEventListener
import android.view.Surface

/**
 * Follows how the phone is held, whatever the app's screen is locked to (phoneset's screen is locked to
 * portrait, so Android's display rotation never changes), and reports it as the `Surface.ROTATION_*` value
 * CameraX takes as a use case's target rotation. A phone held across then gives a landscape picture, which is
 * what a webcam is expected to give; held upright it gives portrait.
 *
 * A change is only reported once the phone has turned well past the half-way point, so one held near 45 degrees
 * does not flip back and forth. A phone lying flat reports nothing and keeps the last rotation.
 */
class DeviceOrientation(context: Context, private val onRotation: (Int) -> Unit) : OrientationEventListener(context) {
  private var current = -1

  override fun onOrientationChanged(degrees: Int) {
    if (degrees == ORIENTATION_UNKNOWN) return
    val rotation = when {
      degrees >= 315 || degrees < 45 -> Surface.ROTATION_0
      degrees < 135 -> Surface.ROTATION_270
      degrees < 225 -> Surface.ROTATION_180
      else -> Surface.ROTATION_90
    }
    if (rotation == current) return
    // The angle the sensor reports when the phone is held exactly in the current rotation.
    if (current != -1 && distance(degrees, centre(current)) < HOLD_DEGREES) return
    current = rotation
    onRotation(rotation)
  }

  /** Starts following; false when this phone has no sensor to tell. */
  fun start(): Boolean {
    if (!canDetectOrientation()) return false
    enable()
    return true
  }

  fun stop() {
    disable()
    current = -1
  }

  private fun centre(rotation: Int) = when (rotation) {
    Surface.ROTATION_90 -> 270
    Surface.ROTATION_180 -> 180
    Surface.ROTATION_270 -> 90
    else -> 0
  }

  private fun distance(a: Int, b: Int): Int {
    val d = Math.abs(a - b) % 360
    return if (d > 180) 360 - d else d
  }

  companion object {
    /** The phone has to be this far (degrees) from the rotation in use before it changes: 45 is the exact half. */
    private const val HOLD_DEGREES = 55
  }
}
