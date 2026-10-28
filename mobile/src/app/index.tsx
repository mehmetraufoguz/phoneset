import { Button, Column, FieldGroup, Host, Row, Spacer, Text } from "@expo/ui"
import { Stack, useFocusEffect, useRouter } from "expo-router"
import { useCallback, useState } from "react"
import { Alert } from "react-native"
import { Session } from "../core/session"
import { type PairedPc, PairedPcs } from "../core/storage"
import { act, BRAND, useColors, useDiscovered, usePairedPcs } from "../ui/hooks"

export default function Home() {
  const router = useRouter()
  const colors = useColors()
  const [refresh, setRefresh] = useState(0)
  useFocusEffect(useCallback(() => setRefresh((n) => n + 1), []))

  const pcs = usePairedPcs(refresh)
  // Scanning here also raises the iOS "Local Network" prompt early, not mid-pairing.
  const nearby = useDiscovered()
  const visible = new Set(nearby.map((d) => d.id))

  const connect = (pc: PairedPc) => {
    act(Session.use((s) => s.start({ kind: "paired", pc })))
    router.push("/session")
  }

  const forget = (pc: PairedPc) =>
    Alert.alert(`Forget ${pc.name}?`, "You'll need to pair with it again to use it.", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Forget",
        style: "destructive",
        onPress: () => {
          act(PairedPcs.use((p) => p.remove(pc.id)))
          setRefresh((n) => n + 1)
        }
      }
    ])

  return (
    <>
      <Stack.Toolbar placement="right">
        <Stack.Toolbar.Button icon="plus" onPress={() => router.push("/pair")} />
      </Stack.Toolbar>
      <Host style={{ flex: 1 }} seedColor={BRAND}>
        <FieldGroup>
          <FieldGroup.Section title="Your PCs">
            {pcs.length === 0 ? (
              <Text textStyle={{ color: colors.secondary }}>
                No PC yet. Start phoneset on your PC, then tap + to pair it.
              </Text>
            ) : (
              pcs.map((pc) => (
                <Row key={pc.id} alignment="center" spacing={8}>
                  <Button variant="text" onPress={() => connect(pc)}>
                    <Column spacing={2}>
                      <Text textStyle={{ fontSize: 17, fontWeight: "600" }}>{pc.name}</Text>
                      <Text textStyle={{ fontSize: 13, color: colors.secondary }}>
                        {visible.has(pc.id) ? "On this Wi-Fi · tap to connect" : "Tap to connect"}
                      </Text>
                    </Column>
                  </Button>
                  <Spacer flexible />
                  <Button variant="text" label="Forget" onPress={() => forget(pc)} />
                </Row>
              ))
            )}
          </FieldGroup.Section>
        </FieldGroup>
      </Host>
    </>
  )
}
