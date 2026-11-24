import { Effect } from "effect"
import { Pressable, StyleSheet, Switch, Text, View } from "react-native"
import { Session, type SessionState, type StreamStatus, type VideoStatus } from "../core/session"
import { modesOfFormat, tiersOffered } from "../core/videoModes"
import { detailOf } from "./cameraStatus"
import { act, usePalette } from "./hooks"
import { Label, NavRow, Segments } from "./parts"

type Palette = ReturnType<typeof usePalette>
type Connected = Extract<SessionState, { _tag: "Connected" }>

/** The three ready-made pictures. They are sizes and rates the camera already offers, named so nobody has to read them. */
const PRESETS = [
  { key: "auto", label: "Auto", hint: "720p", height: 720, fps: 30 },
  { key: "sharp", label: "Sharp", hint: "1080p", height: 1080, fps: 30 },
  { key: "smooth", label: "Smooth", hint: "60 fps", height: 720, fps: 60 }
] as const

/** One line under a switch: why it can't be used, how it is going, or that it is off. */
const line = (s: StreamStatus, on: string): string =>
  s.error ?? (s.on ? on : s.available ? "Off" : "Not available on this PC")

/** The line under the camera's title. */
const cameraLine = (v: VideoStatus): string => {
  if (v.error) return v.error
  if (v.unavailable) return v.unavailable
  if (v.busy) return "Starting…"
  if (!v.on) return "Off"
  return v.report === null || v.report.viewers === 0 ? "Not open on the PC yet" : "In use on the PC"
}

function Tile({
  p,
  icon,
  title,
  sub,
  subColor,
  on,
  disabled,
  onChange,
  children
}: {
  readonly p: Palette
  readonly icon: string
  readonly title: string
  readonly sub: string
  readonly subColor?: string
  readonly on: boolean
  readonly disabled?: boolean
  readonly onChange: (on: boolean) => void
  readonly children?: React.ReactNode
}) {
  return (
    <View
      style={[
        styles.tile,
        { backgroundColor: on ? p.onBg : p.surface, borderColor: on ? p.onLine : p.line }
      ]}
    >
      <Pressable
        style={styles.tileRow}
        disabled={disabled}
        onPress={() => onChange(!on)}
        accessibilityRole="switch"
        accessibilityState={{ checked: on, disabled }}
        accessibilityLabel={title}
      >
        <View style={[styles.icon, { backgroundColor: on ? "#3B6BFF" : p.raised }]}>
          <Text style={styles.iconText}>{icon}</Text>
        </View>
        <View style={styles.tileText}>
          <Text style={[styles.title, { color: p.text }]}>{title}</Text>
          <Text style={[styles.sub, { color: subColor ?? p.sub }]} numberOfLines={2}>
            {sub}
          </Text>
        </View>
        <Switch
          value={on}
          disabled={disabled}
          onValueChange={onChange}
          trackColor={{ true: p.ok }}
        />
      </Pressable>
      {children}
    </View>
  )
}

/**
 * The connected PC as three big switches: microphone, headphones, camera. The camera's picture, which camera and the
 * Dim screen button sit under its switch while it is on; everything rarer (format, echo cancellation, the
 * subscription) is on the "More" screen. Plain views, not Expo UI's form, so a touch anywhere is seen by the caller.
 */
