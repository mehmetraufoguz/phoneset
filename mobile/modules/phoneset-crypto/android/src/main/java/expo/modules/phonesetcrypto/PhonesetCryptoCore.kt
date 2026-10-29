package expo.modules.phonesetcrypto

import java.math.BigInteger
import java.security.AlgorithmParameters
import java.security.KeyFactory
import java.security.KeyPairGenerator
import java.security.MessageDigest
import java.security.interfaces.ECPrivateKey
import java.security.interfaces.ECPublicKey
import java.security.spec.ECFieldFp
import java.security.spec.ECGenParameterSpec
import java.security.spec.ECParameterSpec
import java.security.spec.ECPoint
import java.security.spec.ECPrivateKeySpec
import java.security.spec.ECPublicKeySpec
import javax.crypto.Cipher
import javax.crypto.KeyAgreement
import javax.crypto.Mac
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

/**
 * The primitives of the phoneset protocol (docs/protocol.md), all provided by the Java
 * cryptography architecture, so the app carries no cryptography of its own. Plain Kotlin on top
 * of java.security: no Android or Expo types in this file.
 */
object PhonesetCryptoCore {
  class Failure(message: String, cause: Throwable? = null) : Exception(message, cause)

  private const val SECRET_LENGTH = 32
  private const val PUBLIC_LENGTH = 65
  private const val KEY_LENGTH = 32
  private const val NONCE_LENGTH = 12
  private const val TAG_BYTES = 16
  private const val HMAC = "HmacSHA256"

  private val curve: ECParameterSpec by lazy {
    AlgorithmParameters.getInstance("EC").run {
      init(ECGenParameterSpec("secp256r1"))
      getParameterSpec(ECParameterSpec::class.java)
    }
  }

  /** secret (32 bytes) || public key (65 bytes, uncompressed X9.63). */
  fun generateKeyPair(): ByteArray {
    val generator = KeyPairGenerator.getInstance("EC")
    generator.initialize(ECGenParameterSpec("secp256r1"))
    val pair = generator.generateKeyPair()
    val secret = (pair.private as ECPrivateKey).s
    val point = (pair.public as ECPublicKey).w
    return fixed(secret, SECRET_LENGTH) + encode(point)
  }

  /** The raw ECDH output: the 32-byte x coordinate of the shared point. */
  fun sharedSecret(secret: ByteArray, peerPublic: ByteArray): ByteArray {
    if (secret.size != SECRET_LENGTH || peerPublic.size != PUBLIC_LENGTH || peerPublic[0] != 4.toByte()) {
      throw Failure("malformed key")
    }
    val x = BigInteger(1, peerPublic.copyOfRange(1, 33))
    val y = BigInteger(1, peerPublic.copyOfRange(33, 65))
    // Do not rely on the provider to refuse a point that is not on the curve.
    if (!onCurve(x, y)) throw Failure("the peer's key is not a point on the curve")
    try {
      val factory = KeyFactory.getInstance("EC")
      val privateKey = factory.generatePrivate(ECPrivateKeySpec(BigInteger(1, secret), curve))
      val publicKey = factory.generatePublic(ECPublicKeySpec(ECPoint(x, y), curve))
      val agreement = KeyAgreement.getInstance("ECDH")
      agreement.init(privateKey)
      agreement.doPhase(publicKey, true)
      return agreement.generateSecret()
    } catch (e: java.security.GeneralSecurityException) {
      throw Failure("key agreement failed", e)
    }
  }

  /** RFC 5869 over HMAC-SHA-256 (the Java cryptography architecture has no HKDF class). */
  fun hkdf(ikm: ByteArray, salt: ByteArray, info: ByteArray, length: Int): ByteArray {
    if (length < 1 || length > 255 * 32) throw Failure("bad output length")
    val pseudoRandomKey = hmac(if (salt.isEmpty()) ByteArray(32) else salt, ikm)
    val output = ByteArray(length)
    var block = ByteArray(0)
    var written = 0
    var counter = 1
    while (written < length) {
      val mac = Mac.getInstance(HMAC)
      mac.init(SecretKeySpec(pseudoRandomKey, HMAC))
      mac.update(block)
      mac.update(info)
      mac.update(counter.toByte())
      block = mac.doFinal()
      val n = minOf(block.size, length - written)
      System.arraycopy(block, 0, output, written, n)
      written += n
      counter++
    }
    return output
  }

  fun hmac(key: ByteArray, data: ByteArray): ByteArray {
    val mac = Mac.getInstance(HMAC)
    mac.init(SecretKeySpec(key, HMAC))
    return mac.doFinal(data)
  }

  fun sha256(data: ByteArray): ByteArray = MessageDigest.getInstance("SHA-256").digest(data)

  /** ciphertext || 16-byte tag. */
  fun seal(key: ByteArray, nonce: ByteArray, aad: ByteArray, plaintext: ByteArray): ByteArray {
    if (key.size != KEY_LENGTH || nonce.size != NONCE_LENGTH) throw Failure("bad key or nonce length")
    try {
      val cipher = Cipher.getInstance("AES/GCM/NoPadding")
      cipher.init(Cipher.ENCRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(TAG_BYTES * 8, nonce))
      cipher.updateAAD(aad)
      return cipher.doFinal(plaintext)
    } catch (e: java.security.GeneralSecurityException) {
      throw Failure("encryption failed", e)
    }
  }

  fun open(key: ByteArray, nonce: ByteArray, aad: ByteArray, sealed: ByteArray): ByteArray {
    if (key.size != KEY_LENGTH || nonce.size != NONCE_LENGTH || sealed.size < TAG_BYTES) {
      throw Failure("bad key, nonce or message length")
    }
    try {
      val cipher = Cipher.getInstance("AES/GCM/NoPadding")
      cipher.init(Cipher.DECRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(TAG_BYTES * 8, nonce))
      cipher.updateAAD(aad)
      return cipher.doFinal(sealed)
    } catch (e: java.security.GeneralSecurityException) {
      throw Failure("message failed to authenticate", e)
    }
  }

  private fun encode(point: ECPoint): ByteArray =
    byteArrayOf(4) + fixed(point.affineX, 32) + fixed(point.affineY, 32)

  /** Big-endian, exactly `size` bytes (BigInteger.toByteArray may add or drop a leading zero). */
  private fun fixed(n: BigInteger, size: Int): ByteArray {
    val bytes = n.toByteArray()
    return when {
      bytes.size == size -> bytes
      bytes.size > size -> bytes.copyOfRange(bytes.size - size, bytes.size)
      else -> ByteArray(size - bytes.size) + bytes
    }
  }

  /** y^2 = x^3 + ax + b (mod p), with x and y in [0, p). */
  private fun onCurve(x: BigInteger, y: BigInteger): Boolean {
    val p = (curve.curve.field as ECFieldFp).p
    if (x.signum() < 0 || y.signum() < 0 || x >= p || y >= p) return false
    val left = y.multiply(y).mod(p)
    val right = x.pow(3).add(curve.curve.a.multiply(x)).add(curve.curve.b).mod(p)
    return left == right
  }
}
