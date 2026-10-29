import ExpoModulesCore

private final class CryptoException: GenericException<String> {
  override var reason: String {
    param
  }
}

/// Exposes PhonesetCryptoCore to JavaScript. Every function is synchronous: the audio path calls
/// seal and open once per 20 ms frame.
public class PhonesetCryptoModule: Module {
  public func definition() -> ModuleDefinition {
    Name("PhonesetCrypto")

    Function("generateKeyPair") { () -> Data in
      PhonesetCryptoCore.generateKeyPair()
    }

    Function("sharedSecret") { (secret: Data, peerPublic: Data) throws -> Data in
      try Self.wrap { try PhonesetCryptoCore.sharedSecret(secret: secret, peerPublic: peerPublic) }
    }

    Function("hkdf") { (ikm: Data, salt: Data, info: Data, length: Int) throws -> Data in
      try Self.wrap {
        try PhonesetCryptoCore.hkdf(ikm: ikm, salt: salt, info: info, length: length)
      }
    }

    Function("hmac") { (key: Data, data: Data) -> Data in
      PhonesetCryptoCore.hmac(key: key, data: data)
    }

    Function("sha256") { (data: Data) -> Data in
      PhonesetCryptoCore.sha256(data)
    }

    Function("seal") { (key: Data, nonce: Data, aad: Data, plaintext: Data) throws -> Data in
      try Self.wrap {
        try PhonesetCryptoCore.seal(key: key, nonce: nonce, aad: aad, plaintext: plaintext)
      }
    }

    Function("open") { (key: Data, nonce: Data, aad: Data, sealed: Data) throws -> Data in
      try Self.wrap {
        try PhonesetCryptoCore.open(key: key, nonce: nonce, aad: aad, sealed: sealed)
      }
    }
  }

  private static func wrap(_ body: () throws -> Data) throws -> Data {
    do {
      return try body()
    } catch let failure as PhonesetCryptoCore.Failure {
      throw CryptoException(failure.reason)
    }
  }
}
