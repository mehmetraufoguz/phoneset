import { Effect, Fiber } from "effect"
import { fromBase64, toBase64, utf8 } from "./bytes"
import { CameraIO, type CameraSession, type CameraStats, type Facing } from "./camera"
import { type Channel, openChannel } from "./channel"
import { Frame, VIDEO_WINDOW } from "./constants"
import { Crypto, equal, protocolOf } from "./crypto"
import { AuthError, ProtocolError } from "./errors"
import {
  type ClientMessage,
  decodeVideoMessage,
  encodeClientMessage,
  type VideoServerMessage
} from "./protocol"
import { SocketFactory } from "./socket"

// Everything about sending the phone's camera lives here and in core/camera.ts. Video has a connection
// of its own (docs/protocol.md, section 8), so none of this touches the audio connection, and a phone
// that never calls `runVideo` behaves exactly as before.

/** What the video connection reports to whoever shows it. */
export interface VideoReport {
  readonly fps: number
  readonly kBps: number
  /** Time from sending a frame to its acknowledgement. */
  readonly ackMs: number | null
  /** Average time to seal a frame and hand it to the socket. */
  readonly sendMs: number | null
  readonly viewers: number
  readonly camera: CameraStats | null
}

export interface VideoTarget {
  readonly url: string
  readonly deviceId: string
  /** The pairing token (raw bytes). */
  readonly token: Uint8Array
  readonly facing: Facing
}

/**
 * How much may be in flight. Frames are dropped at the source, never queued: a frame is only made
 * when fewer than VIDEO_WINDOW.frames are unacknowledged and the bytes still in flight, with one more
 * of the size of the last, stay under VIDEO_WINDOW.bytes.
 */
export class Window {
  private readonly inflight = new Map<number, { at: number; size: number }>()
  private bytes = 0
  private last = 0
  private waiting: Array<() => void> = []

  private get open() {
    return (
      this.inflight.size < VIDEO_WINDOW.frames &&
      (this.inflight.size === 0 || this.bytes + this.last <= VIDEO_WINDOW.bytes)
    )
  }

  /** Completes when another frame may be made. */
  readonly ready = Effect.callback<void>((resume) => {
    if (this.open) return resume(Effect.void)
    const wake = () => resume(Effect.void)
    this.waiting.push(wake)
    return Effect.sync(() => {
      this.waiting = this.waiting.filter((w) => w !== wake)
    })
  })

  sent(seq: number, size: number) {
    this.inflight.set(seq, { at: Date.now(), size })
    this.bytes += size
    this.last = size
  }

  /** The milliseconds the frame took to be acknowledged, or null for one we do not know. */
  acknowledged(seq: number): number | null {
    const frame = this.inflight.get(seq)
    if (frame === undefined) return null
    this.inflight.delete(seq)
    this.bytes -= frame.size
    if (this.open) {
      const wake = this.waiting
      this.waiting = []
      for (const w of wake) w()
    }
    return Date.now() - frame.at
  }
}

const REPORT_EVERY = "1 second"

/**
 * Opens the video connection, authenticates with the pairing token, turns the camera on and sends
 * frames until something fails or the fiber is interrupted. `report` is called about once a second.
 */
