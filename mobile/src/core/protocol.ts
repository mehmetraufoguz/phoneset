import { Effect, Schema } from "effect"
import { fromUtf8, utf8 } from "./bytes"
import type { VideoMode } from "./camera"
import { ProtocolError } from "./errors"

// Control messages are JSON inside the encrypted channel.

const Endpoint = Schema.Struct({
  available: Schema.Boolean,
  device: Schema.optionalKey(Schema.String),
  reason: Schema.optionalKey(Schema.String)
})
export type Endpoint = typeof Endpoint.Type

const Ready = Schema.Struct({
  t: Schema.Literal("ready"),
  audio: Schema.Struct({
    rate: Schema.Number,
    frameMs: Schema.Number,
    micChannels: Schema.Number,
    speakerChannels: Schema.Number
  }),
  mic: Endpoint,
  speaker: Endpoint,
  /** Absent on a PC that has no video. */
  video: Schema.optionalKey(
    Schema.Struct({
      available: Schema.Boolean,
      reason: Schema.optionalKey(Schema.String),
      path: Schema.optionalKey(Schema.String),
      maxFrameBytes: Schema.optionalKey(Schema.Number),
      /** The codecs the PC accepts, and optional extras it supports. */
      codecs: Schema.optionalKey(Schema.Array(Schema.String)),
      features: Schema.optionalKey(Schema.Array(Schema.String))
    })
  )
})

const StreamState = Schema.Struct({
  t: Schema.Literal("stream"),
  mic: Schema.optionalKey(Schema.Boolean),
  speaker: Schema.optionalKey(Schema.Boolean),
  micError: Schema.optionalKey(Schema.String),
  speakerError: Schema.optionalKey(Schema.String)
})

const Pong = Schema.Struct({ t: Schema.Literal("pong"), n: Schema.Number, c: Schema.Number })

const Stats = Schema.Struct({
  t: Schema.Literal("stats"),
  mic: Schema.Struct({ bufferMs: Schema.Number, underruns: Schema.Number, drops: Schema.Number }),
  speaker: Schema.Struct({ sent: Schema.Number, dropped: Schema.Number })
})

const Rejected = Schema.Struct({
  t: Schema.Literal("error"),
  code: Schema.String,
  message: Schema.String
})

const Paired = Schema.Struct({
  t: Schema.Literal("paired"),
  pcId: Schema.String,
  pcName: Schema.String,
  token: Schema.String,
  proof: Schema.String
})

const Authed = Schema.Struct({
  t: Schema.Literal("authed"),
  pcId: Schema.String,
  pcName: Schema.String,
  proof: Schema.String
})

export const ServerMessage = Schema.Union([
  Ready,
  StreamState,
  Pong,
  Stats,
  Rejected,
  Paired,
  Authed
])
export type ServerMessage = typeof ServerMessage.Type

// The video connection has its own, smaller set of messages. It is decoded apart from the one above so
// that nothing about video can reach the audio connection's parser (an unknown tag there ends it).

const Mode = Schema.Struct({
  codec: Schema.String,
  width: Schema.Number,
  height: Schema.Number,
  fps: Schema.Number
})

const VideoState = Schema.Struct({
  t: Schema.Literal("video"),
  on: Schema.Boolean,
  camera: Schema.optionalKey(Schema.String),
  mode: Schema.optionalKey(Mode),
  error: Schema.optionalKey(Schema.String)
})
const VideoAck = Schema.Struct({ t: Schema.Literal("vack"), seq: Schema.Number })
const VideoStats = Schema.Struct({
  t: Schema.Literal("vstats"),
  frames: Schema.Number,
  bytes: Schema.Number,
  viewers: Schema.Number
})

export const VideoServerMessage = Schema.Union([Authed, Rejected, VideoState, VideoAck, VideoStats])
export type VideoServerMessage = typeof VideoServerMessage.Type
export type ReadyMessage = typeof Ready.Type
export type StreamMessage = typeof StreamState.Type
export type StatsMessage = typeof Stats.Type
export type VideoOffer = NonNullable<ReadyMessage["video"]>

const parseJson = (body: Uint8Array) =>
  Effect.try({
    try: () => JSON.parse(fromUtf8(body)) as unknown,
    catch: () => new ProtocolError({ message: "The PC sent a message that isn't JSON." })
  })

const unexpected = (e: unknown) =>
  e instanceof ProtocolError
    ? e
    : new ProtocolError({ message: "The PC sent an unexpected message." })

const decode = Schema.decodeUnknownEffect(ServerMessage)
const decodeVideo = Schema.decodeUnknownEffect(VideoServerMessage)

export const decodeServerMessage = (body: Uint8Array) =>
  parseJson(body).pipe(Effect.flatMap(decode), Effect.mapError(unexpected))

export const decodeVideoMessage = (body: Uint8Array) =>
  parseJson(body).pipe(Effect.flatMap(decodeVideo), Effect.mapError(unexpected))

// Phone to PC. Plain objects: we build these, so there is nothing to validate.

export type ClientMessage =
  | {
      readonly t: "pair"
      readonly deviceId: string
      readonly name: string
      readonly platform: string
      readonly method: "qr" | "code"
      readonly proof: string
    }
  | { readonly t: "auth"; readonly deviceId: string; readonly proof: string }
  | { readonly t: "stream"; readonly mic?: boolean; readonly speaker?: boolean }
  | { readonly t: "ping"; readonly n: number; readonly c: number }
  | { readonly t: "bye" }
  // Only on the video connection:
  | {
      readonly t: "video"
      readonly on: boolean
      readonly camera?: "back" | "front"
      /** What the phone is about to send, and everything it is willing to send. */
      readonly mode?: VideoMode
      readonly modes?: ReadonlyArray<VideoMode>
      /** Extras this phone supports; "keyreq": it makes a key frame when the PC asks (docs/protocol.md, 8.3). */
      readonly features?: ReadonlyArray<string>
    }

export const encodeClientMessage = (message: ClientMessage): Uint8Array =>
  utf8(JSON.stringify(message))
