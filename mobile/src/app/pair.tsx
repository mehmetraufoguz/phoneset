import { useLocalSearchParams, useRouter } from "expo-router"
import { useEffect, useRef, useState } from "react"
import { Alert, Pressable, ScrollView, Text, TextInput } from "react-native"
import { DEFAULT_PORT } from "../core/constants"
import type { DiscoveredPc } from "../core/discovery"
import { parsePairLink } from "../core/link"
import { Session, type Target } from "../core/session"
import { runtime } from "../runtime"
import { act, useDiscovered, usePalette } from "../ui/hooks"
import { Card, NavRow, PrimaryButton, QuietButton } from "../ui/parts"

const digits = (s: string) => s.replace(/\D/g, "")
const enc = (s: string | undefined) => encodeURIComponent(s ?? "")

export default function Pair() {
  const router = useRouter()
  const p = usePalette()
  const nearby = useDiscovered()
  const [code, setCode] = useState("")
  const [address, setAddress] = useState("")
  const [showAddress, setShowAddress] = useState(false)

  const begin = (target: Target) => {
    act(Session.use((s) => s.start(target)))
    router.replace("/session")
  }

  const withCode = (go: (code: string) => void) => {
    const entered = digits(code)
    if (entered.length !== 6) {
      Alert.alert(
        "Enter the code",
        "Type the 6-digit code shown in the phoneset window on your PC."
      )
      return
    }
    go(entered)
  }

  const pairWith = (pc: DiscoveredPc) =>
    withCode((entered) =>
      begin({
        kind: "code",
        code: entered,
        name: pc.name,
        hosts: pc.hosts,
        port: pc.port,
        pcId: pc.id
      })
    )

  const pairWithAddress = () =>
    withCode((entered) => {
      const host = address.trim()
      if (!/^[A-Za-z0-9.-]+$/.test(host)) {
        Alert.alert("Check the address", "Use the address shown on your PC, like 192.168.1.20.")
        return
      }
      begin({ kind: "code", code: entered, name: host, hosts: [host], port: DEFAULT_PORT })
    })

  // Opened by the system camera app scanning the PC's QR code (phoneset://pair?...).
  const { id, n, h, p: port, k } = useLocalSearchParams<Record<string, string | undefined>>()
  const handled = useRef<string | null>(null)
  useEffect(() => {
    if (!k || handled.current === k) return
    handled.current = k
    const link = `phoneset://pair?v=2&id=${enc(id)}&n=${enc(n)}&h=${enc(h)}&p=${enc(port)}&k=${enc(k)}`
    runtime
      .runPromise(parsePairLink(link))
      .then((parsed) =>
        // Anyone can craft a link; make the person confirm which PC this is.
        Alert.alert(`Pair with ${parsed.name}?`, `${parsed.hosts[0]} on this network`, [
          { text: "Cancel", style: "cancel" },
          {
            text: "Pair",
            onPress: () => {
              act(Session.use((s) => s.start({ kind: "qr", link: parsed })))
              router.replace("/session")
            }
          }
        ])
      )
      .catch(() =>
        Alert.alert("Not a phoneset code", "Scan the QR code shown by phoneset on your PC.")
      )
  }, [id, n, h, port, k, router])

  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: p.bg }}
      contentInsetAdjustmentBehavior="automatic"
      keyboardShouldPersistTaps="handled"
      contentContainerStyle={{ padding: 16, gap: 14 }}
    >
      <PrimaryButton label="Scan QR code" onPress={() => router.push("/scan")} />
      <Text style={{ color: p.sub, textAlign: "center", fontSize: 13 }}>or type the code</Text>
      <TextInput
        value={code}
        onChangeText={setCode}
        placeholder="000 000"
        placeholderTextColor={p.sub}
        keyboardType="number-pad"
        inputMode="numeric"
        maxLength={7}
        autoCorrect={false}
        style={{
          backgroundColor: p.surface,
          borderColor: p.line,
          borderWidth: 1,
          borderRadius: 16,
          color: p.text,
          fontSize: 28,
          fontVariant: ["tabular-nums"],
          letterSpacing: 4,
          paddingVertical: 14,
          textAlign: "center"
        }}
      />
      {nearby.length === 0 ? (
        <Text style={{ color: p.sub, textAlign: "center", fontSize: 13 }}>
          Looking for PCs on this Wi-Fi…
        </Text>
      ) : (
        nearby.map((pc) => (
          <NavRow key={pc.id} p={p} label={`Pair with ${pc.name}`} onPress={() => pairWith(pc)} />
        ))
      )}
      <Pressable onPress={() => setShowAddress((v) => !v)} accessibilityRole="button">
        <Text style={{ color: "#3B6BFF", textAlign: "center", fontSize: 14, padding: 6 }}>
          PC not listed?
        </Text>
      </Pressable>
      {showAddress ? (
        <Card p={p}>
          <TextInput
            value={address}
            onChangeText={setAddress}
            placeholder="PC address, e.g. 192.168.1.20"
            placeholderTextColor={p.sub}
            keyboardType="numbers-and-punctuation"
            autoCapitalize="none"
            autoCorrect={false}
            style={{ color: p.text, fontSize: 16, paddingVertical: 6 }}
          />
          <QuietButton p={p} label="Pair with this address" onPress={pairWithAddress} />
        </Card>
      ) : null}
    </ScrollView>
  )
}
