import type { CameraCapabilities, VideoMode } from "./camera"

// Which video modes this phone offers (docs/protocol.md, section 8.2). The protocol names no tiers; these
// are the ones planned for the product, listed so that the phone can say which of them its camera
// reports and its encoder has been tried at.

/** The sizes planned, by the picture's height (the long side is 16:9 of it, or what the camera lists). */
export const TIERS = [
  { name: "480p", height: 480 },
  { name: "720p", height: 720 },
  { name: "1080p", height: 1080 },
  { name: "2K", height: 1440 },
  { name: "4K", height: 2160 }
] as const

export const RATES = [30, 60] as const

/**
 * What each codec has actually been tried at on this phone side. JPEG is only exercised at 1280 x 720 and
 * 30 frames a second so far; offering more would be a claim nobody has measured. A codec that is coded in
 * hardware (h264, hevc) would get its own row, with the PC able to decode it.
 */
export const CODEC_TRIED: Record<string, { readonly maxHeight: number; readonly maxFps: number }> =
  {
    jpeg: { maxHeight: 720, maxFps: 30 },
    // Experimental: coded in hardware on the phone, relayed by the PC to /video.h264 and not decoded
    // there (docs/protocol.md, section 8.7).
    h264: { maxHeight: 2160, maxFps: 60 }
  }

/** The tier a mode belongs to ("720p30"), for display. */
export const modeName = (m: VideoMode): string =>
  `${TIERS.find((t) => t.height === m.height)?.name ?? `${m.height}p`}${m.fps}`

/**
 * Every (codec, tier, rate) the camera reports it can deliver, the PC accepts, and has been tried:
 * nothing is assumed about the link.
 */
export const modesFor = (
  caps: CameraCapabilities,
  codecs: ReadonlyArray<string>
): ReadonlyArray<VideoMode> => {
  const modes: Array<VideoMode> = []
  for (const codec of codecs) {
    const tried = CODEC_TRIED[codec]
    if (tried === undefined) continue
    for (const tier of TIERS) {
      // The size the camera lists at this height that is closest to 16:9 (854 x 480 over 640 x 480), the wider
      // of two equals. Not simply the widest: a camera that also lists 20:9 or 21:9 sizes (2400 x 1080) would give
      // a size that is not 16:9, and the native side asks CameraX for 16:9, which then jumps to the next bigger
      // picture (2688 x 1512 where 1080p was asked for, 1920 x 1080 where 720p was).
      const size = caps.sizes
        .filter((s) => s.height === tier.height)
        .reduce<(typeof caps.sizes)[number] | null>((a, s) => {
          if (a === null) return s
          const gap = (x: { width: number; height: number }) =>
            Math.abs(x.width / x.height - 16 / 9)
          return gap(s) < gap(a) || (gap(s) === gap(a) && s.width > a.width) ? s : a
        }, null)
      if (size === null || tier.height > tried.maxHeight) continue
      for (const fps of RATES) {
        if (fps <= size.maxFps && fps <= tried.maxFps)
          modes.push({ codec, width: size.width, height: size.height, fps })
      }
    }
  }
  return modes
}

/** The format the user picked: left to the app, or one codec. */
export type VideoFormat = "auto" | "h264" | "jpeg"

/** What the user asked the camera for: a size (the picture's height), a frame rate and a format. */
export interface VideoPrefs {
  readonly height: number
  readonly fps: number
  readonly format: VideoFormat
}

/** 720p at 30 frames a second, format left to the app: what is sent until the user picks something else. */
export const DEFAULT_PREFS: VideoPrefs = { height: 720, fps: 30, format: "auto" }

/** The mode to use, and whether it is not what was asked for (the phone or the PC does not offer that). */
export interface PickedMode {
  readonly mode: VideoMode
  readonly adjusted: boolean
}

/** One size of the picker: its height, its name and the highest rate offered at it. */
export interface TierOffer {
  readonly height: number
  readonly name: string
  readonly maxFps: number
}

/** The sizes the offered modes cover, smallest first, each with the highest frame rate offered at it. */
export const tiersOffered = (modes: ReadonlyArray<VideoMode>): ReadonlyArray<TierOffer> =>
  TIERS.flatMap((tier) => {
    const at = modes.filter((m) => m.height === tier.height)
    return at.length === 0
      ? []
      : [{ height: tier.height, name: tier.name, maxFps: Math.max(...at.map((m) => m.fps)) }]
  })

/** The modes of the picked format; all of them when the format is left to the app or none is offered. */
export const modesOfFormat = (
  modes: ReadonlyArray<VideoMode>,
  format: VideoFormat
): ReadonlyArray<VideoMode> => {
  if (format === "auto") return modes
  const of = modes.filter((m) => m.codec === format)
  return of.length === 0 ? modes : of
}

/** Where two modes have the same size and rate, the format left to the app is H.264 (smaller, smoother), else the first. */
const preferred = (modes: ReadonlyArray<VideoMode>): VideoMode | null =>
  modes.find((m) => m.codec === "h264") ?? modes[0] ?? null

/**
 * The mode for what was asked: that size at that rate in that format. A format that is not offered at all (the PC
 * does not take it, or the phone cannot make it) is dropped. Where the size or rate is not offered, the nearest:
 * the same size at the closest rate, else the closest size (smaller first on a tie) at the closest rate.
 */
export const pickMode = (
  modes: ReadonlyArray<VideoMode>,
  prefs: VideoPrefs = DEFAULT_PREFS
): PickedMode | null => {
  if (modes.length === 0) return null
  const pool = modesOfFormat(modes, prefs.format)
  const formatDropped = prefs.format !== "auto" && pool === modes
  const closest = (candidates: ReadonlyArray<VideoMode>, by: (m: VideoMode) => number) => {
    const best = Math.min(...candidates.map(by))
    return candidates.filter((m) => by(m) === best)
  }
  const exact = preferred(pool.filter((m) => m.height === prefs.height && m.fps === prefs.fps))
  if (exact !== null) return { mode: exact, adjusted: formatDropped }
  // The nearest size first, then the nearest rate at it (a lower rate before a higher one).
  const sizes = closest(
    pool,
    (m) => Math.abs(m.height - prefs.height) * 2 + (m.height > prefs.height ? 1 : 0)
  )
  const rates = closest(sizes, (m) => Math.abs(m.fps - prefs.fps) * 2 + (m.fps > prefs.fps ? 1 : 0))
  const mode = preferred(rates)
  return mode === null ? null : { mode, adjusted: true }
}
