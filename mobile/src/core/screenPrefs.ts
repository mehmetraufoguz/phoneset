import { Context, Effect, Layer } from "effect"
import type { StorageError } from "./errors"
import { KeyValue } from "./storage"

/** What the person chose about the phone's screen while the camera runs. The same for every PC. */
export interface ScreenChoices {
  /** Seconds without a touch before "Dim screen" starts by itself while the camera is on; 0 means never. */
  readonly autoDimSeconds: number
}

/** The times offered in Settings (0 = never). */
export const AUTO_DIM_OPTIONS = [0, 15, 30, 60] as const

/** Dims after 30 seconds of no touch: a camera left on for hours should not keep the whole screen lit. */
export const DEFAULT_SCREEN: ScreenChoices = { autoDimSeconds: 30 }

const KEY = "phoneset.screenChoices"

/** What was saved, or the defaults when nothing is saved or it is not something this version understands. */
export const parseScreenChoices = (raw: string | undefined): ScreenChoices => {
  if (raw === undefined) return DEFAULT_SCREEN
  try {
    const o = JSON.parse(raw) as Record<string, unknown> | null
    const seconds = AUTO_DIM_OPTIONS.find((n) => n === o?.autoDimSeconds)
    return seconds === undefined ? DEFAULT_SCREEN : { autoDimSeconds: seconds }
  } catch {
    return DEFAULT_SCREEN
  }
}

/** The screen choices, kept in the secure store next to the pairings. */
export class ScreenPrefs extends Context.Service<
  ScreenPrefs,
  {
    readonly get: Effect.Effect<ScreenChoices, StorageError>
    readonly save: (choices: ScreenChoices) => Effect.Effect<void, StorageError>
  }
>()("phoneset/ScreenPrefs") {
  static readonly layer = Layer.effect(
    ScreenPrefs,
    Effect.gen(function* () {
      const kv = yield* KeyValue
      return ScreenPrefs.of({
        get: Effect.map(kv.get(KEY), parseScreenChoices),
        save: (choices) => kv.set(KEY, JSON.stringify(choices))
      })
    })
  )
}
