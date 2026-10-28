import { Button, Column, Host, Text } from "@expo/ui"
import { CameraView, useCameraPermissions } from "expo-camera"
import * as Haptics from "expo-haptics"
import { useRouter } from "expo-router"
import { useRef } from "react"
import { Linking, StyleSheet, View } from "react-native"
import { parsePairLink } from "../core/link"
import { Session } from "../core/session"
import { runtime } from "../runtime"
import { act, BRAND } from "../ui/hooks"

export default function Scan() {
  const router = useRouter()
  const [permission, requestPermission] = useCameraPermissions()
  const busy = useRef(false)

  const onScanned = ({ data }: { data: string }) => {
    if (busy.current) return
    busy.current = true
    runtime
      .runPromise(parsePairLink(data))
      .then((link) => {
        Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success)
        act(Session.use((s) => s.start({ kind: "qr", link })))
        router.dismissTo("/")
        router.push("/session")
      })
      .catch(() => {
        // Not ours (a stray QR code in view): keep looking.
        setTimeout(() => {
          busy.current = false
        }, 1200)
      })
  }

  if (!permission?.granted) {
    return (
      <Host style={{ flex: 1 }} seedColor={BRAND}>
        <Column spacing={16} style={{ padding: 24 }}>
          <Text textStyle={{ fontSize: 17 }}>
            phoneset needs the camera to read the QR code on your PC's screen.
          </Text>
          <Button
            label={permission?.canAskAgain === false ? "Open camera settings" : "Allow camera"}
            onPress={() =>
              permission?.canAskAgain === false ? Linking.openSettings() : requestPermission()
            }
          />
          <Button variant="text" label="Enter the code instead" onPress={() => router.back()} />
        </Column>
      </Host>
    )
  }

  return (
    <View style={styles.fill}>
      <CameraView
        style={styles.fill}
        facing="back"
        barcodeScannerSettings={{ barcodeTypes: ["qr"] }}
        onBarcodeScanned={onScanned}
      />
      <View pointerEvents="none" style={styles.frameWrap}>
        <View style={styles.frame} />
      </View>
    </View>
  )
}

const styles = StyleSheet.create({
  fill: { flex: 1, backgroundColor: "black" },
  frameWrap: { ...StyleSheet.absoluteFill, alignItems: "center", justifyContent: "center" },
  frame: {
    width: 240,
    height: 240,
    borderRadius: 24,
    borderWidth: 3,
    borderColor: "rgba(255,255,255,0.85)"
  }
})
