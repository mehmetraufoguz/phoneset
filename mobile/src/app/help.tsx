import { Button, FieldGroup, Host, Text } from "@expo/ui"
import { useRouter } from "expo-router"
import { INSTALL_URL } from "../core/links"
import { BRAND, useColors } from "../ui/hooks"
import { openUrl } from "../ui/open"

export default function Help() {
  const router = useRouter()
  const colors = useColors()
  const body = { fontSize: 15, color: colors.secondary }

  return (
    <Host style={{ flex: 1 }} seedColor={BRAND}>
      <FieldGroup>
        <FieldGroup.Section title="1. Put phoneset on your PC">
          <Text textStyle={body}>
            This app is the phone half. The PC half is a small free program for Windows 10/11 and
            macOS, and one command installs it.
          </Text>
          <Button
            variant="outlined"
            label="Open the install page"
            onPress={() => openUrl(INSTALL_URL)}
          />
        </FieldGroup.Section>

        <FieldGroup.Section title="2. Start it">
          <Text textStyle={body}>
            Run phoneset in a terminal, or phoneset daemon to keep it running in the background. It
            shows a QR code and a 6-digit code.
          </Text>
        </FieldGroup.Section>

        <FieldGroup.Section title="3. Pair this phone">
          <Text textStyle={body}>
            Put the phone and the PC on the same Wi-Fi. Tap + on the home screen, then scan the QR
            code or type the 6-digit code.
          </Text>
          <Button label="Pair a PC" onPress={() => router.push("/pair")} />
        </FieldGroup.Section>

        <FieldGroup.Section title="4. Use it">
          <Text textStyle={body}>
            Open the PC in the list and switch on Microphone, Headphones or both. Apps on the PC
            hear your phone's microphone through a virtual audio cable (VB-CABLE on Windows,
            BlackHole on macOS), and what the PC plays comes out of the phone.
          </Text>
        </FieldGroup.Section>

        <FieldGroup.Section title="Good to know">
          <Text textStyle={body}>
            Audio goes straight between the phone and the PC, encrypted. It doesn't pass through any
            server, and the app has no account.
          </Text>
        </FieldGroup.Section>
      </FieldGroup>
    </Host>
  )
}
