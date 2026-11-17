import { Button, FieldGroup, Host, Row, Spacer, Text } from "@expo/ui"
import { Effect } from "effect"
import * as Application from "expo-application"
import { useRouter } from "expo-router"
import { Alert } from "react-native"
import { CameraPrefs } from "../core/cameraPrefs"
import { LICENSE_URL, PRIVACY_URL, REPO_URL, SUPPORT_URL } from "../core/links"
import { Session } from "../core/session"
import { act, BRAND, useColors, useEntitlement, useOffers, useStoreActions } from "../ui/hooks"
import { openUrl } from "../ui/open"
import { subscriptionSection } from "../ui/subscriptionSection"

/** Asks first, then forgets what was saved for every PC (and puts a connected one back to the defaults too). */
const resetCameraChoices = () =>
  Alert.alert("Reset camera choices?", "Every PC goes back to the default camera settings.", [
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

export default function Settings() {
  const router = useRouter()
  const colors = useColors()
  // The subscription can be bought, restored and managed here too, not only on the screen of a connected PC: it must be
  // reachable without a PC (App Review has none).
  const subscription = { state: useEntitlement(), offers: useOffers(), actions: useStoreActions() }
  const version = `${Application.nativeApplicationVersion ?? "?"} (${Application.nativeBuildVersion ?? "?"})`

  return (
    <Host style={{ flex: 1 }} seedColor={BRAND}>
      <FieldGroup>
        {subscriptionSection(subscription, colors)}

        <FieldGroup.Section title="Camera">
          <Button variant="text" label="Reset camera choices" onPress={resetCameraChoices} />
          <FieldGroup.SectionFooter>
            <Text textStyle={{ fontSize: 13, color: colors.secondary }}>
              The camera, size, smoothness and format you pick are remembered for each PC. This puts
              them back to the defaults for every PC, whether or not one is connected.
            </Text>
          </FieldGroup.SectionFooter>
        </FieldGroup.Section>

        <FieldGroup.Section title="Help">
          <Button variant="text" label="How it works" onPress={() => router.push("/help")} />
          <Button variant="text" label="Support" onPress={() => openUrl(SUPPORT_URL)} />
          <Button variant="text" label="Privacy policy" onPress={() => openUrl(PRIVACY_URL)} />
        </FieldGroup.Section>

        <FieldGroup.Section title="Open source">
          <Button variant="text" label="Source code" onPress={() => openUrl(REPO_URL)} />
          <Button variant="text" label="MIT license" onPress={() => openUrl(LICENSE_URL)} />
        </FieldGroup.Section>

        <FieldGroup.Section title="About">
          <Row alignment="center" spacing={8}>
            <Text>Version</Text>
            <Spacer flexible />
            <Text textStyle={{ color: colors.secondary }}>{version}</Text>
          </Row>
          <Text textStyle={{ fontSize: 13, color: colors.secondary }}>
            phoneset sends audio between this phone and your own PC over your Wi-Fi. It has no
            account, no analytics and no server of its own.
          </Text>
        </FieldGroup.Section>
      </FieldGroup>
    </Host>
  )
}
