/**
 * How much audio the speaker keeps queued before it plays, so that late frames do not run it dry.
 *
 * The PC sends a frame every 20 ms. On a good network they arrive that way, and 60 ms of cushion is plenty. When the
 * phone's screen is off, Wi-Fi power saving can hold the frames back and hand them over together every 100 ms or so;
 * a 60 ms cushion then runs empty between the groups, and each empty moment is a click and a jump in delay. So the
 * cushion follows the longest wait between frames seen lately: it grows at once when frames come late in groups and
 * shrinks back, over a few seconds, when they come steadily again.
 */
export const MIN_CUSHION_MS = 60
export const MAX_CUSHION_MS = 260
/** A wait this long is a pause in the sound (the PC sends nothing during silence), not lateness. */
const PAUSE_MS = 400
/** How far back the longest wait is looked for. */
const WINDOW_MS = 8000

export class JitterTarget {
  private readonly waits: Array<{ at: number; wait: number }> = []
  private head = 0
  private last = 0
  private extra = 0
  private extraAt = 0

  /** A frame arrived at `now` (milliseconds). */
  arrive(now: number): void {
    if (this.last !== 0) {
      const wait = now - this.last
      if (wait <= PAUSE_MS) this.waits.push({ at: now, wait })
    }
    this.last = now
    while (this.head < this.waits.length && now - (this.waits[this.head]?.at ?? now) > WINDOW_MS) {
      this.head++
    }
    if (this.head > 256) {
      this.waits.splice(0, this.head)
      this.head = 0
    }
  }

  /** The queue ran empty at `now`: ask for more cushion for a while, whatever the waits said. */
  underrun(now: number): void {
    this.extra = Math.min(80, this.decayed(now) + 30)
    this.extraAt = now
  }

  /** The cushion to keep, in milliseconds, at `now`. */
  target(now: number): number {
    let worst = 0
    for (let i = this.head; i < this.waits.length; i++) {
      const wait = this.waits[i]?.wait ?? 0
      if (wait > worst) worst = wait
    }
    const want = worst * 1.15 + 15 + this.decayed(now)
    return Math.max(MIN_CUSHION_MS, Math.min(MAX_CUSHION_MS, Math.round(want)))
  }

  /** The extra cushion after an underrun fades by 10 ms a second. */
  private decayed(now: number): number {
    return Math.max(0, this.extra - ((now - this.extraAt) / 1000) * 10)
  }
}

/** Raises the first `samples` of a channel from silence, so sound that starts after a gap does not start with a click. */
export function fadeIn(channel: Float32Array, samples: number): void {
  const n = Math.min(samples, channel.length)
  for (let i = 0; i < n; i++) channel[i] = (channel[i] ?? 0) * (i / n)
}
