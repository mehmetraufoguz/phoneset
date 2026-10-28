import { Effect, Layer } from "effect"
import Constants from "expo-constants"
import type { LiveActivity } from "expo-widgets"
import { Platform } from "react-native"
import { AudioManager, RecordingNotificationManager } from "react-native-audio-api"
import { Presence, type PresenceSummary } from "../core/presence"
import type { ConnectionProps } from "../widgets/ConnectionActivity"

// iOS has no notification that can't be swiped away. The closest thing is a Live Activity on
// the lock screen and in the Dynamic Island. Android keeps the process alive with a
// foreground service, whose notification stays in the shade while connected.

/** Refreshed while the app runs; if it stops (app suspended), iOS marks the activity stale. */
const STALE_AFTER_MS = 90_000
const HEARTBEAT_MS = 30_000

// Set by app.config.ts (PHONESET_LIVE_ACTIVITY=1). Off, the widget code is never loaded.
const liveActivityEnabled = Constants.expoConfig?.extra?.liveActivity === true

const makeIos = () => {
  let factory: typeof import("../widgets/ConnectionActivity")["default"] | undefined
  const load = async () => {
    factory ??= (await import("../widgets/ConnectionActivity")).default
    return factory
  }
  let activity: LiveActivity<ConnectionProps> | undefined
  let beat: ReturnType<typeof setInterval> | undefined
  let last: ConnectionProps | undefined
  const stale = () => new Date(Date.now() + STALE_AFTER_MS)

  return async (summary: PresenceSummary | null) => {
    if (!liveActivityEnabled) return
    if (summary === null) {
      if (beat !== undefined) clearInterval(beat)
      beat = undefined
      const ending = activity
      activity = undefined
      await ending?.end("immediate")
      return
    }
    const props: ConnectionProps = { pc: summary.pc, line: summary.line, short: summary.short }
    if (activity === undefined) {
      // A launch can find one left over from before; take it over instead of stacking a second.
      const connection = await load()
      const [existing, ...extra] = connection.getInstances()
      for (const e of extra) await e.end("immediate")
      activity = existing ?? connection.start(props, undefined, stale())
    }
    last = props
    await activity.update(props, stale())
    beat ??= setInterval(() => {
      if (activity !== undefined && last !== undefined)
        activity.update(last, stale()).catch(() => {})
    }, HEARTBEAT_MS)
  }
}

const makeAndroid = () => {
  let shown = false
  let asked = false
  return async (summary: PresenceSummary | null) => {
    if (summary === null) {
      if (shown) await RecordingNotificationManager.hide()
      shown = false
      return
    }
    if (!asked) {
      asked = true
      await AudioManager.requestNotificationPermissions()
    }
    // The foreground service is a microphone one: Android refuses to start it without the permission.
    // Until the person has allowed the microphone there is nothing to keep alive.
    if ((await AudioManager.checkRecordingPermissions()) !== "Granted") return
    await RecordingNotificationManager.show({
      title: `phoneset · ${summary.pc}`,
      contentText: summary.line
    })
    shown = true
  }
}

const push = Platform.OS === "ios" ? makeIos() : makeAndroid()

export const PresenceLive = Layer.succeed(Presence, {
  // Presence is a nicety. Whatever goes wrong here must never touch the connection.
  update: (summary) => Effect.promise(() => push(summary).catch(() => {}))
})
