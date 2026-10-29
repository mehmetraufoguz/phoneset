import { Context } from "effect"
import { concat, utf8 } from "./bytes"

// The protocol's cryptography (docs/protocol.md), mirroring pc/internal/secure. Every primitive
// comes from the phone's operating system (CryptoKit on iOS, the Java cryptography architecture
// on Android) through the Crypto service, so the app carries no cryptography of its own.

export interface KeyPair {
  /** 32 bytes. */
  readonly secret: Uint8Array
  /** 65 bytes: an uncompressed P-256 point, 0x04 || X || Y. */
  readonly public: Uint8Array
}

export interface Primitives {
  /** A fresh P-256 key pair. */
  readonly generateKeyPair: () => KeyPair
  /** ECDH: the 32-byte x coordinate of the shared point. Throws on a key that is not on the curve. */
  readonly sharedSecret: (secret: Uint8Array, peerPublic: Uint8Array) => Uint8Array
  /** HKDF-SHA-256. */
  readonly hkdf: (ikm: Uint8Array, salt: Uint8Array, info: Uint8Array, length: number) => Uint8Array
  /** HMAC-SHA-256. */
  readonly hmac: (key: Uint8Array, data: Uint8Array) => Uint8Array
  readonly sha256: (data: Uint8Array) => Uint8Array
  /** AES-256-GCM with a 12-byte nonce: ciphertext followed by the 16-byte tag. */
  readonly seal: (
    key: Uint8Array,
    nonce: Uint8Array,
    aad: Uint8Array,
    plaintext: Uint8Array
  ) => Uint8Array
  /** Throws if the message does not authenticate. */
  readonly open: (
    key: Uint8Array,
    nonce: Uint8Array,
    aad: Uint8Array,
    sealed: Uint8Array
  ) => Uint8Array
}

/** The platform's cryptography. Provided by the platform. */
export class Crypto extends Context.Service<Crypto, Primitives>()("phoneset/Crypto") {}

export interface SessionKeys {
  /** Client to server. */
  readonly send: Uint8Array
  /** Server to client. */
  readonly receive: Uint8Array
}

/** 4 zero bytes then the counter as a big-endian u64, as on the PC. */
const nonce = (counter: number): Uint8Array => {
  const n = new Uint8Array(12)
  const view = new DataView(n.buffer)
  view.setUint32(4, Math.floor(counter / 2 ** 32), false)
  view.setUint32(8, counter >>> 0, false)
  return n
}

/** The protocol's operations, built on a set of primitives. */
export const protocolOf = (p: Primitives) => ({
  /** Binds every proof to this handshake so none can be replayed on another. */
  transcriptOf: (clientPublic: Uint8Array, serverPublic: Uint8Array): Uint8Array =>
    p.sha256(concat(utf8("phoneset/v2"), clientPublic, serverPublic)),

  deriveKeys: (shared: Uint8Array, transcript: Uint8Array): SessionKeys => {
    const okm = p.hkdf(shared, transcript, utf8("phoneset/v2/keys"), 64)
    return { send: okm.slice(0, 32), receive: okm.slice(32, 64) }
  },

  seal: (key: Uint8Array, counter: number, aad: Uint8Array, plaintext: Uint8Array): Uint8Array =>
    p.seal(key, nonce(counter), aad, plaintext),

  /** Throws if the message does not authenticate. */
  open: (key: Uint8Array, counter: number, aad: Uint8Array, ciphertext: Uint8Array): Uint8Array =>
    p.open(key, nonce(counter), aad, ciphertext),

  proof: (
    key: Uint8Array,
    label: string,
    transcript: Uint8Array,
    ...parts: ReadonlyArray<Uint8Array>
  ): Uint8Array => p.hmac(key, concat(utf8(label), transcript, ...parts)),

  /** Key used for the 6-digit code path of pairing. */
  codeKey: (code: string): Uint8Array => p.sha256(utf8(`phoneset/code/${code}`))
})

export const equal = (a: Uint8Array, b: Uint8Array): boolean => {
  if (a.length !== b.length) return false
  let diff = 0
  for (let i = 0; i < a.length; i++) diff |= (a[i] ?? 0) ^ (b[i] ?? 0)
  return diff === 0
}
