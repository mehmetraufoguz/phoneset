import { chacha20poly1305 } from "@noble/ciphers/chacha.js"
import { x25519 } from "@noble/curves/ed25519.js"
import { hkdf } from "@noble/hashes/hkdf.js"
import { hmac } from "@noble/hashes/hmac.js"
import { sha256 } from "@noble/hashes/sha2.js"
import { concat, utf8 } from "./bytes"

// Pure functions mirroring pc/internal/secure. The only effect (randomness) is
// supplied by the caller through the Entropy service.

export interface KeyPair {
  readonly secret: Uint8Array
  readonly public: Uint8Array
}

export const keyPairFromSeed = (seed: Uint8Array): KeyPair => ({
  secret: seed,
  public: x25519.getPublicKey(seed)
})

export const sharedSecret = (secret: Uint8Array, peerPublic: Uint8Array): Uint8Array =>
  x25519.getSharedSecret(secret, peerPublic)

/** Binds every proof to this handshake so none can be replayed on another. */
export const transcriptOf = (clientPublic: Uint8Array, serverPublic: Uint8Array): Uint8Array =>
  sha256(concat(utf8("phoneset/v1"), clientPublic, serverPublic))

export interface SessionKeys {
  /** Client to server. */
  readonly send: Uint8Array
  /** Server to client. */
  readonly receive: Uint8Array
}

export const deriveKeys = (shared: Uint8Array, transcript: Uint8Array): SessionKeys => {
  const okm = hkdf(sha256, shared, transcript, utf8("phoneset/v1/keys"), 64)
  return { send: okm.slice(0, 32), receive: okm.slice(32, 64) }
}

/** 4 zero bytes then the counter as a big-endian u64, as on the PC. */
const nonce = (counter: number): Uint8Array => {
  const n = new Uint8Array(12)
  const view = new DataView(n.buffer)
  view.setUint32(4, Math.floor(counter / 2 ** 32), false)
  view.setUint32(8, counter >>> 0, false)
  return n
}

export const seal = (
  key: Uint8Array,
  counter: number,
  aad: Uint8Array,
  plaintext: Uint8Array
): Uint8Array => chacha20poly1305(key, nonce(counter), aad).encrypt(plaintext)

/** Throws if the message does not authenticate. */
export const open = (
  key: Uint8Array,
  counter: number,
  aad: Uint8Array,
  ciphertext: Uint8Array
): Uint8Array => chacha20poly1305(key, nonce(counter), aad).decrypt(ciphertext)

export const proof = (
  key: Uint8Array,
  label: string,
  transcript: Uint8Array,
  ...parts: ReadonlyArray<Uint8Array>
): Uint8Array => hmac(sha256, key, concat(utf8(label), transcript, ...parts))

/** Key used for the 6-digit code path of pairing. */
export const codeKey = (code: string): Uint8Array => sha256(utf8(`phoneset/code/${code}`))

export const equal = (a: Uint8Array, b: Uint8Array): boolean => {
  if (a.length !== b.length) return false
  let diff = 0
  for (let i = 0; i < a.length; i++) diff |= (a[i] ?? 0) ^ (b[i] ?? 0)
  return diff === 0
}
