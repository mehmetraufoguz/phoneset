import { Button, FieldGroup, Host, Row, Slider, Spacer, Switch, Text } from "@expo/ui"
import { useKeepAwake } from "expo-keep-awake"
import { useRouter } from "expo-router"
import { Platform } from "react-native"
import { Session, type SessionState, type StreamStatus } from "../core/session"
import { act, BRAND, useColors, useSessionState } from "../ui/hooks"

export default function SessionScreen() {
  const router = useRouter()
  const state = useSessionState()
  const colors = useColors()

  // The phone is a live microphone here: keep it awake while this screen is open.
  useKeepAwake()

  // Going back (header button or swipe) keeps the connection: it shows on the home screen and
  // in the notification. Only these buttons end it.
  const disconnect = () => {
    act(Session.use((s) => s.stop))
    router.back()
  }

  return (
    <Host style={{ flex: 1 }} seedColor={BRAND}>
      <FieldGroup>
        {state._tag === "Connected" ? (
          <Connected state={state} onDisconnect={disconnect} />
        ) : state._tag === "Failed" ? (
          <FieldGroup.Section title="Couldn't connect">
            <Text textStyle={{ color: colors.danger }}>{state.message}</Text>
            {state.code === "unknown_device" || state.code === "bad_server" ? null : (
              <Button label="Try again" onPress={() => act(Session.use((s) => s.retry))} />
            )}
            <Button variant="outlined" label="Back" onPress={disconnect} />
          </FieldGroup.Section>
        ) : (
          <FieldGroup.Section title={state._tag === "Connecting" ? state.pcName : "phoneset"}>
            <Text textStyle={{ color: colors.secondary }}>
              {state._tag === "Connecting" ? `${state.step}…` : "Not connected"}
            </Text>
            <Button variant="outlined" label="Cancel" onPress={disconnect} />
          </FieldGroup.Section>
        )}
      </FieldGroup>
    </Host>
  )
}

function Connected({
  state,
  onDisconnect
}: {
  state: Extract<SessionState, { _tag: "Connected" }>
  onDisconnect: () => void
}) {
  const colors = useColors()

  const detail = (s: StreamStatus, on: string, off: string) =>
    s.error ?? (s.on ? on : s.available ? off : "Not available on this PC")

  const diagnostics = [
    state.mic.on && state.stats
      ? `Mic buffer ${state.stats.mic.bufferMs} ms, ${state.stats.mic.underruns} gaps`
      : "",
    state.speaker.on && state.stats ? `${state.stats.speaker.dropped} frames dropped` : ""
  ]
    .filter((line) => line !== "")
    .join("  ·  ")

  return (
    <>
      <FieldGroup.Section title={state.pc.name}>
        <Row alignment="center" spacing={8}>
          <Text textStyle={{ color: colors.ok, fontWeight: "600" }}>Connected</Text>
          <Spacer flexible />
          <Text textStyle={{ color: colors.secondary }}>
            {state.rttMs === null ? "measuring…" : `${state.rttMs} ms`}
          </Text>
        </Row>
      </FieldGroup.Section>

      <FieldGroup.Section title="Use this phone as">
        <Switch
          label="Microphone"
          value={state.mic.on}
          disabled={state.mic.busy || !state.mic.available}
          onValueChange={(on) => act(Session.use((s) => s.setMic(on)))}
        />
        <Text
          textStyle={{ fontSize: 13, color: state.mic.error ? colors.danger : colors.secondary }}
        >
          {detail(state.mic, `Sending to ${state.mic.device ?? "the PC"}`, "Off")}
        </Text>
        {state.mic.on ? (
          <Slider
            value={Math.min(1, Math.sqrt(state.micLevel) * 1.6)}
            onValueChange={() => {}}
            disabled
          />
        ) : null}

        <Switch
          label="Headphones"
          value={state.speaker.on}
          disabled={state.speaker.busy || !state.speaker.available}
          onValueChange={(on) => act(Session.use((s) => s.setSpeaker(on)))}
        />
        <Text
          textStyle={{
            fontSize: 13,
            color: state.speaker.error ? colors.danger : colors.secondary
          }}
        >
          {detail(state.speaker, `Playing ${state.speaker.device ?? "PC audio"}`, "Off")}
        </Text>
      </FieldGroup.Section>

      <FieldGroup.Section title="Sound">
        {Platform.OS === "ios" ? (
          <>
            <Switch
              label="Echo cancellation"
              value={state.echoCancellation}
              onValueChange={(on) => act(Session.use((s) => s.setVoiceProcessing(on)))}
            />
            <Text textStyle={{ fontSize: 13, color: colors.secondary }}>
              Keeps the sound from the phone's own speaker out of its microphone. It puts the phone
              in its voice-call audio mode, so the PC's audio may sound different while it is on.
              Earphones avoid echo altogether.
            </Text>
          </>
        ) : (
          <Text textStyle={{ fontSize: 13, color: colors.secondary }}>
            Echo cancellation isn't available on Android yet. Earphones avoid echo altogether.
          </Text>
        )}
        {diagnostics === "" ? null : (
          <Text textStyle={{ fontSize: 13, color: colors.secondary }}>{diagnostics}</Text>
        )}
      </FieldGroup.Section>

      <FieldGroup.Section>
        <Button variant="outlined" label="Disconnect" onPress={onDisconnect} />
      </FieldGroup.Section>
    </>
  )
}
