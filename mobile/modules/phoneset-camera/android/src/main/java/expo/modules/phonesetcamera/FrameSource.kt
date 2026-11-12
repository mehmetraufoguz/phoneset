package expo.modules.phonesetcamera

import android.content.Context
import android.graphics.Bitmap
import android.graphics.Matrix
import android.os.SystemClock
import android.util.Log
import android.util.Size
import androidx.camera.core.AspectRatio
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.ImageProxy
import androidx.camera.core.resolutionselector.AspectRatioStrategy
import androidx.camera.core.resolutionselector.ResolutionSelector
import androidx.camera.core.resolutionselector.ResolutionStrategy
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.core.content.ContextCompat
import androidx.lifecycle.LifecycleOwner
import java.io.ByteArrayOutputStream
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicReference

class CameraFailure(message: String) : Exception(message)

typealias FrameCallback = (frame: ByteArray?, error: Throwable?) -> Unit

/** A source of frames for JavaScript to pull: JPEG ([FrameSource]) or H.264 ([H264Source]). */
interface FrameProvider {
  /** Waits for the next frame; the callback gets the frame message body, or an error. */
  fun next(callback: FrameCallback)
  fun stop()
  /** What the camera side has done since it started, for measuring. */
  fun stats(): Map<String, Any>
  /** The PC asked for a picture that can be decoded on its own. A JPEG always is one, so only H.264 acts on this. */
  fun requestKeyFrame() {}
}

/**
 * The phone's camera as a source of JPEG frames, with no Expo types in it.
 *
 * CameraX hands every analysed picture to [analyze]. A picture is only turned into a JPEG when
 * something is waiting for one ([next]) and the frame rate allows it, so the CPU follows what is
 * actually sent: when the connection is slow, JavaScript does not ask, and the camera's pictures are
 * dropped here, before any work and long before they could queue behind the microphone.
 *
 * A frame is the message body of docs/protocol.md type 0x04: seq (u32), capture time in ms (u32),
 * codec (u8), flags (u8), width (u16), height (u16), then the JPEG. The picture is upright and not
 * mirrored.
 */
class FrameSource(private val context: Context) : FrameProvider {
  private val analysisExecutor = Executors.newSingleThreadExecutor { r -> Thread(r, "phoneset-camera") }
  private val waiter = AtomicReference<FrameCallback?>(null)
  private val jpeg = ByteArrayOutputStream(256 * 1024)

  private var provider: ProcessCameraProvider? = null

  @Volatile private var minIntervalNs = 0L
  @Volatile private var quality = 70
  @Volatile private var lastEncodeNs = 0L
  @Volatile private var seq = 0

  // Counters for [stats]; written by the analysis thread, read from any.
  private val lock = Any()
  private var analysed = 0L
  private var encoded = 0L
  private var encodeNs = 0L
  private var jpegBytes = 0L
  private var lagMs = 0L
  private var lagSamples = 0L
  private var startedMs = 0L
  private var width = 0
  private var height = 0

