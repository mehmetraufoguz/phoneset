import { Context, type Effect, type Scope, type Stream } from "effect"
import type { AudioError } from "./errors"

export interface PlayerStats {
  readonly queuedMs: number
  readonly underruns: number
  readonly drops: number
}

/** Plays 48 kHz stereo s16le received from the PC. */
export interface Player {
  readonly write: (pcm: Uint8Array) => Effect.Effect<void>
  readonly stats: Effect.Effect<PlayerStats>
}

export interface AudioOptions {
  /** Echo cancellation and noise suppression (the platform's voice-chat mode). */
  readonly voiceProcessing: boolean
}

/** The phone's microphone and speaker/headphones. Provided by the platform. */
export class AudioIO extends Context.Service<
  AudioIO,
  {
    readonly ensureMicPermission: Effect.Effect<void, AudioError>
    readonly configure: (options: AudioOptions) => Effect.Effect<void, AudioError>
    /**
     * 20 ms frames of 48 kHz mono s16le. Capture starts when the stream is run
     * and stops when it ends or its fiber is interrupted.
     */
    readonly mic: Stream.Stream<Uint8Array, AudioError>
    /** Opens playback; closing the scope stops it. */
    readonly openPlayer: Effect.Effect<Player, AudioError, Scope.Scope>
  }
>()("phoneset/AudioIO") {}
