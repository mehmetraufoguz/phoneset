import { Button, FieldGroup, Host, Row, Spacer, Text } from "@expo/ui"
import * as Application from "expo-application"
import { useRouter } from "expo-router"
import { LICENSE_URL, PRIVACY_URL, REPO_URL, SUPPORT_URL } from "../core/links"
import { BRAND, useColors } from "../ui/hooks"
import { openUrl } from "../ui/open"

export default function Settings() {
  const router = useRouter()
  const colors = useColors()
  const version = `${Application.nativeApplicationVersion ?? "?"} (${Application.nativeBuildVersion ?? "?"})`

  return (
    <Host style={{ flex: 1 }} seedColor={BRAND}>
      <FieldGroup>
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
