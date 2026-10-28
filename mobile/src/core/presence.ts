import { Context, Effect, Layer, Stream, SubscriptionRef } from "effect"
import { Session, type SessionState } from "./session"

/** What the phone shows outside the app while connected: a notification or a Live Activity. */
export interface PresenceSummary {
  readonly pc: string
  readonly line: string
  /** A couple of characters for the tiny Dynamic Island view. */
  readonly short: string
}

export const summarize = (state: SessionState): PresenceSummary | null => {
  switch (state._tag) {
    case "Connected": {
      const on = [state.mic.on ? "Microphone on" : null, state.speaker.on ? "Headphones on" : null]
      const parts = on.filter((p) => p !== null)
      return {
        pc: state.pc.name,
        line: parts.length > 0 ? parts.join(" · ") : "Connected",
        short: parts.length > 0 ? "ON" : "·"
      }
    }
    case "Connecting":
      return { pc: state.pcName, line: `${state.step}…`, short: "…" }
    default:
      return null
  }
}

/** Mirrors the connection to the lock screen / notification shade. Provided by the platform. */
export class Presence extends Context.Service<
  Presence,
  {
    /** null means nothing is connected: take the notification down. */
    readonly update: (summary: PresenceSummary | null) => Effect.Effect<void>
  }
>()("phoneset/Presence") {
  /** Keeps the presence in step with the session for as long as the app runs. */
  static readonly watcher = Layer.effectDiscard(
    Effect.gen(function* () {
      const session = yield* Session
      const presence = yield* Presence
      yield* SubscriptionRef.changes(session.state).pipe(
        Stream.map(summarize),
        // The session emits ten times a second for the level meter; only real changes matter.
        Stream.changesWith((a, b) => a?.pc === b?.pc && a?.line === b?.line),
        Stream.runForEach(presence.update),
        Effect.forkScoped
      )
    })
  )
}
