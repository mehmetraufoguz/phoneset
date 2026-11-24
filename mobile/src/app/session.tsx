import { Button, FieldGroup, Host, Text } from "@expo/ui"
import { useKeepAwake } from "expo-keep-awake"
import { useRouter } from "expo-router"
import { useCallback, useEffect, useRef, useState } from "react"
import { ScrollView, View } from "react-native"
import { Session } from "../core/session"
import { ConnectedView } from "../ui/connectedView"
import { DimScreen } from "../ui/dimScreen"
import { act, BRAND, useColors, usePalette, useScreenChoices, useSessionState } from "../ui/hooks"

export default function SessionScreen() {
  const router = useRouter()
  const state = useSessionState()
  const colors = useColors()
  const palette = usePalette()
  const { autoDimSeconds } = useScreenChoices()

  // The phone is a live microphone here: keep it awake while this screen is open.
  useKeepAwake()

  // Going back (header button or swipe) keeps the connection: it shows on the home screen and
  // in the notification. Only these buttons end it.
  const disconnect = () => {
    act(Session.use((s) => s.stop))
    router.back()
  }

  // Dim screen: a black cover over the controls while the camera runs. It starts by itself after `autoDimSeconds` without
  // a touch (when that is not 0) and only while the camera is on; a touch on the controls starts the wait again.
  const cameraOn = state._tag === "Connected" && state.video.on
  const [dimmed, setDimmed] = useState(false)
  const [since, setSince] = useState(() => Date.now())
  const idle = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  useEffect(() => {
    if (cameraOn) setSince(Date.now())
    else setDimmed(false)
  }, [cameraOn])

  const arm = useCallback(() => {
    clearTimeout(idle.current)
    if (cameraOn && autoDimSeconds > 0 && !dimmed) {
      idle.current = setTimeout(() => setDimmed(true), autoDimSeconds * 1000)
    }
  }, [cameraOn, autoDimSeconds, dimmed])

  useEffect(() => {
    arm()
    return () => clearTimeout(idle.current)
  }, [arm])

  if (state._tag === "Connected") {
    return (
      <View style={{ flex: 1, backgroundColor: palette.bg }} onTouchStart={arm}>
        <ScrollView contentInsetAdjustmentBehavior="automatic">
          <ConnectedView
            state={state}
            autoDimSeconds={autoDimSeconds}
            onDim={() => setDimmed(true)}
            onMore={() => router.push("/more")}
            onDisconnect={disconnect}
          />
        </ScrollView>
        <DimScreen visible={dimmed && cameraOn} since={since} onWake={() => setDimmed(false)} />
      </View>
    )
  }

  return (
    <Host style={{ flex: 1 }} seedColor={BRAND}>
      <FieldGroup>
        {state._tag === "Failed" ? (
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
