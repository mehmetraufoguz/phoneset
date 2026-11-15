import { Button, Column, FieldGroup, Host, Row, Spacer, Text } from "@expo/ui"
import { Effect } from "effect"
import { Stack, useFocusEffect, useRouter } from "expo-router"
import { useCallback, useEffect, useState } from "react"
import { summarize } from "../core/presence"
import { Session } from "../core/session"
import { type PairedPc, PairedPcs } from "../core/storage"
import { act, BRAND, useColors, useDiscovered, usePairedPcs, useSessionState } from "../ui/hooks"
import { canSwipeToForget, swipeToForget } from "../ui/swipeToForget"
import { toolbarButton } from "../ui/toolbar"

/** How long the Undo row stays after a PC is forgotten. */
const UNDO_FOR_MS = 8000

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

  const details = (pc: PairedPc) => {
    const info = detail(pc)
    return (
      <Button variant="text" onPress={() => open(pc)}>
        <Column spacing={2}>
          <Text textStyle={{ fontSize: 17, fontWeight: "600" }}>
            {connectedId === pc.id ? `● ${pc.name}` : pc.name}
          </Text>
          <Text textStyle={{ fontSize: 13, color: info.color }}>{info.text}</Text>
        </Column>
      </Button>
    )
  }

  // Forgetting is one step, with no question: the PC is kept in memory for a few seconds and an Undo row
  // puts it back (a dialog for a thing that can be undone only gets in the way).
  const [forgotten, setForgotten] = useState<PairedPc | null>(null)
  useEffect(() => {
    if (forgotten === null) return
    const timer = setTimeout(() => setForgotten(null), UNDO_FOR_MS)
    return () => clearTimeout(timer)
  }, [forgotten])

  // The list is read again once the change has been written, not before: `act` only starts the work.
  const refreshed = Effect.sync(() => setRefresh((n) => n + 1))

  const forget = (pc: PairedPc) => {
    setForgotten(pc)
    act(PairedPcs.use((p) => p.remove(pc.id)).pipe(Effect.andThen(refreshed)))
  }

  const undo = () => {
    if (forgotten === null) return
    const pc = forgotten
    setForgotten(null)
    act(PairedPcs.use((p) => p.save(pc)).pipe(Effect.andThen(refreshed)))
  }

  return (
    <>
      <Stack.Toolbar placement="left">
        {toolbarButton("settings", "Settings", () => router.push("/settings"))}
      </Stack.Toolbar>
      <Stack.Toolbar placement="right">
        {toolbarButton("plus", "Pair a PC", () => router.push("/pair"))}
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
              // A connected PC's status line is long ("Connected · Microphone on · Headphones on").
              // Beside it in a Row it takes the width first and squeezes Disconnect into
              // "Disc/onne/ct", so Disconnect goes under it.
              pcs.map((pc) =>
                connectedId === pc.id ? (
                  <Column key={pc.id} spacing={2}>
                    {details(pc)}
                    <Button
                      variant="text"
                      label="Disconnect"
                      onPress={() => act(Session.use((s) => s.stop))}
                    />
                  </Column>
                ) : (
                  // Swipe the row to forget it where the platform can; otherwise it has a Forget button.
                  swipeToForget(
                    pc.id,
                    <Row key={pc.id} alignment="center" spacing={8}>
                      {details(pc)}
                      <Spacer flexible />
                      {canSwipeToForget ? null : (
                        <Button variant="text" label="Forget" onPress={() => forget(pc)} />
                      )}
                    </Row>,
                    () => forget(pc)
                  )
                )
              )
            )}
            {forgotten === null ? null : (
              <Row alignment="center" spacing={8}>
                <Text textStyle={{ color: colors.secondary }}>{`Forgot ${forgotten.name}`}</Text>
                <Spacer flexible />
                <Button variant="text" label="Undo" onPress={undo} />
              </Row>
            )}
          </FieldGroup.Section>
        </FieldGroup>
      </Host>
    </>
  )
}
