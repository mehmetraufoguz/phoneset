package expo.modules.phonesetcamera

import android.content.Context
import android.media.MediaCodec
import android.media.MediaCodecInfo
import android.media.MediaFormat
import android.os.Build
import android.os.Bundle
import android.os.SystemClock
import android.util.Log
import android.util.Range
import android.util.Size
import androidx.camera.core.AspectRatio
import androidx.camera.core.CameraSelector
import androidx.camera.core.Preview
import androidx.camera.core.SurfaceRequest
import androidx.camera.core.resolutionselector.AspectRatioStrategy
import androidx.camera.core.resolutionselector.ResolutionSelector
import androidx.camera.core.resolutionselector.ResolutionStrategy
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.core.content.ContextCompat
import androidx.lifecycle.LifecycleOwner
import java.util.ArrayDeque
import java.util.concurrent.Executors

/**
 * The phone's camera as a source of H.264 frames, coded by the hardware encoder (MediaCodec).
 *
 * CameraX's Preview use case draws the camera's pictures straight onto the encoder's input surface, so no
 * picture is copied or converted by the CPU. The encoder hands back one Annex B access unit per picture
 * (docs/protocol.md, section 8.3): the SPS and the PPS in front of every key frame, no B-frames, a key
 * frame every 2 seconds. The picture is not rotated here: it is in the camera's own orientation, `width` and
 * `height` in the frame are the coded size, and bits 1 and 2 of `flags` say how many quarter turns clockwise
 * the receiver applies to make it upright (CameraX's transformation info for the way the phone is held).
 *
 * A coded frame depends on those before it, so it is never dropped alone: frames wait for a request in a short
 * queue, and when it overflows everything in it is dropped, the encoder is asked for a key frame and nothing
 * is sent until that arrives (the PC's reader does the same on a gap in `seq`).
 */
class H264Source(private val context: Context) : FrameProvider {
  private val lock = Any()
  private val waiters = ArrayDeque<FrameCallback>()
  private val ready = ArrayDeque<ByteArray>()
  private val surfaceExecutor = Executors.newSingleThreadExecutor { r -> Thread(r, "phoneset-h264-surface") }

  private var provider: ProcessCameraProvider? = null
  private var codec: MediaCodec? = null
  private var drain: Thread? = null
  @Volatile private var running = false

  private var config: ByteArray? = null // SPS and PPS, Annex B
  private var seq = 0
  private var needKey = true
  @Volatile private var turns = 0 // quarter turns clockwise to make the picture upright
  private var width = 0
  private var height = 0
  private var startedMs = 0L
  private var frames = 0L
  private var bytes = 0L
  private var dropped = 0L

