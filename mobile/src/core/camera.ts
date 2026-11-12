import { Context, type Effect, type Scope } from "effect"
import type { Handover } from "./channel"
import type { CameraError, TransportError } from "./errors"
import type { Socket } from "./socket"

export type Facing = "back" | "front"

/** A codec, a size and a frame rate (docs/protocol.md, section 8.2). The protocol names no tiers. */
export interface VideoMode {
  readonly codec: string
  readonly width: number
  readonly height: number
  readonly fps: number
}

/** What a camera says it can deliver: each size with the highest frame rate it lists for it. */
export interface CameraCapabilities {
  readonly sizes: ReadonlyArray<{
    readonly width: number
    readonly height: number
    readonly maxFps: number
  }>
}

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

/** What the native pipe did since it was last asked (see [NativeVideo.numbers]). */
export interface PipeNumbers {
  readonly sentFrames: number
  readonly sentBytes: number
  /** Seconds spent from sending frames to their acknowledgements, and how many were acknowledged. */
  readonly ackSeconds: number
  readonly acks: number
  /** Seconds spent sealing frames and handing them to the socket, and how many. */
  readonly sendSeconds: number
  readonly sends: number
}

/**
 * Where the platform can run the video connection natively. JavaScript opens the socket and does the handshake,
 * the authentication and the mode negotiation on it; `start` then hands the connection over, and from there the
 * frames are pulled from the camera, sealed, sent and acknowledged without passing through JavaScript (React
 * Native turns every message that does into base64). Absent where the module does not have it: the frames
 * then go through JavaScript.
 */
export interface NativeVideo {
  /** Opens a socket run by native code; closing the scope closes it. */
  readonly connect: (url: string) => Effect.Effect<Socket, TransportError, Scope.Scope>
  /** Takes the authenticated connection over; closing the scope stops the pipe. `depth`: pictures asked for ahead. */
  readonly start: (
    handover: Handover,
    depth: number
  ) => Effect.Effect<void, TransportError, Scope.Scope>
  /** The next control message from the PC that JavaScript has to look at (not the acknowledgements, which the pipe handles); fails when the pipe has ended. */
  readonly control: Effect.Effect<string, TransportError>
  readonly numbers: Effect.Effect<PipeNumbers>
}

/** The phone's camera as a source of JPEG frames. Provided by the platform. */
export class CameraIO extends Context.Service<
  CameraIO,
  {
    /** False where the native module is missing or the phone has no camera. */
    readonly available: boolean
    /** The codecs this phone's camera module can make (jpeg and h264). */
    readonly codecs: ReadonlyArray<string>
    readonly ensurePermission: Effect.Effect<void, CameraError>
    /** Read from the system, never assumed. */
    readonly capabilities: (facing: Facing) => Effect.Effect<CameraCapabilities, CameraError>
    /** The native pipe, where the platform has one. */
    readonly native: NativeVideo | null
    /** Opens the camera for a mode; closing the scope stops it. */
    readonly open: (
      facing: Facing,
      mode: VideoMode
    ) => Effect.Effect<CameraSession, CameraError, Scope.Scope>
  }
>()("phoneset/CameraIO") {}
