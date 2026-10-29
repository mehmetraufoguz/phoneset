import { Button, FieldGroup, Host, Text, TextInput } from "@expo/ui"
import { useLocalSearchParams, useRouter } from "expo-router"
import { useEffect, useRef, useState } from "react"
import { Alert } from "react-native"
import { DEFAULT_PORT } from "../core/constants"
import type { DiscoveredPc } from "../core/discovery"
import { parsePairLink } from "../core/link"
import { Session, type Target } from "../core/session"
import { runtime } from "../runtime"
import { act, BRAND, useColors, useDiscovered } from "../ui/hooks"

const digits = (s: string) => s.replace(/\D/g, "")
const enc = (s: string | undefined) => encodeURIComponent(s ?? "")

export default function Pair() {
  const router = useRouter()
  const colors = useColors()
  const nearby = useDiscovered()
  const [code, setCode] = useState("")
  const [address, setAddress] = useState("")

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
  const { id, n, h, p, k } = useLocalSearchParams<Record<string, string | undefined>>()
  const handled = useRef<string | null>(null)
  useEffect(() => {
    if (!k || handled.current === k) return
    handled.current = k
    const link = `phoneset://pair?v=2&id=${enc(id)}&n=${enc(n)}&h=${enc(h)}&p=${enc(p)}&k=${enc(k)}`
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
  }, [id, n, h, p, k, router])

  return (
    <Host style={{ flex: 1 }} seedColor={BRAND}>
      <FieldGroup>
        <FieldGroup.Section title="With the QR code">
          <Button label="Scan QR code" onPress={() => router.push("/scan")} />
        </FieldGroup.Section>

        <FieldGroup.Section title="Or with the 6-digit code">
          <TextInput
            placeholder="123 456"
            keyboardType="number-pad"
            inputMode="numeric"
            maxLength={7}
            autoCorrect={false}
            onChangeText={setCode}
          />
          {nearby.length === 0 ? (
            <Text textStyle={{ color: colors.secondary }}>Looking for PCs on this Wi-Fi…</Text>
          ) : (
            nearby.map((pc) => (
              <Button
                key={pc.id}
                variant="text"
                label={`Pair with ${pc.name}`}
                onPress={() => pairWith(pc)}
              />
            ))
          )}
        </FieldGroup.Section>

        <FieldGroup.Section title="Can't see your PC?">
          <TextInput
            placeholder="PC address, e.g. 192.168.1.20"
            keyboardType="numbers-and-punctuation"
            autoCapitalize="none"
            autoCorrect={false}
            onChangeText={setAddress}
          />
          <Button variant="outlined" label="Pair with this address" onPress={pairWithAddress} />
        </FieldGroup.Section>
      </FieldGroup>
    </Host>
  )
}
