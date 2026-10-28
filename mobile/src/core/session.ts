import {
  Context,
  Effect,
  FiberHandle,
  Layer,
  Option,
  Queue,
  Ref,
  Stream,
  SubscriptionRef
} from "effect"
import { AudioIO, type Player } from "./audio"
import { fromBase64, toBase64, u32be, utf8 } from "./bytes"
import { type Channel, openChannel } from "./channel"
import { AUDIO_HEADER_LEN, Frame, WS_PATH } from "./constants"
import { codeKey, equal, proof } from "./crypto"
import { Discovery, firstReachable } from "./discovery"
import { Entropy } from "./entropy"
import {
  type AudioError,
  AuthError,
  type ConnectError,
  ProtocolError,
  TransportError
} from "./errors"
import type { PairLink } from "./link"
import { rms } from "./pcm"
import {
  type ClientMessage,
  decodeServerMessage,
  encodeClientMessage,
  type ReadyMessage,
  type ServerMessage,
  type StatsMessage
} from "./protocol"
import { SocketFactory } from "./socket"
import { type PairedPc, PairedPcs, type PhoneIdentity } from "./storage"

export interface StreamStatus {
  readonly on: boolean
  /** A request is in flight. */
  readonly busy: boolean
  /** The PC can serve this stream (e.g. a virtual cable is installed). */
  readonly available: boolean
  readonly device: string | null
  readonly error: string | null
}

export type SessionState =
  | { readonly _tag: "Idle" }
  | { readonly _tag: "Connecting"; readonly pcName: string; readonly step: string }
  | {
      readonly _tag: "Connected"
      readonly pc: { readonly id: string; readonly name: string }
      readonly mic: StreamStatus
      readonly speaker: StreamStatus
      readonly rttMs: number | null
      readonly micLevel: number
      readonly stats: StatsMessage | null
    }
  | { readonly _tag: "Failed"; readonly message: string; readonly code: string | null }

/** How to reach and prove ourselves to a PC. */
export type Target =
  | { readonly kind: "paired"; readonly pc: PairedPc }
  | { readonly kind: "qr"; readonly link: PairLink }
  | {
      readonly kind: "code"
      readonly code: string
      readonly name: string
      readonly hosts: ReadonlyArray<string>
      readonly port: number
      /** Known when the PC was found by discovery; unknown for a typed-in address. */
      readonly pcId?: string
    }

interface StreamRequest {
  readonly mic?: boolean
  readonly speaker?: boolean
}

interface Route {
  readonly host: string
  readonly port: number
  readonly name: string
}

const MAX_RECONNECTS = 6
const PING_EVERY = "2 seconds"
const HANDSHAKE_TIMEOUT = "10 seconds"
const LEVEL_EVERY_FRAMES = 5

const idle: SessionState = { _tag: "Idle" }

const nameOf = (t: Target): string =>
  t.kind === "paired" ? t.pc.name : t.kind === "qr" ? t.link.name : t.name

export class Session extends Context.Service<
  Session,
  {
    readonly state: SubscriptionRef.SubscriptionRef<SessionState>
    /** Connects (and pairs, for qr/code targets), replacing any current connection. */
    readonly start: (target: Target) => Effect.Effect<void>
    /** Starts over with the last target; after pairing that is the paired PC. */
    readonly retry: Effect.Effect<void>
    readonly stop: Effect.Effect<void>
    readonly setMic: (on: boolean) => Effect.Effect<void>
    readonly setSpeaker: (on: boolean) => Effect.Effect<void>
    /** Takes effect the next time a stream starts. */
    readonly setVoiceProcessing: (on: boolean) => Effect.Effect<void>
  }
>()("phoneset/Session") {
  static readonly layer = Layer.effect(Session, makeSession())
}