  /** Opens the camera. Done is called on the main thread, with null when the encoder is running. */
  fun start(owner: LifecycleOwner, facing: String, wantWidth: Int, wantHeight: Int, fps: Int, done: (Throwable?) -> Unit) {
    val main = ContextCompat.getMainExecutor(context)
    main.execute {
      try {
        stopOnMain()
        synchronized(lock) {
          ready.clear(); config = null; seq = 0; needKey = true; turns = 0
          frames = 0; bytes = 0; dropped = 0; startedMs = SystemClock.elapsedRealtime()
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
              ResolutionStrategy(Size(wantWidth, wantHeight), ResolutionStrategy.FALLBACK_RULE_CLOSEST_HIGHER_THEN_LOWER)
            )
            .build()
          val preview = Preview.Builder()
            .setResolutionSelector(resolution)
            .setTargetFrameRate(Range(fps, fps))
            .build()
          var answered = false
          preview.setSurfaceProvider(surfaceExecutor) { request ->
            // The size CameraX chose is only known now, and the encoder has to be made for exactly that size.
            val failure = try { openEncoder(request, fps); null } catch (e: Throwable) { request.willNotProvideSurface(); e }
            if (!answered) {
              answered = true
              main.execute { done(failure) }
            }
          }
          provider.unbindAll()
          provider.bindToLifecycle(owner, selector, preview)
          this.provider = provider
        } catch (e: Throwable) {
          done(e)
        }
      }, main)
    }
  }

  private fun openEncoder(request: SurfaceRequest, fps: Int) {
    val size = request.resolution
    val bitrate = (size.width.toLong() * size.height * fps / 10).coerceIn(1_500_000L, 60_000_000L).toInt()
    var made: MediaCodec? = null
    var surface: android.view.Surface? = null
    // High profile compresses best; a coder that refuses it gets the default one.
    for (withProfile in listOf(true, false)) {
      val format = MediaFormat.createVideoFormat(MediaFormat.MIMETYPE_VIDEO_AVC, size.width, size.height)
      format.setInteger(MediaFormat.KEY_COLOR_FORMAT, MediaCodecInfo.CodecCapabilities.COLOR_FormatSurface)
      format.setInteger(MediaFormat.KEY_BIT_RATE, bitrate)
      format.setInteger(MediaFormat.KEY_FRAME_RATE, fps)
      format.setInteger(MediaFormat.KEY_I_FRAME_INTERVAL, 2)
      format.setInteger(MediaFormat.KEY_BITRATE_MODE, MediaCodecInfo.EncoderCapabilities.BITRATE_MODE_VBR)
      if (Build.VERSION.SDK_INT >= 23) format.setInteger(MediaFormat.KEY_PRIORITY, 0) // real time
      format.setInteger("prepend-sps-pps-to-idr-frames", 1) // ignored by a coder that does not know it
      if (withProfile) {
        format.setInteger(MediaFormat.KEY_PROFILE, MediaCodecInfo.CodecProfileLevel.AVCProfileHigh)
      }
      val candidate = MediaCodec.createEncoderByType(MediaFormat.MIMETYPE_VIDEO_AVC)
      try {
        candidate.configure(format, null, null, MediaCodec.CONFIGURE_FLAG_ENCODE)
        surface = candidate.createInputSurface()
        candidate.start()
        made = candidate
        break
      } catch (e: Throwable) {
        candidate.release()
        if (!withProfile) throw CameraFailure("The hardware H.264 coder refused ${size.width} x ${size.height}: ${e.message}")
      }
    }
    val encoder = made ?: throw CameraFailure("No H.264 coder.")
    val input = surface ?: throw CameraFailure("The H.264 coder gave no input surface.")
    synchronized(lock) { width = size.width; height = size.height }
    codec = encoder
    // How far the picture has to be turned to be upright for the way the phone is held; CameraX tells us, and
    // tells again when the phone is turned. The front camera is not mirrored, so bit 3 stays 0.
    request.setTransformationInfoListener(surfaceExecutor) { info ->
      turns = ((info.rotationDegrees / 90) % 4 + 4) % 4
    }
    request.provideSurface(input, surfaceExecutor) { input.release() }
    running = true
    drain = Thread({ drainLoop(encoder) }, "phoneset-h264-drain").also { it.start() }
  }

  private fun drainLoop(encoder: MediaCodec) {
    val info = MediaCodec.BufferInfo()
    try {
      while (running) {
        val index = encoder.dequeueOutputBuffer(info, 10_000)
        when {
          index >= 0 -> {
            val buffer = encoder.getOutputBuffer(index)
            if (buffer != null && info.size > 0) {
              buffer.position(info.offset)
              buffer.limit(info.offset + info.size)
              val data = ByteArray(info.size)
              buffer.get(data)
              if (info.flags and MediaCodec.BUFFER_FLAG_CODEC_CONFIG != 0) {
                synchronized(lock) { config = data }
              } else {
                publish(data, info.flags and MediaCodec.BUFFER_FLAG_KEY_FRAME != 0, info.presentationTimeUs)
              }
            }
            encoder.releaseOutputBuffer(index, false)
          }
          index == MediaCodec.INFO_OUTPUT_FORMAT_CHANGED -> {
            val format = encoder.outputFormat
            val sps = format.getByteBuffer("csd-0")
            val pps = format.getByteBuffer("csd-1")
            if (sps != null && pps != null) {
              val all = ByteArray(sps.remaining() + pps.remaining())
              sps.duplicate().get(all, 0, sps.remaining())
              pps.duplicate().get(all, sps.remaining(), pps.remaining())
              synchronized(lock) { if (config == null) config = all }
            }
          }
        }
      }
    } catch (_: IllegalStateException) {
      // The encoder was released while this loop was waiting: the camera was stopped.
    }
  }

  private fun publish(data: ByteArray, key: Boolean, presentationUs: Long) {
    var callback: FrameCallback? = null
    var frame: ByteArray? = null
    synchronized(lock) {
      if (needKey && !key) { dropped++; return }
      needKey = false
      val parameterSets = config
      // The SPS and PPS go in front of a key frame that does not already start with them.
      val prepend = key && parameterSets != null && !startsWithSps(data)
      val payload = if (prepend) parameterSets!!.size + data.size else data.size
      val out = ByteArray(HEADER + payload)
      putInt(out, 0, seq++)
      putInt(out, 4, (presentationUs / 1000).toInt())
      out[8] = CODEC_H264
      // Bit 0: key frame. Bits 1 and 2: the quarter turns clockwise that make the picture upright (section 8.3).
      out[9] = ((if (key) FLAG_KEY.toInt() else 0) or (turns shl 1)).toByte()
      out[10] = (width shr 8).toByte(); out[11] = width.toByte()
      out[12] = (height shr 8).toByte(); out[13] = height.toByte()
      if (prepend) {
        System.arraycopy(parameterSets!!, 0, out, HEADER, parameterSets.size)
        System.arraycopy(data, 0, out, HEADER + parameterSets.size, data.size)
      } else {
        System.arraycopy(data, 0, out, HEADER, data.size)
      }
      frames++
      bytes += payload
      callback = waiters.pollFirst()
      if (callback != null) {
        frame = out
      } else {
        ready.addLast(out)
        if (ready.size > MAX_READY) {
          // Nobody is taking frames: drop all of them and start again at the next key frame.
          dropped += ready.size
          ready.clear()
          needKey = true
          requestKeyFrame()
        }
      }
    }
    val cb = callback
    val f = frame
    if (cb != null && f != null) cb(f, null)
  }

  private fun startsWithSps(data: ByteArray): Boolean {
    // 00 00 00 01 67 (SPS) or 00 00 01 67
    if (data.size > 5 && data[0] == 0.toByte() && data[1] == 0.toByte() && data[2] == 0.toByte() && data[3] == 1.toByte()) {
      return (data[4].toInt() and 0x1F) == 7
    }
    if (data.size > 4 && data[0] == 0.toByte() && data[1] == 0.toByte() && data[2] == 1.toByte()) {
      return (data[3].toInt() and 0x1F) == 7
    }
    return false
  }

  /** Asks the encoder for a key frame now (the PC's reader joined, or lost a frame); it comes with the next picture. */
  override fun requestKeyFrame() {
    try {
      codec?.setParameters(Bundle().apply { putInt(MediaCodec.PARAMETER_KEY_REQUEST_SYNC_FRAME, 0) })
    } catch (_: Throwable) {
      // The coder is gone; the next start begins with a key frame anyway.
    }
  }

  /** Waits for the next frame. Several requests may be waiting at once; the oldest is answered first. */
  override fun next(callback: FrameCallback) {
    var frame: ByteArray?
    var replaced: FrameCallback? = null
    synchronized(lock) {
      frame = ready.pollFirst()
      if (frame == null) {
        waiters.addLast(callback)
        if (waiters.size > MAX_WAITING) replaced = waiters.pollFirst()
      }
    }
    val f = frame
    if (f != null) callback(f, null)
    replaced?.invoke(null, CameraFailure("Another frame was asked for."))
  }

  /** Stops the camera and the encoder and fails whoever was waiting for a frame. */
  override fun stop() {
    running = false
    ContextCompat.getMainExecutor(context).execute { stopOnMain() }
    val thread = drain
    drain = null
    thread?.join(500)
    val encoder = codec
    codec = null
    try { encoder?.stop() } catch (_: Throwable) { }
    try { encoder?.release() } catch (_: Throwable) { }
    val pending = synchronized(lock) {
      val all = waiters.toList()
      waiters.clear()
      ready.clear()
      all
    }
    pending.forEach { it(null, CameraFailure("The camera was stopped.")) }
  }

  private fun stopOnMain() {
    provider?.unbindAll()
    provider = null
  }

  /** What the camera side has done since it started, for measuring. */
  override fun stats(): Map<String, Any> = synchronized(lock) {
    val seconds = maxOf(1L, SystemClock.elapsedRealtime() - startedMs) / 1000.0
    // For measuring: `adb shell setprop log.tag.PhonesetCamera VERBOSE` prints these once a second.
    if (Log.isLoggable(TAG, Log.VERBOSE)) {
      Log.v(TAG, "h264 ${width}x$height frames/s=${"%.1f".format(frames / seconds)} " +
        "KB/frame=${"%.0f".format(if (frames == 0L) 0.0 else bytes / 1024.0 / frames)} dropped=$dropped ready=${ready.size}")
    }
    mapOf(
      "width" to width,
      "height" to height,
      "analysedFps" to frames / seconds,
      "encodedFps" to frames / seconds,
      "encodeMs" to 0.0,
      "jpegKB" to if (frames == 0L) 0.0 else bytes / 1024.0 / frames, // the size of what was coded
      "captureLagMs" to -1.0,
      "encoded" to frames.toDouble()
    )
  }

  private fun putInt(b: ByteArray, at: Int, v: Int) {
    b[at] = (v ushr 24).toByte(); b[at + 1] = (v ushr 16).toByte()
    b[at + 2] = (v ushr 8).toByte(); b[at + 3] = v.toByte()
  }

  companion object {
    const val HEADER = 14
    const val CODEC_H264: Byte = 2
    const val FLAG_KEY: Byte = 1
    private const val MAX_READY = 8 // frames that may wait for a request
    private const val MAX_WAITING = 4 // requests that may wait for a frame
    private const val TAG = "PhonesetCamera"
  }
}
