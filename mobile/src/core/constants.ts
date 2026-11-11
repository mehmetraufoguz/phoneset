// Wire constants. Keep in sync with pc/internal/proto and docs/protocol.md.

export const PROTOCOL_VERSION = 2
export const DEFAULT_PORT = 47831
export const WS_PATH = "/v2/ws"
export const INFO_PATH = "/v2/info"

/** mDNS service advertised by the PC: _phoneset._tcp.local. */
export const MDNS = { type: "phoneset", protocol: "tcp", domain: "local." } as const

export const SAMPLE_RATE = 48_000
export const FRAME_MS = 20
export const FRAME_SAMPLES = (SAMPLE_RATE * FRAME_MS) / 1000
export const MIC_CHANNELS = 1
export const SPEAKER_CHANNELS = 2

/** First plaintext byte of every message inside the encrypted channel. */
export const Frame = { control: 0x01, mic: 0x02, speaker: 0x03, video: 0x04 } as const

/** Audio messages carry a big-endian u32 sequence number before the PCM. */
export const AUDIO_HEADER_LEN = 4

/** The camera is on a connection of its own (docs/protocol.md, section 8). */
export const VIDEO_PATH = "/v2/video"

/** JPEG quality asked of the camera side. The size and rate come from the video mode (core/videoModes.ts). */
export const VIDEO_QUALITY = 70

/**
 * Frames sent and not yet acknowledged: at most this many, and at most this many bytes. Three frames and
 * 2 MB, from the measurements in docs/webcam-measurements.md: with 2 frames and 256 KB a frame over about
 * 128 KB (1080p and up) was the only one in flight, the next was made only after its acknowledgement, and
 * 1080p30 reached 17 fps instead of 30. A larger window costs latency on a slow link (more frames wait).
 */
export const VIDEO_WINDOW = { frames: 3, bytes: 2 * 1024 * 1024 } as const
