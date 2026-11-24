import type { VideoStatus } from "../core/session"
import { modeName } from "../core/videoModes"

/** The line under the switch: why it can't be used, how it is going, or what is being sent. */
export function detailOf(v: VideoStatus): { text: string; problem: boolean } {
  if (v.error) return { text: v.error, problem: true }
  if (v.unavailable) return { text: v.unavailable, problem: false }
  if (v.busy) return { text: "Starting…", problem: false }
  const r = v.report
  if (!v.on || r === null) return { text: "Off", problem: false }
  const used = `${modeName(r.mode)} ${r.mode.codec === "h264" ? "H.264" : "JPEG"}`
  const viewers =
    r.viewers === 0
      ? "not open on the PC yet"
      : r.viewers === 1
        ? "in use on the PC"
        : `${r.viewers} viewers`
  // What is really being sent, not only what was asked for: the rate measured (a phone that cannot encode 30 a second
  // said "720p30" while it made 21), and the size when the camera's picture has a different number of pixels than the
  // mode (turning the picture upright does not change the count).
  const rate = r.fps >= 1 ? `${Math.round(r.fps)} fps` : null
  const cam = r.camera
  const asked = r.mode.width * r.mode.height
  const size =
    cam !== null && cam.width > 0 && Math.abs(cam.width * cam.height - asked) / asked > 0.1
      ? `${cam.width}×${cam.height}`
      : null
  const sent = [used, size, rate, `${((r.kBps * 8) / 1000).toFixed(1)} Mbit/s`, viewers]
    .filter((part) => part !== null)
    .join("  ·  ")
  return {
    text: r.adjusted ? `What you picked isn't available here, so ${sent}` : sent,
    problem: false
  }
}
