import { useRouter } from "expo-router"
import { ScrollView, StyleSheet, Text, View } from "react-native"
import { INSTALL_URL } from "../core/links"
import { usePalette } from "../ui/hooks"
import { openUrl } from "../ui/open"
import { Card, PrimaryButton, QuietButton } from "../ui/parts"

/** Four steps, a few words each. The buttons do the step, so nothing has to be explained. */
export default function Help() {
  const router = useRouter()
  const p = usePalette()

  const step = (n: number, icon: string, title: string, sub?: string) => (
    <View style={styles.step}>
      <View style={[styles.num, { backgroundColor: p.raised }]}>
        <Text style={{ fontSize: 22 }}>{icon}</Text>
      </View>
      <View style={{ flex: 1 }}>
        <Text style={{ color: p.text, fontSize: 17, fontWeight: "600" }}>{`${n}. ${title}`}</Text>
        {sub === undefined ? null : <Text style={{ color: p.sub, fontSize: 13 }}>{sub}</Text>}
      </View>
    </View>
  )

  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: p.bg }}
      contentInsetAdjustmentBehavior="automatic"
      contentContainerStyle={{ padding: 16, gap: 12 }}
    >
      <Card p={p}>
        {step(1, "💻", "Install on your PC", "Windows or Mac, free")}
        <QuietButton p={p} label="Open the install page" onPress={() => openUrl(INSTALL_URL)} />
      </Card>
      <Card p={p}>{step(2, "▶️", "Run phoneset", "It shows a QR code")}</Card>
      <Card p={p}>
        {step(3, "📱", "Scan the QR code", "Same Wi-Fi as the PC")}
        <PrimaryButton label="Pair a PC" onPress={() => router.push("/pair")} />
      </Card>
      <Card p={p}>{step(4, "🎙️", "Switch on what you need", "Microphone, headphones, camera")}</Card>
      <Text style={{ color: p.sub, fontSize: 12.5, textAlign: "center" }}>
        No account. Sound and picture go straight to your PC.
      </Text>
    </ScrollView>
  )
}

const styles = StyleSheet.create({
  step: { flexDirection: "row", alignItems: "center", gap: 12 },
  num: { width: 44, height: 44, borderRadius: 14, alignItems: "center", justifyContent: "center" }
})