export function ConnectedView({
  state,
  autoDimSeconds,
  onDim,
  onMore,
  onDisconnect
}: {
  readonly state: Connected
  readonly autoDimSeconds: number
  readonly onDim: () => void
  readonly onMore: () => void
  readonly onDisconnect: () => void
}) {
  const p = usePalette()
  const v = state.video

  // The sizes and rates this phone and PC offer (known once the camera has been on): a picture that is not offered is greyed.
  const offered = v.report === null ? null : tiersOffered(modesOfFormat(v.report.offered, v.format))
  const available = (height: number, fps: number) =>
    offered === null || offered.some((t) => t.height === height && t.maxFps >= fps)

  const pick = (height: number, fps: number) =>
    act(
      Effect.gen(function* () {
        if (v.quality !== height) yield* Session.use((s) => s.setQuality(height))
        if (v.rate !== fps) yield* Session.use((s) => s.setRate(fps))
      })
    )

  const picked = PRESETS.find((x) => x.height === v.quality && x.fps === v.rate)?.key ?? null
  const detail = detailOf(v)

  return (
    <View style={styles.wrap}>
      <View style={styles.head}>
        <View style={{ flex: 1 }}>
          <Text style={[styles.pcName, { color: p.text }]} numberOfLines={1}>
            {state.pc.name}
          </Text>
          <Text style={{ color: p.ok, fontSize: 13, fontWeight: "600" }}>Connected</Text>
        </View>
        <Text style={[styles.sub, { color: p.sub }]}>
          {state.rttMs === null ? "…" : `${state.rttMs} ms`}
        </Text>
      </View>

      <Tile
        p={p}
        icon="🎙️"
        title="Microphone"
        sub={line(state.mic, `Sending to ${state.mic.device ?? "the PC"}`)}
        subColor={state.mic.error ? p.danger : undefined}
        on={state.mic.on}
        disabled={state.mic.busy || !state.mic.available}
        onChange={(on) => act(Session.use((s) => s.setMic(on)))}
      >
        {state.mic.on ? (
          <View style={[styles.levelTrack, { backgroundColor: p.raised }]}>
            <View
              style={[
                styles.levelFill,
                { width: `${Math.round(Math.min(1, Math.sqrt(state.micLevel) * 1.6) * 100)}%` }
              ]}
            />
          </View>
        ) : null}
      </Tile>

      <Tile
        p={p}
        icon="🎧"
        title="Headphones"
        sub={line(state.speaker, `Playing ${state.speaker.device ?? "PC audio"}`)}
        subColor={state.speaker.error ? p.danger : undefined}
        on={state.speaker.on}
        disabled={state.speaker.busy || !state.speaker.available}
        onChange={(on) => act(Session.use((s) => s.setSpeaker(on)))}
      />

      <Tile
        p={p}
        icon="📷"
        title="Camera"
        sub={cameraLine(v)}
        subColor={v.error ? p.danger : undefined}
        on={v.on || v.busy}
        disabled={v.busy || v.unavailable !== null}
        onChange={(on) => act(Session.use((s) => s.setVideo(on)))}
      >
        {v.on ? (
          <View style={styles.cameraBody}>
            <Label p={p} text="PICTURE" />
            <Segments
              p={p}
              items={PRESETS.map((x) => ({
                key: x.key,
                label: x.label,
                hint: x.hint,
                on: picked === x.key,
                disabled: v.busy || !available(x.height, x.fps),
                onPress: () => pick(x.height, x.fps)
              }))}
            />
            <Label p={p} text="CAMERA" />
            <Segments
              p={p}
              items={(["back", "front"] as const).map((facing) => ({
                key: facing,
                label: facing === "back" ? "Back" : "Front",
                on: v.camera === facing,
                disabled: v.busy,
                onPress: () => act(Session.use((s) => s.setCamera(facing)))
              }))}
            />
            {detail.text === "Off" ? null : (
              <Text style={[styles.small, { color: detail.problem ? p.danger : p.sub }]}>
                {detail.text}
              </Text>
            )}
          </View>
        ) : null}
      </Tile>

      {v.on ? (
        <View style={styles.dim}>
          <Pressable
            style={[styles.primary, { backgroundColor: "#3B6BFF" }]}
            onPress={onDim}
            accessibilityRole="button"
          >
            <Text style={styles.primaryText}>Dim screen</Text>
          </Pressable>
          <Text style={[styles.small, { color: p.sub, textAlign: "center" }]}>
            {autoDimSeconds > 0
              ? `Also dims by itself after ${autoDimSeconds} s without a touch`
              : "Auto-dim is off (Settings)"}
          </Text>
        </View>
      ) : null}

      <NavRow p={p} label="More options" onPress={onMore} />

      <Pressable
        style={[styles.quiet, { backgroundColor: p.raised }]}
        onPress={onDisconnect}
        accessibilityRole="button"
      >
        <Text style={[styles.quietText, { color: p.text }]}>Disconnect</Text>
      </Pressable>
    </View>
  )
}

const styles = StyleSheet.create({
  wrap: { padding: 16, gap: 12 },
  head: { flexDirection: "row", alignItems: "center", gap: 8, paddingHorizontal: 4 },
  pcName: { fontSize: 24, fontWeight: "700", letterSpacing: -0.4 },
  tile: { borderRadius: 20, borderWidth: 1, padding: 14, gap: 12 },
  tileRow: { flexDirection: "row", alignItems: "center", gap: 12 },
  icon: { width: 44, height: 44, borderRadius: 14, alignItems: "center", justifyContent: "center" },
  iconText: { fontSize: 22 },
  tileText: { flex: 1, gap: 2 },
  title: { fontSize: 17, fontWeight: "600" },
  sub: { fontSize: 13 },
  small: { fontSize: 12.5 },
  levelTrack: { height: 4, borderRadius: 2, overflow: "hidden" },
  levelFill: { height: 4, borderRadius: 2, backgroundColor: "#3B6BFF" },
  cameraBody: { gap: 8 },
  dim: { gap: 8 },
  primary: { borderRadius: 14, paddingVertical: 14, alignItems: "center" },
  primaryText: { color: "#FFFFFF", fontSize: 16, fontWeight: "600" },
  quiet: { borderRadius: 14, paddingVertical: 14, alignItems: "center" },
  quietText: { fontSize: 16, fontWeight: "600" }
})
