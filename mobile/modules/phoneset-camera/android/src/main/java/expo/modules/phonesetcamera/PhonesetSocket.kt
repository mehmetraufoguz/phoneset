package expo.modules.phonesetcamera

import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import okio.ByteString.Companion.toByteString
import java.util.ArrayDeque
import java.util.concurrent.TimeUnit

/**
 * A WebSocket that the video connection runs on, owned by native code. JavaScript uses it for the plaintext
 * handshake and the authentication (a few small messages); once the connection is authenticated the video pipe
 * ([PhonesetVideoPipe]) takes it over, so that frames are sealed and sent without passing through JavaScript,
 * where React Native would turn every message into base64.
 *
 * Messages that arrive are queued; one reader at a time takes the next with [next].
 */
class PhonesetSocket {
  private val lock = Any()
  private val inbox = ArrayDeque<ByteArray>()
  private var failure: String? = null
  private var reader: ((ByteArray?, String?) -> Unit)? = null
  private var opened: ((String?) -> Unit)? = null
  private var socket: WebSocket? = null

  /** Connects; [done] gets null when the socket is open, or why it could not be. */
  fun connect(url: String, done: (String?) -> Unit) {
    val request = try {
      Request.Builder().url(url).build()
    } catch (e: IllegalArgumentException) {
      done("That address is not valid.")
      return
    }
    synchronized(lock) { opened = done }
    socket = CLIENT.newWebSocket(request, object : WebSocketListener() {
      override fun onOpen(webSocket: WebSocket, response: Response) {
        val waiting = synchronized(lock) { opened.also { opened = null } }
        waiting?.invoke(null)
      }

      override fun onMessage(webSocket: WebSocket, bytes: ByteString) {
        push(bytes.toByteArray())
      }

      override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
        fail("The PC closed the connection.")
        webSocket.close(1000, null)
      }

      override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
        fail("The connection closed.")
      }

      override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
        val neverOpened = synchronized(lock) { opened != null }
        fail(if (neverOpened) "Couldn't reach the PC." else "The connection failed.")
      }
    })
  }

  private fun push(data: ByteArray) {
    val waiting: ((ByteArray?, String?) -> Unit)?
    synchronized(lock) {
      waiting = reader
      reader = null
      if (waiting == null) inbox.addLast(data)
    }
    waiting?.invoke(data, null)
  }

  private fun fail(message: String) {
    val why: String?
    val waiting: ((ByteArray?, String?) -> Unit)?
    val opening: ((String?) -> Unit)?
    synchronized(lock) {
      if (failure == null) failure = message
      why = failure
      waiting = reader
      reader = null
      opening = opened
      opened = null
    }
    opening?.invoke(why)
    waiting?.invoke(null, why)
  }

  /** The next message, or why there will be none. One reader at a time. */
  fun next(callback: (ByteArray?, String?) -> Unit) {
    var data: ByteArray? = null
    var why: String? = null
    synchronized(lock) {
      when {
        inbox.isNotEmpty() -> data = inbox.removeFirst()
        failure != null -> why = failure
        else -> {
          reader = callback
          return
        }
      }
    }
    callback(data, why)
  }

  fun send(data: ByteArray) {
    val ok = socket?.send(data.toByteString()) ?: false
    if (!ok) fail("The connection dropped.")
  }

  fun close() {
    socket?.close(1000, null)
    fail("The connection closed.")
  }

  companion object {
    private val CLIENT: OkHttpClient = OkHttpClient.Builder()
      .connectTimeout(5, TimeUnit.SECONDS)
      .readTimeout(0, TimeUnit.MILLISECONDS) // a WebSocket is quiet between messages
      .build()
  }
}
