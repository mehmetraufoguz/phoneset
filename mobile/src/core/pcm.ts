// Small, allocation-conscious PCM helpers. Wire format is s16le.

export const s16ToFloat = (pcm: Uint8Array): Float32Array => {
  const view = new DataView(pcm.buffer, pcm.byteOffset, pcm.byteLength)
  const out = new Float32Array(pcm.byteLength >> 1)
  for (let i = 0; i < out.length; i++) out[i] = view.getInt16(i * 2, true) / 32768
  return out
}

export const floatToS16 = (samples: Float32Array): Uint8Array => {
  const out = new Uint8Array(samples.length * 2)
  const view = new DataView(out.buffer)
  for (let i = 0; i < samples.length; i++) {
    const s = Math.max(-1, Math.min(1, samples[i] ?? 0))
    view.setInt16(i * 2, Math.round(s * 32767), true)
  }
  return out
}

/** Splits interleaved stereo s16le into two float channels. */
export const deinterleaveStereo = (pcm: Uint8Array): readonly [Float32Array, Float32Array] => {
  const view = new DataView(pcm.buffer, pcm.byteOffset, pcm.byteLength)
  const frames = pcm.byteLength >> 2
  const left = new Float32Array(frames)
  const right = new Float32Array(frames)
  for (let i = 0; i < frames; i++) {
    left[i] = view.getInt16(i * 4, true) / 32768
    right[i] = view.getInt16(i * 4 + 2, true) / 32768
  }
  return [left, right]
}

/** Root-mean-square level of s16le audio, 0..1. */
export const rms = (pcm: Uint8Array): number => {
  const view = new DataView(pcm.buffer, pcm.byteOffset, pcm.byteLength)
  const n = pcm.byteLength >> 1
  if (n === 0) return 0
  let sum = 0
  for (let i = 0; i < n; i++) {
    const s = view.getInt16(i * 2, true) / 32768
    sum += s * s
  }
  return Math.sqrt(sum / n)
}

/** Streaming linear resampler for mono audio. */
export class Resampler {
  private readonly step: number
  private pos = 0
  private carry: Float32Array = new Float32Array(0)

  constructor(inRate: number, outRate: number) {
    this.step = inRate / outRate
  }

  process(input: Float32Array): Float32Array {
    if (this.step === 1) return input
    const buf = new Float32Array(this.carry.length + input.length)
    buf.set(this.carry)
    buf.set(input, this.carry.length)

    const out: number[] = []
    while (Math.floor(this.pos) + 1 < buf.length) {
      const i = Math.floor(this.pos)
      const frac = this.pos - i
      const a = buf[i] ?? 0
      const b = buf[i + 1] ?? 0
      out.push(a + (b - a) * frac)
      this.pos += this.step
    }
    const keep = Math.min(Math.floor(this.pos), buf.length)
    this.carry = buf.slice(keep)
    this.pos -= keep
    return Float32Array.from(out)
  }
}

/** Re-cuts a stream of s16le chunks into fixed-size frames. */
export class Chunker {
  private pending = new Uint8Array(0)

  constructor(private readonly frameBytes: number) {}

  push(data: Uint8Array): Uint8Array[] {
    const joined = new Uint8Array(this.pending.length + data.length)
    joined.set(this.pending)
    joined.set(data, this.pending.length)

    const frames: Uint8Array[] = []
    let offset = 0
    while (joined.length - offset >= this.frameBytes) {
      frames.push(joined.slice(offset, offset + this.frameBytes))
      offset += this.frameBytes
    }
    this.pending = joined.slice(offset)
    return frames
  }
}
