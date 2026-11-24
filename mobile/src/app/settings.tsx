import { Effect } from "effect"
import * as Application from "expo-application"
import { useRouter } from "expo-router"
import { useState } from "react"
import { Alert, ScrollView, Text, View } from "react-native"
import { CameraPrefs } from "../core/cameraPrefs"
import { allowedAt, FREE_BUILD } from "../core/entitlement"
import { PRIVACY_URL, REPO_URL, SUPPORT_URL } from "../core/links"
import { AUTO_DIM_OPTIONS, ScreenPrefs } from "../core/screenPrefs"
import { Session } from "../core/session"
import { act, useEntitlement, usePalette, useScreenChoices } from "../ui/hooks"
import { openUrl } from "../ui/open"
import { Label, NavRow, Segments } from "../ui/parts"
import { PLAN_NAME } from "../ui/plans"

const AUTO_DIM_LABEL: Record<number, string> = { 0: "Never", 15: "15 s", 30: "30 s", 60: "1 min" }

/** Asks first, then forgets what was saved for every PC (and puts a connected one back to the defaults too). */
const resetCameraChoices = () =>
  Alert.alert("Reset camera choices?", "Every PC goes back to the defaults.", [
    { text: "Cancel", style: "cancel" },
    {
      text: "Reset",
      style: "destructive",
      onPress: () =>
        act(
          Session.use((s) => s.resetCamera).pipe(Effect.andThen(CameraPrefs.use((p) => p.resetAll)))
        )
    }
  ])

/** A few short rows. Nothing here needs a sentence to explain it. */
export default function Settings() {
  const router = useRouter()
  const p = usePalette()
  // The subscription must be reachable without a PC (App Review has none).
  const entitlement = useEntitlement()
  const [refresh, setRefresh] = useState(0)
  const { autoDimSeconds } = useScreenChoices(refresh)
  const held = allowedAt(entitlement, Date.now())
  const plan =
    entitlement._tag === "Active" && entitlement.plan !== null ? PLAN_NAME[entitlement.plan] : null
  const version = `${Application.nativeApplicationVersion ?? "?"} (${Application.nativeBuildVersion ?? "?"})`

  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: p.bg }}
      contentInsetAdjustmentBehavior="automatic"
      contentContainerStyle={{ padding: 16, gap: 14 }}
    >
      {FREE_BUILD ? null : (
        <NavRow
          p={p}
          label="Camera subscription"
          value={held ? (plan === null ? "Subscribed" : `Subscribed · ${plan}`) : "Not subscribed"}
          onPress={() => router.push("/subscription")}
        />
      )}

      <View style={{ gap: 8 }}>
        <Label p={p} text="DIM SCREEN AFTER" />
        <Segments
          p={p}
          items={AUTO_DIM_OPTIONS.map((seconds) => ({
            key: String(seconds),
            label: AUTO_DIM_LABEL[seconds] ?? `${seconds} s`,
            on: autoDimSeconds === seconds,
            onPress: () =>
              act(
                ScreenPrefs.use((s) => s.save({ autoDimSeconds: seconds })).pipe(
                  Effect.andThen(Effect.sync(() => setRefresh((n) => n + 1)))
                )
              )
          }))}
        />
      </View>

      <NavRow p={p} label="Reset camera choices" onPress={resetCameraChoices} />

      <View style={{ gap: 8 }}>
        <Label p={p} text="HELP" />
        <NavRow p={p} label="How it works" onPress={() => router.push("/help")} />
        <NavRow p={p} label="Support" onPress={() => openUrl(SUPPORT_URL)} />
        <NavRow p={p} label="Privacy policy" onPress={() => openUrl(PRIVACY_URL)} />
        <NavRow p={p} label="Source code (MIT)" onPress={() => openUrl(REPO_URL)} />
      </View>

      <Text
        style={{ color: p.sub, fontSize: 12.5, textAlign: "center" }}
      >{`phoneset ${version}`}</Text>
    </ScrollView>
  )
}
