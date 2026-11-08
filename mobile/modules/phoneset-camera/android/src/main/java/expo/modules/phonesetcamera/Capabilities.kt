package expo.modules.phonesetcamera

import android.content.Context
import android.graphics.ImageFormat
import android.hardware.camera2.CameraCharacteristics
import android.hardware.camera2.CameraManager
import android.media.MediaCodecInfo
import android.media.MediaCodecList
import android.os.Build

/**
 * What the phone can do, read from the system and never assumed: the sizes and frame rates a camera
 * reports (Camera2), and the video encoders the system lists (MediaCodec). This is what the phone
 * offers as video modes (docs/protocol.md, section 8.2) and what a codec other than JPEG would need.
 */
object Capabilities {
  /** The sizes the camera can deliver as YUV frames, each with the highest rate it reports for it. */
  fun camera(context: Context, facing: String): Map<String, Any> {
    val manager = context.getSystemService(Context.CAMERA_SERVICE) as CameraManager
    val want =
      if (facing == "front") CameraCharacteristics.LENS_FACING_FRONT else CameraCharacteristics.LENS_FACING_BACK
    val id = manager.cameraIdList.firstOrNull {
      manager.getCameraCharacteristics(it).get(CameraCharacteristics.LENS_FACING) == want
    } ?: return mapOf("sizes" to emptyList<Any>(), "id" to "")
    val c = manager.getCameraCharacteristics(id)
    val map = c.get(CameraCharacteristics.SCALER_STREAM_CONFIGURATION_MAP)
    val sizes = (map?.getOutputSizes(ImageFormat.YUV_420_888) ?: emptyArray()).map { size ->
      val duration = map?.getOutputMinFrameDuration(ImageFormat.YUV_420_888, size) ?: 0L
      mapOf(
        "width" to size.width,
        "height" to size.height,
        "maxFps" to if (duration > 0) (1_000_000_000L / duration).toInt() else 0
      )
    }
    val ranges = (c.get(CameraCharacteristics.CONTROL_AE_AVAILABLE_TARGET_FPS_RANGES) ?: emptyArray())
      .map { "${it.lower}-${it.upper}" }
    return mapOf(
      "id" to id,
      "hardwareLevel" to (c.get(CameraCharacteristics.INFO_SUPPORTED_HARDWARE_LEVEL) ?: -1),
      "fpsRanges" to ranges,
      "sizes" to sizes
    )
  }

  private val tiers = listOf(
    Triple("480p30", 854 to 480, 30.0), Triple("720p30", 1280 to 720, 30.0),
    Triple("720p60", 1280 to 720, 60.0), Triple("1080p30", 1920 to 1080, 30.0),
    Triple("1080p60", 1920 to 1080, 60.0), Triple("2K30", 2560 to 1440, 30.0),
    Triple("2K60", 2560 to 1440, 60.0), Triple("4K30", 3840 to 2160, 30.0), Triple("4K60", 3840 to 2160, 60.0)
  )

  /** The H.264 and H.265 encoders the system lists, and which of the planned modes each says it handles. */
  fun encoders(): List<Map<String, Any>> {
    val out = mutableListOf<Map<String, Any>>()
    for (info in MediaCodecList(MediaCodecList.REGULAR_CODECS).codecInfos) {
      if (!info.isEncoder) continue
      for (mime in info.supportedTypes) {
        if (mime != "video/avc" && mime != "video/hevc") continue
        val video = info.getCapabilitiesForType(mime).videoCapabilities ?: continue
        val modes = tiers.filter { (_, size, fps) ->
          try { video.areSizeAndRateSupported(size.first, size.second, fps) } catch (_: Exception) { false }
        }.map { it.first }
        out += mapOf(
          "name" to info.name,
          "mime" to mime,
          "hardware" to if (Build.VERSION.SDK_INT >= 29) info.isHardwareAccelerated else false,
          "softwareOnly" to if (Build.VERSION.SDK_INT >= 29) info.isSoftwareOnly else false,
          "maxWidth" to video.supportedWidths.upper,
          "maxHeight" to video.supportedHeights.upper,
          "maxBitrateMbps" to video.bitrateRange.upper / 1_000_000.0,
          "colorFormats" to info.getCapabilitiesForType(mime).colorFormats.size,
          "modes" to modes
        )
      }
    }
    return out
  }
}
