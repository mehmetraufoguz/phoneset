import { Context, type Effect, type Scope } from "effect"
import type { CameraError } from "./errors"

export type Facing = "back" | "front"

/** What the camera side measured since it started (for the diagnostics line). */
export interface CameraStats {
  readonly width: number
  readonly height: number
  /** Pictures the camera delivered, and JPEGs made from them, per second. */
  readonly analysedFps: number
  readonly encodedFps: number
  /** Average time to make one JPEG, and its average size. */
  readonly encodeMs: number
  readonly jpegKB: number
}

/**
 * An open camera. Frames are pulled, not pushed: `next` makes the picture the camera has now, so a
 * sender that is not ready simply does not ask and no work is done for pictures that would be dropped.
 */
export interface CameraSession {
  /** One frame, the body of a type 0x04 message (see docs/protocol.md, section 8.3). */
  readonly next: Effect.Effect<Uint8Array, CameraError>
  readonly stats: Effect.Effect<CameraStats>
}

/** The phone's camera as a source of JPEG frames. Provided by the platform. */
export class CameraIO extends Context.Service<
  CameraIO,
  {
    /** False where the native module is missing or the phone has no camera. */
    readonly available: boolean
    readonly ensurePermission: Effect.Effect<void, CameraError>
    /** Opens the camera; closing the scope stops it. */
    readonly open: (facing: Facing) => Effect.Effect<CameraSession, CameraError, Scope.Scope>
  }
>()("phoneset/CameraIO") {}
