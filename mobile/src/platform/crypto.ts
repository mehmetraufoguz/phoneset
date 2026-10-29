import { Layer } from "effect"
import { requireNativeModule } from "expo"
import { Crypto } from "../core/crypto"

// The native module in modules/phoneset-crypto: CryptoKit on iOS, the Java cryptography
// architecture on Android. A development build made before it was added has to be rebuilt.
interface PhonesetCryptoModule {
  generateKeyPair(): unknown
  sharedSecret(secret: Uint8Array, peerPublic: Uint8Array): unknown
  hkdf(ikm: Uint8Array, salt: Uint8Array, info: Uint8Array, length: number): unknown
  hmac(key: Uint8Array, data: Uint8Array): unknown
  sha256(data: Uint8Array): unknown
  seal(key: Uint8Array, nonce: Uint8Array, aad: Uint8Array, plaintext: Uint8Array): unknown
  open(key: Uint8Array, nonce: Uint8Array, aad: Uint8Array, sealed: Uint8Array): unknown
}

const native = requireNativeModule<PhonesetCryptoModule>("PhonesetCrypto")

/** Whatever the native side hands back, as a Uint8Array. */
const bytes = (value: unknown): Uint8Array =>
  value instanceof Uint8Array
    ? value
    : value instanceof ArrayBuffer
      ? new Uint8Array(value)
      : Uint8Array.from(value as ArrayLike<number>)

/** A view onto part of a larger buffer is copied, so the native side sees exactly these bytes. */
const own = (b: Uint8Array): Uint8Array =>
  b.byteOffset === 0 && b.byteLength === b.buffer.byteLength ? b : b.slice()

export const CryptoLive = Layer.succeed(Crypto, {
  generateKeyPair: () => {
    const pair = bytes(native.generateKeyPair())
    return { secret: pair.slice(0, 32), public: pair.slice(32, 97) }
  },
  sharedSecret: (secret, peerPublic) => bytes(native.sharedSecret(own(secret), own(peerPublic))),
  hkdf: (ikm, salt, info, length) => bytes(native.hkdf(own(ikm), own(salt), own(info), length)),
  hmac: (key, data) => bytes(native.hmac(own(key), own(data))),
  sha256: (data) => bytes(native.sha256(own(data))),
  seal: (key, nonce, aad, plaintext) =>
    bytes(native.seal(own(key), own(nonce), own(aad), own(plaintext))),
  open: (key, nonce, aad, sealed) => bytes(native.open(own(key), own(nonce), own(aad), own(sealed)))
})
