import { Button, Column, FieldGroup, Host, Row, Spacer, Text } from "@expo/ui"
import { Stack, useFocusEffect, useRouter } from "expo-router"
import { useCallback, useState } from "react"
import { Alert } from "react-native"
import { summarize } from "../core/presence"
import { Session } from "../core/session"
import { type PairedPc, PairedPcs } from "../core/storage"
import { act, BRAND, useColors, useDiscovered, usePairedPcs, useSessionState } from "../ui/hooks"
import { ToolbarButton } from "../ui/toolbar"

export default function Home() {
  const router = useRouter()
  const colors = useColors()
  const [refresh, setRefresh] = useState(0)
  useFocusEffect(useCallback(() => setRefresh((n) => n + 1), []))

  const pcs = usePairedPcs(refresh)
  // Scanning here also raises the iOS "Local Network" prompt early, not mid-pairing.
  const nearby = useDiscovered()
  const visible = new Set(nearby.map((d) => d.id))
  const session = useSessionState()

  // One phone talks to one PC at a time, so "connected" is a property of the session.
  const connectedId = session._tag === "Connected" ? session.pc.id : null

  const open = (pc: PairedPc) => {
    // Tapping the connected PC just opens its controls; anything else (re)connects.
    if (connectedId !== pc.id) act(Session.use((s) => s.start({ kind: "paired", pc })))
    router.push("/session")
  }

  const detail = (pc: PairedPc): { text: string; color: string } => {
    if (connectedId === pc.id) {
      return { text: `Connected · ${summarize(session)?.line ?? ""}`, color: colors.ok }
    }
    if (session._tag === "Connecting" && session.pcName === pc.name) {
      return { text: `${session.step}…`, color: colors.secondary }
    }
    return {
      text: visible.has(pc.id) ? "On this Wi-Fi · tap to connect" : "Tap to connect",
      color: colors.secondary
    }
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
      <Stack.Toolbar placement="left">
        <ToolbarButton glyph="settings" label="Settings" onPress={() => router.push("/settings")} />
      </Stack.Toolbar>
      <Stack.Toolbar placement="right">
        <ToolbarButton glyph="plus" label="Pair a PC" onPress={() => router.push("/pair")} />
      </Stack.Toolbar>
      <Host style={{ flex: 1 }} seedColor={BRAND}>
        <FieldGroup>
          <FieldGroup.Section title="Your PCs">
            {pcs.length === 0 ? (
              <>
                <Text textStyle={{ color: colors.secondary }}>
                  No PC yet. Start phoneset on your PC, then tap + to pair it.
                </Text>
                <Button
                  variant="outlined"
                  label="How it works"
                  onPress={() => router.push("/help")}
                />
              </>
            ) : (
              pcs.map((pc) => {
                const info = detail(pc)
                const connected = connectedId === pc.id
                return (
                  <Row key={pc.id} alignment="center" spacing={8}>
                    <Button variant="text" onPress={() => open(pc)}>
                      <Column spacing={2}>
                        <Text textStyle={{ fontSize: 17, fontWeight: "600" }}>
                          {connected ? `● ${pc.name}` : pc.name}
                        </Text>
                        <Text textStyle={{ fontSize: 13, color: info.color }}>{info.text}</Text>
                      </Column>
                    </Button>
                    <Spacer flexible />
                    {connected ? (
                      <Button
                        variant="text"
                        label="Disconnect"
                        onPress={() => act(Session.use((s) => s.stop))}
                      />
                    ) : (
                      <Button variant="text" label="Forget" onPress={() => forget(pc)} />
                    )}
                  </Row>
                )
              })
            )}
          </FieldGroup.Section>
        </FieldGroup>
      </Host>
    </>
  )
}