export const runVideo = Effect.fn("Video.run")(function* (
  target: VideoTarget,
  report: (r: VideoReport) => Effect.Effect<void>
) {
  const sockets = yield* SocketFactory
  const crypto = yield* Crypto
  const camera = yield* CameraIO
  const { proof } = protocolOf(crypto)

  const channel = yield* openChannel(target.url).pipe(
    Effect.provideService(SocketFactory, sockets),
    Effect.provideService(Crypto, crypto)
  )
  const send = (m: ClientMessage) => channel.send(Frame.control, encodeClientMessage(m))

  // Only a phone that is already paired may open this connection; it proves itself as when reconnecting.
  const id = utf8(target.deviceId)
  yield* send({
    t: "auth",
    deviceId: target.deviceId,
    proof: toBase64(proof(target.token, "phoneset/auth/c", channel.transcript, id))
  })
  const reply = yield* nextMessage(channel)
  if (reply.t === "error") return yield* new AuthError({ code: reply.code, message: reply.message })
  if (reply.t !== "authed")
    return yield* new ProtocolError({ message: "Unexpected reply from the PC." })
  const theirs = yield* Effect.try({
    try: () => fromBase64(reply.proof),
    catch: () => new ProtocolError({ message: "The PC sent malformed data." })
  })
  if (!equal(theirs, proof(target.token, "phoneset/auth/s", channel.transcript, id)))
    return yield* new AuthError({
      code: "bad_server",
      message: "That PC couldn't prove who it is."
    })

  const session: CameraSession = yield* camera.open(target.facing)
  yield* send({ t: "video", on: true, camera: target.facing })
  yield* report({ fps: 0, kBps: 0, ackMs: null, sendMs: null, viewers: 0, camera: null }) // the camera is on

  const window = new Window()
  let viewers = 0
  let sentFrames = 0
  let sentBytes = 0
  let ackTotal = 0
  let acks = 0
  let sendTotal = 0
  let sends = 0
  let since = Date.now()

  const handle = (m: VideoServerMessage): Effect.Effect<void, AuthError | ProtocolError> => {
    switch (m.t) {
      case "vack": {
        const ms = window.acknowledged(m.seq)
        if (ms !== null) {
          ackTotal += ms
          acks++
        }
        return Effect.void
      }
      case "vstats":
        viewers = m.viewers
        return Effect.void
      case "error":
        return Effect.fail(new AuthError({ code: m.code, message: m.message }))
      case "video":
        return m.on
          ? Effect.void
          : Effect.fail(new ProtocolError({ message: m.error ?? "The PC turned the camera off." }))
      default:
        return Effect.void
    }
  }

  const receive = Effect.forever(
    channel.receive.pipe(
      Effect.flatMap(
        (m): Effect.Effect<void, AuthError | ProtocolError> =>
          m.type === Frame.control
            ? decodeVideoMessage(m.body).pipe(Effect.flatMap(handle))
            : Effect.void
      )
    )
  )

  // The next frame is asked for while the last is still being sealed and sent, so the camera side makes
  // it in parallel (a pull that waited for the send to finish let every second picture go by). A frame is
  // counted as in flight the moment it exists, so the window still decides when the next one is made.
  const pull = window.ready.pipe(Effect.andThen(session.next))
  const pump = Effect.gen(function* () {
    let pending = yield* Effect.forkChild(pull)
    while (true) {
      const frame = yield* Fiber.join(pending)
      // Header: seq (u32), time, width, height. The sequence number is the first four bytes.
      const seq = new DataView(frame.buffer, frame.byteOffset, 4).getUint32(0, false)
      window.sent(seq, frame.length)
      sentFrames++
      sentBytes += frame.length
      pending = yield* Effect.forkChild(pull)
      // Let the pull reach the camera before the heavy part (sealing and base64) takes the thread.
      yield* Effect.yieldNow
      const before = Date.now()
      yield* channel.send(Frame.video, frame)
      sendTotal += Date.now() - before
      sends++
    }
  })

  const reporter = Effect.forever(
    Effect.sleep(REPORT_EVERY).pipe(
      Effect.andThen(
        Effect.gen(function* () {
          const now = Date.now()
          const seconds = Math.max(0.001, (now - since) / 1000)
          const stats = yield* session.stats
          yield* report({
            fps: sentFrames / seconds,
            kBps: sentBytes / seconds / 1000,
            ackMs: acks === 0 ? null : ackTotal / acks,
            sendMs: sends === 0 ? null : sendTotal / sends,
            viewers,
            camera: stats
          })
          sentFrames = sentBytes = ackTotal = acks = sendTotal = sends = 0
          since = now
        })
      )
    )
  )

  // The first of the three to fail ends the video and stops the others; interrupting this ends all.
  yield* Effect.all([receive, pump, reporter], { concurrency: "unbounded", discard: true })
})

const nextMessage = (channel: Channel) =>
  channel.receive.pipe(
    Effect.flatMap((m) =>
      m.type === Frame.control
        ? decodeVideoMessage(m.body)
        : Effect.fail(new ProtocolError({ message: "The PC sent a frame before it was ready." }))
    )
  )