function makeSession() {
  return Effect.gen(function* () {
    const pcs = yield* PairedPcs
    const audio = yield* AudioIO
    const discovery = yield* Discovery
    const sockets = yield* SocketFactory
    const entropy = yield* Entropy

    const state = yield* SubscriptionRef.make<SessionState>(idle)
    const commands = yield* Queue.unbounded<StreamRequest>()
    const connection = yield* FiberHandle.make<void, never>()
    const lastTarget = yield* Ref.make<Target | null>(null)
    const desired = yield* Ref.make<Required<StreamRequest>>({ mic: false, speaker: false })
    const voiceProcessing = yield* Ref.make(false)

    const setState = (s: SessionState) => SubscriptionRef.set(state, s)
    const whenConnected = (
      f: (
        s: Extract<SessionState, { _tag: "Connected" }>
      ) => Extract<SessionState, { _tag: "Connected" }>
    ) => SubscriptionRef.update(state, (s) => (s._tag === "Connected" ? f(s) : s))

    const connecting = (target: Target, step: string) =>
      setState({ _tag: "Connecting", pcName: nameOf(target), step })

    // ---------------------------------------------------------------- routing

    const rediscover = (pc: PairedPc) =>
      discovery.scan.pipe(
        Stream.flatMap((found) => Stream.fromIterable(found.filter((d) => d.id === pc.id))),
        Stream.runHead,
        Effect.timeout("6 seconds"),
        Effect.flatMap(
          Option.match({
            onNone: () => Effect.fail(new TransportError({ message: "not found" })),
            onSome: (d) =>
              firstReachable(d.hosts, d.port, pc.id).pipe(
                Effect.map((r) => ({ ...r, port: d.port }))
              )
          })
        ),
        Effect.mapError(
          () => new TransportError({ message: `Couldn't find ${pc.name} on this Wi-Fi network.` })
        )
      )

    const resolve = (target: Target): Effect.Effect<Route, TransportError> => {
      switch (target.kind) {
        case "qr":
          return firstReachable(target.link.hosts, target.link.port, target.link.id).pipe(
            Effect.map((r) => ({ host: r.host, port: target.link.port, name: target.link.name }))
          )
        case "code":
          return firstReachable(target.hosts, target.port, target.pcId).pipe(
            Effect.map((r) => ({ host: r.host, port: target.port, name: r.info.name }))
          )
        case "paired":
          return firstReachable(target.pc.hosts, target.pc.port, target.pc.id).pipe(
            Effect.map((r) => ({ host: r.host, port: target.pc.port, name: r.info.name })),
            Effect.catch(() =>
              rediscover(target.pc).pipe(
                Effect.map((r) => ({ host: r.host, port: r.port, name: r.info.name }))
              )
            )
          )
      }
    }

    // --------------------------------------------------------- authentication

    const sendControl = (channel: Channel, message: ClientMessage) =>
      channel.send(Frame.control, encodeClientMessage(message))

    const nextControl = (channel: Channel) =>
      channel.receive.pipe(
        Effect.flatMap((m) =>
          m.type === Frame.control
            ? decodeServerMessage(m.body)
            : Effect.fail(new ProtocolError({ message: "The PC sent audio before it was ready." }))
        )
      )

    const decode64 = (value: string) =>
      Effect.try({
        try: () => fromBase64(value),
        catch: () => new ProtocolError({ message: "The PC sent malformed data." })
      })

    const rejected = (m: Extract<ServerMessage, { t: "error" }>) =>
      new AuthError({ code: m.code, message: m.message })

    const authenticate = (channel: Channel, target: Target, me: PhoneIdentity, route: Route) =>
      Effect.gen(function* () {
        const id = utf8(me.deviceId)
        const badServer = new AuthError({
          code: "bad_server",
          message: "That PC couldn't prove who it is. Pair again from the PC."
        })

        if (target.kind === "paired") {
          const token = yield* decode64(target.pc.token)
          yield* sendControl(channel, {
            t: "auth",
            deviceId: me.deviceId,
            proof: toBase64(proof(token, "phoneset/auth/c", channel.transcript, id))
          })
          const reply = yield* nextControl(channel)
          if (reply.t === "error") return yield* rejected(reply)
          if (reply.t !== "authed")
            return yield* new ProtocolError({ message: "Unexpected reply from the PC." })
          const theirs = yield* decode64(reply.proof)
          if (!equal(theirs, proof(token, "phoneset/auth/s", channel.transcript, id)))
            return yield* badServer
          return {
            ...target.pc,
            name: reply.pcName,
            hosts: [route.host, ...target.pc.hosts.filter((h) => h !== route.host)]
          } satisfies PairedPc
        }

        const key = target.kind === "qr" ? target.link.secret : codeKey(target.code)
        yield* sendControl(channel, {
          t: "pair",
          deviceId: me.deviceId,
          name: me.name,
          platform: me.platform,
          method: target.kind,
          proof: toBase64(proof(key, "phoneset/pair/c", channel.transcript, id))
        })
        const reply = yield* nextControl(channel)
        if (reply.t === "error") return yield* rejected(reply)
        if (reply.t !== "paired")
          return yield* new ProtocolError({ message: "Unexpected reply from the PC." })
        const token = yield* decode64(reply.token)
        const theirs = yield* decode64(reply.proof)
        if (!equal(theirs, proof(key, "phoneset/pair/s", channel.transcript, id, token)))
          return yield* badServer
        const known = target.kind === "qr" ? target.link.hosts : target.hosts
        return {
          id: reply.pcId,
          name: reply.pcName,
          token: reply.token,
          hosts: [route.host, ...known.filter((h) => h !== route.host)],
          port: route.port,
          pairedAt: Date.now()
        } satisfies PairedPc
      }).pipe(
        Effect.timeoutOrElse({
          duration: HANDSHAKE_TIMEOUT,
          orElse: () =>
            Effect.fail(new TransportError({ message: "The PC didn't answer in time." }))
        })
      )

    const awaitReady = (channel: Channel) =>
      Effect.gen(function* () {
        const m = yield* nextControl(channel)
        if (m.t === "error") return yield* rejected(m)
        if (m.t !== "ready")
          return yield* new ProtocolError({ message: "The PC didn't send its audio setup." })
        return m
      })

    // --------------------------------------------------------------- streaming

    const toStatus = (e: ReadyMessage["mic"]): StreamStatus => ({
      on: false,
      busy: false,
      available: e.available,
      device: e.device ?? null,
      error: e.available ? null : (e.reason ?? null)
    })

    const serve = (channel: Channel, pc: PairedPc, ready: ReadyMessage) =>
      Effect.gen(function* () {
        const micFiber = yield* FiberHandle.make<void, never>()
        const playerFiber = yield* FiberHandle.make<void, never>()
        const player = yield* Ref.make<Player | null>(null)
        let micSeq = 0
        let pingN = 0

        yield* setState({
          _tag: "Connected",
          pc: { id: pc.id, name: pc.name },
          mic: toStatus(ready.mic),
          speaker: toStatus(ready.speaker),
          rttMs: null,
          micLevel: 0,
          stats: null
        })

        const setMicStatus = (patch: Partial<StreamStatus>) =>
          whenConnected((s) => ({ ...s, mic: { ...s.mic, ...patch } }))
        const setSpeakerStatus = (patch: Partial<StreamStatus>) =>
          whenConnected((s) => ({ ...s, speaker: { ...s.speaker, ...patch } }))

        const audioFailure = (which: "mic" | "speaker") => (e: AudioError) =>
          Effect.gen(function* () {
            const patch = { on: false, busy: false, error: e.message }
            yield* which === "mic" ? setMicStatus(patch) : setSpeakerStatus(patch)
            yield* Ref.update(desired, (d) => ({ ...d, [which]: false }))
            yield* sendControl(channel, { t: "stream", [which]: false }).pipe(Effect.ignore)
          })

        const micPump = audio.mic.pipe(
          Stream.runForEach((frame) =>
            Effect.gen(function* () {
              const n = micSeq++
              yield* channel.send(Frame.mic, u32be(n), frame)
              if (n % LEVEL_EVERY_FRAMES === 0) {
                const level = rms(frame)
                yield* whenConnected((s) => ({ ...s, micLevel: level }))
              }
            })
          ),
          Effect.catch((e) => (e._tag === "AudioError" ? audioFailure("mic")(e) : Effect.void)),
          Effect.andThen(whenConnected((s) => ({ ...s, micLevel: 0 })))
        )

        const playback = Effect.scoped(
          Effect.gen(function* () {
            const p = yield* audio.openPlayer
            yield* Ref.set(player, p)
            return yield* Effect.never
          })
        ).pipe(Effect.catch(audioFailure("speaker")), Effect.ensuring(Ref.set(player, null)))

        const applyStream = (m: Extract<ServerMessage, { t: "stream" }>) =>
          Effect.gen(function* () {
            if (m.mic !== undefined || m.micError !== undefined) {
              yield* setMicStatus({ on: m.mic ?? false, busy: false, error: m.micError ?? null })
              if (m.mic === true) yield* FiberHandle.run(micFiber, micPump, { onlyIfMissing: true })
              else if (m.mic === false) yield* FiberHandle.clear(micFiber)
            }
            if (m.speaker !== undefined || m.speakerError !== undefined) {
              yield* setSpeakerStatus({
                on: m.speaker ?? false,
                busy: false,
                error: m.speakerError ?? null
              })
              if (m.speaker === true)
                yield* FiberHandle.run(playerFiber, playback, { onlyIfMissing: true })
              else if (m.speaker === false) yield* FiberHandle.clear(playerFiber)
            }
          })

        const handle = (m: ServerMessage) => {
          switch (m.t) {
            case "stream":
              return applyStream(m)
            case "pong":
              return whenConnected((s) => ({ ...s, rttMs: Math.max(0, Date.now() - m.c) }))
            case "stats":
              return whenConnected((s) => ({ ...s, stats: m }))
            case "error":
              return Effect.fail(rejected(m))
            default:
              return Effect.void
          }
        }

        const request = (req: StreamRequest) =>
          Effect.gen(function* () {
            if (req.mic !== undefined) {
              if (req.mic) {
                const permitted = yield* audio.ensureMicPermission.pipe(
                  Effect.as(true),
                  Effect.catch((e) =>
                    setMicStatus({ busy: false, error: e.message }).pipe(Effect.as(false))
                  )
                )
                if (!permitted) return
              }
              yield* setMicStatus({ busy: true, error: null })
            }
            if (req.speaker !== undefined) yield* setSpeakerStatus({ busy: true, error: null })
            yield* sendControl(channel, { t: "stream", ...req })
          })

        // Streams the user had on before a reconnect come back on their own.
        const wanted = yield* Ref.get(desired)
        if (wanted.mic || wanted.speaker) {
          yield* request({
            ...(wanted.mic ? { mic: true } : {}),
            ...(wanted.speaker ? { speaker: true } : {})
          })
        }

        yield* Queue.clear(commands)
        yield* Effect.forkChild(
          Effect.forever(Queue.take(commands).pipe(Effect.flatMap(request))).pipe(Effect.ignore)
        )
        yield* Effect.forkChild(
          Effect.forever(
            Effect.sleep(PING_EVERY).pipe(
              Effect.andThen(
                Effect.suspend(() => sendControl(channel, { t: "ping", n: pingN++, c: Date.now() }))
              )
            )
          ).pipe(Effect.ignore)
        )

        return yield* Effect.forever(
          channel.receive.pipe(
            Effect.flatMap((message) => {
              if (message.type === Frame.control) {
                return decodeServerMessage(message.body).pipe(Effect.flatMap(handle))
              }
              if (message.type === Frame.speaker && message.body.length > AUDIO_HEADER_LEN) {
                const pcm = message.body.subarray(AUDIO_HEADER_LEN)
                return Ref.get(player).pipe(
                  Effect.flatMap((p) => (p === null ? Effect.void : p.write(pcm)))
                )
              }
              return Effect.void
            })
          )
        )
      })

    // -------------------------------------------------------------- connection

    const connectOnce = (
      targetRef: Ref.Ref<Target>,
      connectedOnce: Ref.Ref<boolean>
    ): Effect.Effect<never, ConnectError> =>
      Effect.scoped(
        Effect.gen(function* () {
          const target = yield* Ref.get(targetRef)
          yield* connecting(
            target,
            target.kind === "paired" ? "Looking for your PC" : "Reaching your PC"
          )
          const route = yield* resolve(target)

          yield* connecting(target, "Connecting securely")
          const channel = yield* openChannel(`ws://${route.host}:${route.port}${WS_PATH}`).pipe(
            Effect.provideService(SocketFactory, sockets),
            Effect.provideService(Entropy, entropy)
          )
          yield* Effect.addFinalizer(() => sendControl(channel, { t: "bye" }).pipe(Effect.ignore))

          const me = yield* pcs.identity
          const pc = yield* authenticate(channel, target, me, route)
          yield* pcs.save(pc)
          // A QR secret and a code work once; from here on we are an ordinary paired phone.
          yield* Ref.set(targetRef, { kind: "paired", pc })
          yield* Ref.set(lastTarget, { kind: "paired", pc })

          const ready = yield* awaitReady(channel)
          yield* audio.configure({ voiceProcessing: yield* Ref.get(voiceProcessing) })
          yield* Ref.set(connectedOnce, true)
          return yield* serve(channel, pc, ready)
        })
      )

    const supervise = (
      targetRef: Ref.Ref<Target>,
      connectedOnce: Ref.Ref<boolean>,
      attempt: number
    ): Effect.Effect<void> =>
      connectOnce(targetRef, connectedOnce).pipe(
        Effect.catch((error: ConnectError) =>
          Effect.gen(function* () {
            const target = yield* Ref.get(targetRef)
            const dropped = error._tag === "TransportError" || error._tag === "ProtocolError"
            if (dropped && (yield* Ref.get(connectedOnce)) && attempt < MAX_RECONNECTS) {
              yield* connecting(
                target,
                `Connection lost. Reconnecting (${attempt + 1}/${MAX_RECONNECTS})`
              )
              yield* Effect.sleep(`${1500 * (attempt + 1)} millis`)
              return yield* supervise(targetRef, connectedOnce, attempt + 1)
            }
            yield* setState({
              _tag: "Failed",
              message: error.message,
              code: error._tag === "AuthError" ? error.code : null
            })
          })
        )
      )

    const start = (target: Target): Effect.Effect<void> =>
      Effect.gen(function* () {
        yield* Ref.set(lastTarget, target)
        const targetRef = yield* Ref.make(target)
        const connectedOnce = yield* Ref.make(false)
        yield* Ref.set(desired, { mic: false, speaker: false })
        yield* connecting(target, "Starting")
        yield* FiberHandle.run(connection, supervise(targetRef, connectedOnce, 0))
      })

    const retry = Ref.get(lastTarget).pipe(
      Effect.flatMap((target) => (target === null ? Effect.void : start(target)))
    )

    const stop = Effect.gen(function* () {
      yield* FiberHandle.clear(connection)
      yield* setState(idle)
    })

    const request = (req: StreamRequest) =>
      Ref.update(desired, (d) => ({ ...d, ...req })).pipe(
        Effect.andThen(Queue.offer(commands, req))
      )

    return Session.of({
      state,
      start,
      retry,
      stop,
      setMic: (on) => request({ mic: on }).pipe(Effect.asVoid),
      setSpeaker: (on) => request({ speaker: on }).pipe(Effect.asVoid),
      setVoiceProcessing: (on) => Ref.set(voiceProcessing, on)
    })
  })
}
