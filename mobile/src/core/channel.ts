import { Effect, type Scope } from "effect"
import { concat } from "./bytes"
import { PROTOCOL_VERSION } from "./constants"
import { deriveKeys, keyPairFromSeed, open, seal, sharedSecret, transcriptOf } from "./crypto"
import { Entropy } from "./entropy"
import { ProtocolError, type TransportError } from "./errors"
import { SocketFactory } from "./socket"

/** An authenticated-encryption layer over a Socket (see docs/protocol.md). */
export interface Channel {
  /** Binds proofs to this handshake. */
  readonly transcript: Uint8Array
  readonly send: (
    type: number,
    ...parts: ReadonlyArray<Uint8Array>
  ) => Effect.Effect<void, TransportError>
  readonly receive: Effect.Effect<Message, TransportError | ProtocolError>
}

export interface Message {
  readonly type: number
  readonly body: Uint8Array
}

/** Opens the socket and runs the X25519 handshake. Closing the scope closes the socket. */
export const openChannel = Effect.fn("Channel.open")(function* (url: string) {
  const sockets = yield* SocketFactory
  const entropy = yield* Entropy

  const socket = yield* sockets.connect(url)

  const me = keyPairFromSeed(entropy.bytes(32))
  yield* socket.send(concat(Uint8Array.of(PROTOCOL_VERSION), me.public))

  const serverPublic = yield* socket.receive
  if (serverPublic.length !== 32) {
    return yield* new ProtocolError({ message: "The PC sent a malformed handshake." })
  }
  const transcript = transcriptOf(me.public, serverPublic)
  const keys = deriveKeys(sharedSecret(me.secret, serverPublic), transcript)

  // Counters are the nonces. The socket is ordered and reliable, so both sides
  // stay in step without sending them.
  let sent = 0
  let received = 0

  const channel: Channel = {
    transcript,
    send: (type, ...parts) =>
      Effect.suspend(() => {
        const sealed = seal(keys.send, sent++, transcript, concat(Uint8Array.of(type), ...parts))
        return socket.send(sealed)
      }),
    receive: Effect.flatMap(socket.receive, (data) =>
      Effect.try({
        try: () => {
          const plain = open(keys.receive, received++, transcript, data)
          return { type: plain[0] ?? 0, body: plain.subarray(1) } satisfies Message
        },
        catch: () => new ProtocolError({ message: "A message from the PC failed to authenticate." })
      })
    )
  }
  return channel
}) satisfies (
  url: string
) => Effect.Effect<Channel, TransportError | ProtocolError, SocketFactory | Entropy | Scope.Scope>
