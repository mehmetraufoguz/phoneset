import { useRouter } from "expo-router"
import { Platform, ScrollView, Switch, Text, View } from "react-native"
import { FREE_BUILD } from "../core/entitlement"
import { Session, type SessionState } from "../core/session"
import { modesOfFormat, TIERS, tiersOffered, type VideoFormat } from "../core/videoModes"
import { act, usePalette, useSessionState } from "../ui/hooks"
import { Card, Label, NavRow, Segments } from "../ui/parts"

type Connected = Extract<SessionState, { _tag: "Connected" }>

const FORMATS: ReadonlyArray<{ key: VideoFormat; label: string; hint: string }> = [
  { key: "auto", label: "Auto", hint: "The app picks H.264 when it can" },
  { key: "h264", label: "H.264", hint: "Small and smooth" },
  { key: "jpeg", label: "JPEG", hint: "For programs that read plain MJPEG. Uses much more Wi-Fi" }
]

/**
 * What is rarely needed while a PC is connected: the camera's exact size, smoothness and format, echo cancellation,
 * and a way to the subscription and the help. Each control says what it does in a few words; nothing here is a
 * paragraph.
 */
export default function More() {
  const router = useRouter()
  const state = useSessionState()
  const p = usePalette()

  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: p.bg }}
      contentInsetAdjustmentBehavior="automatic"
      contentContainerStyle={{ padding: 16, gap: 14 }}
    >
      {state._tag === "Connected" ? <ConnectedOptions state={state} p={p} /> : null}
      {FREE_BUILD ? null : (
        <NavRow p={p} label="Camera subscription" onPress={() => router.push("/settings")} />
      )}
      <NavRow p={p} label="How it works" onPress={() => router.push("/help")} />
    </ScrollView>
  )
}

function ConnectedOptions({
  state,
  p
}: {
  readonly state: Connected
  readonly p: ReturnType<typeof usePalette>
}) {
  const v = state.video

  // The sizes and rates this phone and PC offer (known once the camera has been on); until then every planned size is listed.
  const offered = v.report === null ? null : tiersOffered(modesOfFormat(v.report.offered, v.format))
  const sizes = (offered ?? TIERS.map((t) => ({ height: t.height, name: t.name, maxFps: 60 }))).map(
    (t) => ({ height: t.height, name: t.name })
  )
  // What was picked stays in the list, so the control never shows a value that is not in it.
  if (!sizes.some((t) => t.height === v.quality)) {
    const tier = TIERS.find((t) => t.height === v.quality)
    sizes.push({ height: v.quality, name: tier?.name ?? `${v.quality}p` })
    sizes.sort((a, b) => a.height - b.height)
  }
  const sixty = offered === null || offered.some((t) => t.height === v.quality && t.maxFps >= 60)

  const diagnostics = [
    state.mic.on && state.stats
      ? `Mic buffer ${state.stats.mic.bufferMs} ms, ${state.stats.mic.underruns} gaps`
      : "",
    state.speaker.on && state.stats ? `${state.stats.speaker.dropped} frames dropped` : ""
  ].filter((line) => line !== "")

  return (
    <>
      {v.unavailable === null ? (
        <View style={{ gap: 8 }}>
          <Label p={p} text="CAMERA" />
          <Card p={p}>
            <Label p={p} text="SIZE" />
            <Segments
              p={p}
              items={sizes.map((t) => ({
                key: String(t.height),
                label: t.name,
                on: v.quality === t.height,
                disabled: v.busy,
                onPress: () => act(Session.use((s) => s.setQuality(t.height)))
              }))}
            />
            <Label p={p} text="SMOOTHNESS" />
            <Segments
              p={p}
              items={[30, 60].map((fps) => ({
                key: String(fps),
                label: `${fps} fps`,
                on: v.rate === fps,
                disabled: v.busy || (fps === 60 && !sixty && v.rate !== 60),
                onPress: () => act(Session.use((s) => s.setRate(fps)))
              }))}
            />
            <Label p={p} text="FORMAT" />
            <Segments
              p={p}
              items={FORMATS.map((f) => ({
                key: f.key,
                label: f.label,
                on: v.format === f.key,
                disabled: v.busy,
                onPress: () => act(Session.use((s) => s.setFormat(f.key)))
              }))}
            />
            <Text style={{ color: p.sub, fontSize: 12.5 }}>
              {FORMATS.find((f) => f.key === v.format)?.hint}
            </Text>
          </Card>
          <NavRow
            p={p}
            label="Reset camera to defaults"
            onPress={() => act(Session.use((s) => s.resetCamera))}
          />
        </View>
      ) : null}

      {Platform.OS === "ios" ? (
        <View style={{ gap: 8 }}>
          <Label p={p} text="SOUND" />
          <Card p={p}>
            <View style={{ flexDirection: "row", alignItems: "center", gap: 12 }}>
              <View style={{ flex: 1 }}>
                <Text style={{ color: p.text, fontSize: 16, fontWeight: "600" }}>
                  Echo cancellation
                </Text>
                <Text style={{ color: p.sub, fontSize: 12.5 }}>
                  Keeps the speaker out of the mic. Use earphones to avoid echo
                </Text>
              </View>
              <Switch
                value={state.echoCancellation}
                onValueChange={(on) => act(Session.use((s) => s.setVoiceProcessing(on)))}
                trackColor={{ true: p.ok }}
              />
            </View>
          </Card>
        </View>
      ) : null}

      {diagnostics.length === 0 ? null : (
        <Text style={{ color: p.sub, fontSize: 12.5, paddingHorizontal: 4 }}>
          {diagnostics.join("  ·  ")}
        </Text>
      )}
    </>
  )
}
