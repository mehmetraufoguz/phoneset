import { Effect, Layer } from "effect"
import { requireOptionalNativeModule } from "expo"
import { Camera } from "expo-camera"
import {
  type CameraCapabilities,
  CameraIO,
  type CameraStats,
  type NativeVideo,
  type PipeNumbers
} from "../core/camera"
import { VIDEO_QUALITY, VIDEO_WINDOW } from "../core/constants"
import { CameraError, TransportError } from "../core/errors"

// The native module in modules/phoneset-camera: CameraX on Android, AVFoundation on iOS. Where it is
// missing (a development build made before it was added) the camera is simply not offered.
interface PhonesetCameraModule {
  isAvailable(): boolean
  start(
    facing: string,
    width: number,
    height: number,
    fps: number,
    quality: number,
    codec: string
  ): Promise<void>
  nextFrame(): Promise<unknown>
  capabilities(facing: string): CameraCapabilities
  stop(): void
  stats(): Record<string, number>
  // The native video connection (absent in a build made before it was added).
  nativePipe?(): boolean
  socketConnect?(url: string): Promise<number>
  socketReceive?(id: number): Promise<unknown>
  socketSend?(id: number, data: Uint8Array): void
  socketClose?(id: number): void
  // `material` is the send key, the receive key and the transcript, one after the other; `settings` is sent,
  // received, window frames, window bytes and depth (packed: Expo's Kotlin functions take at most eight arguments).
  pipeStart?(id: number, material: Uint8Array, settings: number[]): Promise<void>
  pipeControl?(): Promise<string>
  pipeNumbers?(): Record<string, number>
  pipeStop?(): void
}

const native = requireOptionalNativeModule<PhonesetCameraModule>("PhonesetCamera")

const failure = (fallback: string) => (cause: unknown) =>
  new CameraError({ message: cause instanceof Error ? cause.message : fallback })

/** Whatever the native side hands back, as a Uint8Array. */
const bytes = (value: unknown): Uint8Array =>
  value instanceof Uint8Array
    ? value
    : value instanceof ArrayBuffer
      ? new Uint8Array(value)
      : Uint8Array.from(value as ArrayLike<number>)

/** The arrays one after the other in a new one. */
const joined = (...parts: Uint8Array[]): Uint8Array => {
  const out = new Uint8Array(parts.reduce((n, part) => n + part.length, 0))
  let at = 0
  for (const part of parts) {
    out.set(part, at)
    at += part.length
  }
  return out
}

const ensurePermission = Effect.tryPromise({
  try: async () => {
    const status = await Camera.requestCameraPermissionsAsync()
    if (!status.granted) throw new Error("denied")
  },
  catch: () =>
    new CameraError({ message: "Camera access is off. Turn it on for phoneset in Settings." })
})

const transport = (fallback: string) => (cause: unknown) =>
  new TransportError({ message: cause instanceof Error ? cause.message : fallback })

/** The native video connection, where this build of the module has it. */
const nativeVideo: NativeVideo | null =
  native?.nativePipe?.() === true
    ? {
        connect: (url) =>
          Effect.gen(function* () {
            const module = native as Required<PhonesetCameraModule>
            const id = yield* Effect.acquireRelease(
              Effect.tryPromise({
                try: () => module.socketConnect(url),
                catch: transport("Couldn't reach the PC.")
              }),
              (id) => Effect.sync(() => module.socketClose(id))
            )
            return {
              nativeId: id,
              send: (data) =>
                Effect.try({
                  try: () => module.socketSend(id, data),
                  catch: transport("The connection dropped.")
                }),
              receive: Effect.tryPromise({
                try: () => module.socketReceive(id),
                catch: transport("The connection closed.")
              }).pipe(Effect.map(bytes))
            }
          }),
        start: (handover, depth) =>
          Effect.acquireRelease(
            Effect.tryPromise({
              try: () =>
                (native as Required<PhonesetCameraModule>).pipeStart(
                  handover.socketId,
                  joined(handover.sendKey, handover.receiveKey, handover.transcript),
                  [handover.sent, handover.received, VIDEO_WINDOW.frames, VIDEO_WINDOW.bytes, depth]
                ),
              catch: transport("The video connection didn't start.")
            }),
            () => Effect.sync(() => (native as Required<PhonesetCameraModule>).pipeStop())
          ),
        control: Effect.tryPromise({
          try: () => (native as Required<PhonesetCameraModule>).pipeControl(),
          catch: transport("The video ended.")
        }),
        numbers: Effect.sync(
          () => (native as Required<PhonesetCameraModule>).pipeNumbers() as unknown as PipeNumbers
        )
      }
    : null

export const CameraLive = Layer.succeed(CameraIO, {
  available: native?.isAvailable() === true,
  native: nativeVideo,
  // JPEG is made on the CPU and h264 by the hardware encoder, on both platforms.
  codecs: ["jpeg", "h264"],
  ensurePermission,
  capabilities: (facing) =>
    Effect.try({
      try: () => (native as PhonesetCameraModule).capabilities(facing),
      catch: failure("Couldn't read what the camera can do.")
    }),
  open: (facing, mode) =>
    Effect.gen(function* () {
      const module = native
      if (module === null)
        return yield* new CameraError({ message: "This phone can't send its camera yet." })
      yield* Effect.acquireRelease(
        Effect.tryPromise({
          try: () =>
            module.start(facing, mode.width, mode.height, mode.fps, VIDEO_QUALITY, mode.codec),
          catch: failure("The camera didn't start.")
        }),
        () => Effect.sync(() => module.stop())
      )
      return {
        next: Effect.tryPromise({
          try: () => module.nextFrame(),
          catch: failure("The camera stopped.")
        }).pipe(Effect.map(bytes)),
        stats: Effect.sync(() => module.stats() as unknown as CameraStats)
      }
    })
})
