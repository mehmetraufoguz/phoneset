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
    // Experimental: coded in hardware on the iPhone, relayed by the PC to /video.h264 and not decoded
    // there (docs/protocol.md, section 8.7). Android does not make it yet.
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
      // The widest size the camera lists at this height (854 x 480 over 640 x 480).
      const size = caps.sizes
        .filter((s) => s.height === tier.height)
        .reduce<(typeof caps.sizes)[number] | null>(
          (a, s) => (a === null || s.width > a.width ? s : a),
          null
        )
      if (size === null || tier.height > tried.maxHeight) continue
      for (const fps of RATES) {
        if (fps <= size.maxFps && fps <= tried.maxFps)
          modes.push({ codec, width: size.width, height: size.height, fps })
      }
    }
  }
  return modes
}

/** The mode to start with: 720p at 30 if offered, else the largest the phone offers. */
export const pickMode = (modes: ReadonlyArray<VideoMode>): VideoMode | null =>
  modes.find((m) => m.height === 720 && m.fps === 30) ??
  [...modes].sort((a, b) => b.width * b.height - a.width * a.height || b.fps - a.fps)[0] ??
  null
