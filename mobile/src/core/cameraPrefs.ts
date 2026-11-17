import { Context, Effect, Layer } from "effect"
import type { Facing } from "./camera"
import type { StorageError } from "./errors"
import { KeyValue } from "./storage"
import { DEFAULT_PREFS, TIERS, type VideoFormat } from "./videoModes"

/** What the user picked for the camera, kept for one PC: which camera, the size, the rate and the format. */
export interface CameraChoices {
  readonly camera: Facing
  readonly quality: number
  readonly rate: number
  readonly format: VideoFormat
}

/** What the camera starts with before anything was picked (the back camera, 720p at 30, format left to the app). */
export const DEFAULT_CHOICES: CameraChoices = {
  camera: "back",
  quality: DEFAULT_PREFS.height,
  rate: DEFAULT_PREFS.fps,
  format: DEFAULT_PREFS.format
}

const KEY = "phoneset.cameraChoices"

/** One saved entry, or null when it is not one this version understands (it is then dropped, not guessed at). */
const valid = (v: unknown): CameraChoices | null => {
  if (typeof v !== "object" || v === null) return null
  const o = v as Record<string, unknown>
  const camera = o.camera === "front" ? "front" : o.camera === "back" ? "back" : null
  const format = o.format === "h264" || o.format === "jpeg" || o.format === "auto" ? o.format : null
  const quality = TIERS.find((t) => t.height === o.quality)?.height
  const rate = o.rate === 30 || o.rate === 60 ? o.rate : null
  return camera === null || format === null || quality === undefined || rate === null
    ? null
    : { camera, quality, rate, format }
}

const parse = (raw: string | undefined): Record<string, CameraChoices> => {
  if (raw === undefined) return {}
  try {
    const all = JSON.parse(raw) as unknown
    if (typeof all !== "object" || all === null) return {}
    const out: Record<string, CameraChoices> = {}
    for (const [id, entry] of Object.entries(all)) {
      const choices = valid(entry)
      if (choices !== null) out[id] = choices
    }
    return out
  } catch {
    return {}
  }
}

/**
 * The camera choices, one set per PC (a laptop on a good Wi-Fi may want 1080p while a desktop on a weak one wants
 * 480p). Kept next to the pairings, in the secure store. A PC without a saved set gets the defaults.
 */
export class CameraPrefs extends Context.Service<
  CameraPrefs,
  {
    /** The choices saved for this PC, or the defaults. */
    readonly get: (pcId: string) => Effect.Effect<CameraChoices, StorageError>
    readonly save: (pcId: string, choices: CameraChoices) => Effect.Effect<void, StorageError>
    /** Forgets what was saved for one PC (it goes back to the defaults). */
    readonly reset: (pcId: string) => Effect.Effect<void, StorageError>
    /** Forgets what was saved for every PC; needs no connection. */
    readonly resetAll: Effect.Effect<void, StorageError>
  }
>()("phoneset/CameraPrefs") {
  static readonly layer = Layer.effect(
    CameraPrefs,
    Effect.gen(function* () {
      const kv = yield* KeyValue
      const read = Effect.map(kv.get(KEY), parse)
      const write = (all: Record<string, CameraChoices>) =>
        Object.keys(all).length === 0 ? kv.remove(KEY) : kv.set(KEY, JSON.stringify(all))
      return CameraPrefs.of({
        get: (pcId) => Effect.map(read, (all) => all[pcId] ?? DEFAULT_CHOICES),
        save: (pcId, choices) => Effect.flatMap(read, (all) => write({ ...all, [pcId]: choices })),
        reset: (pcId) =>
          Effect.flatMap(read, (all) => {
            const { [pcId]: _gone, ...rest } = all
            return write(rest)
          }),
        resetAll: kv.remove(KEY)
      })
    })
  )
}
