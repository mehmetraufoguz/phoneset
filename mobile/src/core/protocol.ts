import { Effect, Schema } from "effect"
import { fromUtf8, utf8 } from "./bytes"
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
  speaker: Endpoint
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
export type ReadyMessage = typeof Ready.Type
export type StreamMessage = typeof StreamState.Type
export type StatsMessage = typeof Stats.Type

const decode = Schema.decodeUnknownEffect(ServerMessage)

export const decodeServerMessage = (body: Uint8Array) =>
  Effect.try({
    try: () => JSON.parse(fromUtf8(body)) as unknown,
    catch: () => new ProtocolError({ message: "The PC sent a message that isn't JSON." })
  }).pipe(
    Effect.flatMap(decode),
    Effect.mapError((e) =>
      e instanceof ProtocolError
        ? e
        : new ProtocolError({ message: "The PC sent an unexpected message." })
    )
  )

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

export const encodeClientMessage = (message: ClientMessage): Uint8Array =>
  utf8(JSON.stringify(message))
