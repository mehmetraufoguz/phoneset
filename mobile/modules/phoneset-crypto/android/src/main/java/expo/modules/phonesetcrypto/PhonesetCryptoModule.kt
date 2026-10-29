package expo.modules.phonesetcrypto

import expo.modules.kotlin.exception.CodedException
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition

/**
 * Exposes PhonesetCryptoCore to JavaScript. Every function is synchronous: the audio path calls
 * seal and open once per 20 ms frame.
 */
class PhonesetCryptoModule : Module() {
  override fun definition() = ModuleDefinition {
    Name("PhonesetCrypto")

    Function("generateKeyPair") { wrap { PhonesetCryptoCore.generateKeyPair() } }

    Function("sharedSecret") { secret: ByteArray, peerPublic: ByteArray ->
      wrap { PhonesetCryptoCore.sharedSecret(secret, peerPublic) }
    }

    Function("hkdf") { ikm: ByteArray, salt: ByteArray, info: ByteArray, length: Int ->
      wrap { PhonesetCryptoCore.hkdf(ikm, salt, info, length) }
    }

    Function("hmac") { key: ByteArray, data: ByteArray ->
      wrap { PhonesetCryptoCore.hmac(key, data) }
    }

    Function("sha256") { data: ByteArray -> wrap { PhonesetCryptoCore.sha256(data) } }

    Function("seal") { key: ByteArray, nonce: ByteArray, aad: ByteArray, plaintext: ByteArray ->
      wrap { PhonesetCryptoCore.seal(key, nonce, aad, plaintext) }
    }

    Function("open") { key: ByteArray, nonce: ByteArray, aad: ByteArray, sealed: ByteArray ->
      wrap { PhonesetCryptoCore.open(key, nonce, aad, sealed) }
    }
  }

  private inline fun wrap(body: () -> ByteArray): ByteArray =
    try {
      body()
    } catch (e: PhonesetCryptoCore.Failure) {
      throw CodedException("ERR_PHONESET_CRYPTO", e.message, e)
    } catch (e: java.security.GeneralSecurityException) {
      throw CodedException("ERR_PHONESET_CRYPTO", e.message, e)
    }
}
