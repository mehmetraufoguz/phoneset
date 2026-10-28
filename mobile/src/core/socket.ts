import { Cause, Context, Effect, Layer, Queue, type Scope } from "effect"
import { TransportError } from "./errors"

export interface Socket {
  readonly send: (data: Uint8Array) => Effect.Effect<void, TransportError>
  /** Next binary message; fails with TransportError once the socket is closed. */
  readonly receive: Effect.Effect<Uint8Array, TransportError>
}

export class SocketFactory extends Context.Service<
  SocketFactory,
  {
    readonly connect: (url: string) => Effect.Effect<Socket, TransportError, Scope.Scope>
  }
>()("phoneset/SocketFactory") {
  /** Uses the global WebSocket, which React Native and Node 22+ both provide. */
  static readonly layer = Layer.succeed(SocketFactory, {
    connect: (url: string) => connectWebSocket(url)
  })
}

const OPEN_TIMEOUT = "5 seconds"

const connectWebSocket = Effect.fn("Socket.connect")(function* (url: string) {
  const inbox = yield* Queue.unbounded<Uint8Array, TransportError>()

  const ws = yield* Effect.acquireRelease(
    Effect.sync(() => {
      const socket = new WebSocket(url)
      socket.binaryType = "arraybuffer"
      return socket
    }),
    (socket) =>
      Effect.sync(() => {
        socket.onopen = socket.onmessage = socket.onerror = socket.onclose = null
        try {
          socket.close()
        } catch {
          // already closed
        }
      })
  )

  const fail = (message: string) =>
    Queue.failCauseUnsafe(inbox, Cause.fail(new TransportError({ message })))

  yield* Effect.callback<void, TransportError>((resume) => {
    ws.onopen = () => resume(Effect.void)
    ws.onerror = () =>
      resume(Effect.fail(new TransportError({ message: "Couldn't reach the PC." })))
    ws.onclose = () =>
      resume(Effect.fail(new TransportError({ message: "The PC closed the connection." })))
  }).pipe(
    Effect.timeoutOrElse({
      duration: OPEN_TIMEOUT,
      orElse: () => Effect.fail(new TransportError({ message: "The PC didn't answer in time." }))
    })
  )

  ws.onmessage = (event: MessageEvent) => {
    if (event.data instanceof ArrayBuffer) Queue.offerUnsafe(inbox, new Uint8Array(event.data))
  }
  ws.onerror = () => fail("The connection failed.")
  ws.onclose = () => fail("The connection closed.")

  const socket: Socket = {
    send: (data) =>
      Effect.try({
        try: () => ws.send(data),
        catch: () => new TransportError({ message: "The connection dropped." })
      }),
    receive: Queue.take(inbox)
  }
  return socket
})
