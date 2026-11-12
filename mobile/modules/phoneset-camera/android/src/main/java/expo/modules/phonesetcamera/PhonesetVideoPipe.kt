package expo.modules.phonesetcamera

import org.json.JSONObject
import java.nio.ByteBuffer
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

/**
 * The hot path of the video connection, in native code: it pulls frames from the camera, seals them (AES-256-GCM as in
 * docs/protocol.md, section 4), sends them on the socket, reads the PC's acknowledgements and keeps the window of how
 * much may be in flight (section 8.4). JavaScript opens the connection, authenticates and negotiates the mode, then
 * hands the keys and the counters over; from then on it only gets the control messages it has to look at (not the
 * acknowledgements) and, once a second, the numbers.
 *
 * All state is touched on one thread.
 */
class PhonesetVideoPipe(
  private val socket: PhonesetSocket,
  sendKey: ByteArray,
  receiveKey: ByteArray,
  private val transcript: ByteArray,
  private var sent: Long,
  private var received: Long,
  private val windowFrames: Int,
  private val windowBytes: Int,
  depth: Int,
  private val source: FrameProvider
) {
  private val executor = Executors.newSingleThreadExecutor { r -> Thread(r, "phoneset-video-pipe") }
  private val sendKey = SecretKeySpec(sendKey, "AES")
  private val receiveKey = SecretKeySpec(receiveKey, "AES")
  private val depth = maxOf(1, depth)
  private val cipher = Cipher.getInstance("AES/GCM/NoPadding")

  private var pulls = 0
  private val inflight = HashMap<Long, Pair<Long, Int>>() // seq -> (sent at ns, size)
  private var inflightBytes = 0
  private var lastSize = 0
  private var ended: String? = null

  // What JavaScript is told, taken and reset by [numbers].
  private var sentFrames = 0L
  private var sentBytes = 0L
  private var ackNs = 0L
  private var acks = 0L
  private var sendNs = 0L
  private var sends = 0L

  // Control messages JavaScript has to see, and the one that is waiting for the next.
  private val controls = ArrayDeque<String>()
  private var controlReader: ((String?, String?) -> Unit)? = null

  fun start() {
    executor.execute {
      receiveLoop()
      request()
    }
  }

  /** Ends the pipe (and the socket) with a reason. Safe to call more than once. */
  fun stop(reason: String) {
    executor.execute { end(reason) }
  }

  // ---------------------------------------------------------------- sending

  private fun windowOpen() =
    inflight.size < windowFrames && (inflight.isEmpty() || inflightBytes + lastSize <= windowBytes)

  /** Asks the camera for pictures while the window has room and fewer than [depth] requests are out. */
  private fun request() {
    if (ended != null) return
    while (pulls < depth && windowOpen()) {
      pulls++
      source.next { frame, error ->
        executor.execute {
          pulls--
          if (ended != null) return@execute
          if (frame != null) send(frame) else end("The camera stopped: ${error?.message ?: "no frame"}")
        }
      }
    }
  }

  private fun send(frame: ByteArray) {
    if (frame.size < 4) return
    val seq = ByteBuffer.wrap(frame, 0, 4).int.toLong() and 0xFFFFFFFFL
    val started = System.nanoTime()
    inflight[seq] = started to frame.size
    inflightBytes += frame.size
    lastSize = frame.size
    sentFrames++
    sentBytes += frame.size
    // A frame counts as in flight from the moment it exists, so the window decides when the next is asked for.
    request()

    val plain = ByteArray(1 + frame.size)
    plain[0] = FRAME_TYPE
    System.arraycopy(frame, 0, plain, 1, frame.size)
    try {
      socket.send(seal(plain))
    } catch (e: Exception) {
      end("Couldn't seal a video frame.")
      return
    }
    sendNs += System.nanoTime() - started
    sends++
  }

  /** ciphertext || tag, with the counter as the nonce (4 zero bytes, then the counter as a big-endian u64). */
  private fun seal(plain: ByteArray): ByteArray {
    cipher.init(Cipher.ENCRYPT_MODE, sendKey, GCMParameterSpec(TAG_BITS, nonce(sent)))
    cipher.updateAAD(transcript)
    val sealed = cipher.doFinal(plain)
    sent++
    return sealed
  }

  private fun open(message: ByteArray): ByteArray {
    cipher.init(Cipher.DECRYPT_MODE, receiveKey, GCMParameterSpec(TAG_BITS, nonce(received)))
    cipher.updateAAD(transcript)
    val plain = cipher.doFinal(message)
    received++
    return plain
  }

  private fun nonce(counter: Long): ByteArray = ByteBuffer.allocate(12).putInt(0).putLong(counter).array()

  // -------------------------------------------------------------- receiving

  private fun receiveLoop() {
    socket.next { data, error ->
      executor.execute {
        if (ended != null) return@execute
        if (data == null) {
          end(error ?: "The connection closed.")
          return@execute
        }
        try {
          handle(open(data))
        } catch (e: Exception) {
          end("A message from the PC failed to authenticate.")
          return@execute
        }
        receiveLoop()
      }
    }
  }

  private fun handle(plain: ByteArray) {
    if (plain.size < 2 || plain[0] != CONTROL_TYPE) return // other kinds are ignored
    val text = String(plain, 1, plain.size - 1, Charsets.UTF_8)
    val message = try { JSONObject(text) } catch (e: Exception) { return }
    when (message.optString("t")) {
      "vack" -> {
        val entry = inflight.remove(message.optLong("seq", -1))
        if (entry != null) {
          inflightBytes -= entry.second
          ackNs += System.nanoTime() - entry.first
          acks++
        }
        request()
      }
      "vkey" -> source.requestKeyFrame()
      else -> give(text) // vstats, an error, the PC's answer to the mode: for JavaScript
    }
  }

  // ------------------------------------------------------------- for JavaScript

  private fun give(text: String) {
    val reader = controlReader
    if (reader != null) {
      controlReader = null
      reader(text, null)
    } else {
      controls.addLast(text)
    }
  }

  /** The next control message JavaScript has to look at, or why the pipe ended. One reader at a time. */
  fun nextControl(callback: (String?, String?) -> Unit) {
    executor.execute {
      val why = ended
      when {
        controls.isNotEmpty() -> callback(controls.removeFirst(), null)
        why != null -> callback(null, why)
        else -> controlReader = callback
      }
    }
  }

  /** What was done since the last call, for the once-a-second report. */
  fun numbers(): Map<String, Double> {
    val result = AtomicReference<Map<String, Double>>(emptyMap())
    val done = CountDownLatch(1)
    executor.execute {
      result.set(
        mapOf(
          "sentFrames" to sentFrames.toDouble(),
          "sentBytes" to sentBytes.toDouble(),
          "ackSeconds" to ackNs / 1e9,
          "acks" to acks.toDouble(),
          "sendSeconds" to sendNs / 1e9,
          "sends" to sends.toDouble(),
          "inflight" to inflight.size.toDouble()
        )
      )
      sentFrames = 0; sentBytes = 0; ackNs = 0; acks = 0; sendNs = 0; sends = 0
      done.countDown()
    }
    done.await(500, TimeUnit.MILLISECONDS)
    return result.get()
  }

  private fun end(reason: String) {
    if (ended != null) return
    ended = reason
    socket.close()
    val reader = controlReader
    controlReader = null
    reader?.invoke(null, reason)
  }

  companion object {
    private const val FRAME_TYPE: Byte = 0x04
    private const val CONTROL_TYPE: Byte = 0x01
    private const val TAG_BITS = 128
  }
}
