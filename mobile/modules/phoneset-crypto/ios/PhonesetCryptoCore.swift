import CryptoKit
import Foundation

/// The primitives of the phoneset protocol (docs/protocol.md), all provided by CryptoKit, so the
/// app carries no cryptography of its own. No Expo types in this file: it is plain Swift.
enum PhonesetCryptoCore {
  struct Failure: Error {
    let reason: String
  }

  static let secretLength = 32
  static let publicLength = 65
  static let keyLength = 32
  static let nonceLength = 12
  static let tagLength = 16

  /// secret (32 bytes) || public key (65 bytes, uncompressed X9.63).
  static func generateKeyPair() -> Data {
    let key = P256.KeyAgreement.PrivateKey()
    return key.rawRepresentation + key.publicKey.x963Representation
  }

  /// The raw ECDH output: the 32-byte x coordinate of the shared point.
  static func sharedSecret(secret: Data, peerPublic: Data) throws -> Data {
    guard secret.count == secretLength, peerPublic.count == publicLength else {
      throw Failure(reason: "malformed key")
    }
    do {
      let privateKey = try P256.KeyAgreement.PrivateKey(rawRepresentation: secret)
      // Rejects a point that is not on the curve.
      let publicKey = try P256.KeyAgreement.PublicKey(x963Representation: peerPublic)
      let shared = try privateKey.sharedSecretFromKeyAgreement(with: publicKey)
      return shared.withUnsafeBytes { Data($0) }
    } catch {
      throw Failure(reason: "key agreement failed")
    }
  }

  static func hkdf(ikm: Data, salt: Data, info: Data, length: Int) throws -> Data {
    guard length > 0, length <= 255 * 32 else { throw Failure(reason: "bad output length") }
    let key = HKDF<SHA256>.deriveKey(
      inputKeyMaterial: SymmetricKey(data: ikm), salt: salt, info: info, outputByteCount: length)
    return key.withUnsafeBytes { Data($0) }
  }

  static func hmac(key: Data, data: Data) -> Data {
    Data(HMAC<SHA256>.authenticationCode(for: data, using: SymmetricKey(data: key)))
  }

  static func sha256(_ data: Data) -> Data {
    Data(SHA256.hash(data: data))
  }

  /// ciphertext || 16-byte tag.
  static func seal(key: Data, nonce: Data, aad: Data, plaintext: Data) throws -> Data {
    guard key.count == keyLength, nonce.count == nonceLength else {
      throw Failure(reason: "bad key or nonce length")
    }
    do {
      let box = try AES.GCM.seal(
        plaintext, using: SymmetricKey(data: key), nonce: try AES.GCM.Nonce(data: nonce),
        authenticating: aad)
      return box.ciphertext + box.tag
    } catch {
      throw Failure(reason: "encryption failed")
    }
  }

  static func open(key: Data, nonce: Data, aad: Data, sealed: Data) throws -> Data {
    guard key.count == keyLength, nonce.count == nonceLength, sealed.count >= tagLength else {
      throw Failure(reason: "bad key, nonce or message length")
    }
    do {
      let box = try AES.GCM.SealedBox(
        nonce: try AES.GCM.Nonce(data: nonce),
        ciphertext: sealed.prefix(sealed.count - tagLength),
        tag: sealed.suffix(tagLength))
      return try AES.GCM.open(box, using: SymmetricKey(data: key), authenticating: aad)
    } catch {
      throw Failure(reason: "message failed to authenticate")
    }
  }
}
