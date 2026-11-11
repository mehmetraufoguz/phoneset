import { Effect, Layer } from "effect"
import { requireOptionalNativeModule } from "expo"
import { Camera } from "expo-camera"
import { type CameraCapabilities, CameraIO, type CameraStats } from "../core/camera"
import { VIDEO_QUALITY } from "../core/constants"
import { CameraError } from "../core/errors"

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

const ensurePermission = Effect.tryPromise({
  try: async () => {
    const status = await Camera.requestCameraPermissionsAsync()
    if (!status.granted) throw new Error("denied")
  },
  catch: () =>
    new CameraError({ message: "Camera access is off. Turn it on for phoneset in Settings." })
})

export const CameraLive = Layer.succeed(CameraIO, {
  available: native?.isAvailable() === true,
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