  /** Opens the camera. Done is called on the main thread, with null when frames are coming. */
  fun start(
    owner: LifecycleOwner,
    facing: String,
    width: Int,
    height: Int,
    fps: Int,
    quality: Int,
    done: (Throwable?) -> Unit
  ) {
    val main = ContextCompat.getMainExecutor(context)
    main.execute {
      try {
        stopOnMain()
        this.quality = quality.coerceIn(30, 95)
        this.minIntervalNs = 1_000_000_000L / fps.coerceIn(1, 60)
        this.seq = 0
        this.lastEncodeNs = 0L
        synchronized(lock) {
          analysed = 0; encoded = 0; encodeNs = 0; jpegBytes = 0; lagMs = 0; lagSamples = 0
          startedMs = SystemClock.elapsedRealtime()
        }
      } catch (e: Throwable) {
        done(e)
        return@execute
      }
      val future = ProcessCameraProvider.getInstance(context)
      future.addListener({
        try {
          val provider = future.get()
          val selector = CameraSelector.Builder()
            .requireLensFacing(
              if (facing == "front") CameraSelector.LENS_FACING_FRONT else CameraSelector.LENS_FACING_BACK
            )
            .build()
          if (!provider.hasCamera(selector)) throw CameraFailure("This phone has no $facing camera.")
          val resolution = ResolutionSelector.Builder()
            .setAspectRatioStrategy(
              AspectRatioStrategy(AspectRatio.RATIO_16_9, AspectRatioStrategy.FALLBACK_RULE_AUTO)
            )
            .setResolutionStrategy(
              ResolutionStrategy(
                Size(width, height),
                ResolutionStrategy.FALLBACK_RULE_CLOSEST_HIGHER_THEN_LOWER
              )
            )
            .build()
          val analysis = ImageAnalysis.Builder()
            .setResolutionSelector(resolution)
            .setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST)
            .setOutputImageFormat(ImageAnalysis.OUTPUT_IMAGE_FORMAT_RGBA_8888)
            .build()
          analysis.setAnalyzer(analysisExecutor, ImageAnalysis.Analyzer { image -> analyze(image) })
          provider.unbindAll()
          provider.bindToLifecycle(owner, selector, analysis)
          this.provider = provider
          done(null)
        } catch (e: Throwable) {
          done(e)
        }
      }, main)
    }
  }

  /** Stops the camera and fails whoever was waiting for a frame. */
  override fun stop() {
    ContextCompat.getMainExecutor(context).execute { stopOnMain() }
    waiter.getAndSet(null)?.invoke(null, CameraFailure("The camera was stopped."))
  }

  private fun stopOnMain() {
    provider?.unbindAll()
    provider = null
  }

  /** Waits for the next frame. A call replaces one that is still waiting. */
  override fun next(callback: FrameCallback) {
    waiter.getAndSet(callback)?.invoke(null, CameraFailure("Another frame was asked for."))
  }

  private fun analyze(image: ImageProxy) {
    try {
      synchronized(lock) { analysed++ }
      val callback = waiter.get() ?: return // nobody is ready for a picture: it is dropped
      val now = System.nanoTime()
      // Faster than asked for: dropped. The camera's own 30 fps is never exactly 33.3 ms apart, so a
      // picture a little early still counts, or about every other one would be thrown away.
      if (now - lastEncodeNs < minIntervalNs * 85 / 100) return
      if (!waiter.compareAndSet(callback, null)) return
      lastEncodeNs = now
      try {
        callback(encode(image, now, System.currentTimeMillis()), null)
      } catch (e: Throwable) {
        callback(null, e)
      }
    } finally {
      image.close()
    }
  }

  private fun encode(image: ImageProxy, startNs: Long, entryWallMs: Long): ByteArray {
    // How old the picture already is when it reaches us (only meaningful when the camera's clock is the
    // phone's steady clock; anything outside 0..1 s means it is not).
    val nowMs = SystemClock.elapsedRealtime()
    val lag = nowMs - image.imageInfo.timestamp / 1_000_000L
    val lagKnown = lag in 0..1000
    val capturedMs = if (lagKnown) nowMs - lag else nowMs

    var bitmap = image.toBitmap()
    val rotation = image.imageInfo.rotationDegrees
    if (rotation != 0) {
      val matrix = Matrix().apply { postRotate(rotation.toFloat()) }
      val rotated = Bitmap.createBitmap(bitmap, 0, 0, bitmap.width, bitmap.height, matrix, true)
      bitmap.recycle()
      bitmap = rotated
    }
    val w = bitmap.width
    val h = bitmap.height

    jpeg.reset()
    jpeg.write(ByteArray(HEADER)) // the header is filled in below
    bitmap.compress(Bitmap.CompressFormat.JPEG, quality, jpeg)
    bitmap.recycle()
    val out = jpeg.toByteArray()

    val n = seq++
    putInt(out, 0, n)
    putInt(out, 4, capturedMs.toInt())
    out[8] = CODEC_JPEG
    out[9] = FLAG_KEY // every JPEG can be decoded on its own
    out[10] = (w shr 8).toByte(); out[11] = w.toByte()
    out[12] = (h shr 8).toByte(); out[13] = h.toByte()

    // For measuring: `adb shell setprop log.tag.PhonesetCamera VERBOSE` prints when each frame was
    // captured by the phone's wall clock, to compare with when the PC delivered it.
    if (Log.isLoggable(TAG, Log.VERBOSE)) {
      Log.v(TAG, "frame seq=$n captureWallMs=${entryWallMs - (if (lagKnown) lag else 0)} lagKnown=$lagKnown " +
        "encodeMs=${(System.nanoTime() - startNs) / 1e6} bytes=${out.size - HEADER} size=${w}x$h")
    }

    synchronized(lock) {
      encoded++
      encodeNs += System.nanoTime() - startNs
      jpegBytes += out.size - HEADER
      if (lagKnown) { lagMs += lag; lagSamples++ }
      width = w; height = h
    }
    return out
  }

  /** What the camera side has done since it started, for measuring. */
  override fun stats(): Map<String, Any> = synchronized(lock) {
    val seconds = maxOf(1L, SystemClock.elapsedRealtime() - startedMs) / 1000.0
    mapOf(
      "width" to width,
      "height" to height,
      "analysedFps" to analysed / seconds,
      "encodedFps" to encoded / seconds,
      "encodeMs" to if (encoded == 0L) 0.0 else encodeNs / 1e6 / encoded,
      "jpegKB" to if (encoded == 0L) 0.0 else jpegBytes / 1024.0 / encoded,
      "captureLagMs" to if (lagSamples == 0L) -1.0 else lagMs.toDouble() / lagSamples,
      "encoded" to encoded.toDouble()
    )
  }

  private fun putInt(b: ByteArray, at: Int, v: Int) {
    b[at] = (v ushr 24).toByte(); b[at + 1] = (v ushr 16).toByte()
    b[at + 2] = (v ushr 8).toByte(); b[at + 3] = v.toByte()
  }

  companion object {
    const val HEADER = 14
    const val CODEC_JPEG: Byte = 1
    const val FLAG_KEY: Byte = 1
    private const val TAG = "PhonesetCamera"
  }
}
