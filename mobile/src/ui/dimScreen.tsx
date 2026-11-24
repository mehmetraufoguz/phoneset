import { StatusBar } from "expo-status-bar"
import { useCallback, useEffect, useRef, useState } from "react"
import { Modal, Pressable, StyleSheet, Text, useWindowDimensions, View } from "react-native"

/** How often the status line moves to another place, so no pixel of the screen stays lit for hours. */
const MOVE_EVERY_MS = 30_000
/** How long the controls stay after a tap. */
const REVEAL_MS = 3_000
/** Room kept around the line when it moves: it is about this wide and tall, and stays off the screen's edges. */
const LINE_WIDTH = 170
const MARGIN_X = 20
const MARGIN_TOP = 90
const MARGIN_BOTTOM = 170

const pad = (n: number) => (n < 10 ? `0${n}` : `${n}`)

/** 3725 seconds as "1:02:05". */
export const clock = (seconds: number): string => {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return `${h}:${pad(m)}:${pad(seconds % 60)}`
}

/**
 * "Dim screen": a black screen over the controls while the camera keeps running. On an OLED screen black pixels give
 * no light, so it costs the panel almost nothing; the one dim line says the camera is live and moves every
 * `MOVE_EVERY_MS`. A tap shows a button to wake it for a few seconds; a long press wakes it at once. The app stays in
 * front (the camera needs that), so the phone does not lock.
 */
export function DimScreen({
  visible,
  since,
  onWake
}: {
  readonly visible: boolean
  /** When the camera went on (milliseconds), for the "live for" clock. */
  readonly since: number
  readonly onWake: () => void
}) {
  const { width, height } = useWindowDimensions()
  const [now, setNow] = useState(() => Date.now())
  const [spot, setSpot] = useState({ x: MARGIN_X, y: MARGIN_TOP })
  const [revealed, setRevealed] = useState(false)
  const hideTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  const move = useCallback(() => {
    const room = (size: number) => Math.max(0, size)
    setSpot({
      x: MARGIN_X + Math.random() * room(width - LINE_WIDTH - 2 * MARGIN_X),
      y: MARGIN_TOP + Math.random() * room(height - MARGIN_TOP - MARGIN_BOTTOM)
    })
  }, [width, height])

  useEffect(() => {
    if (!visible) return
    setNow(Date.now())
    setRevealed(false)
    move()
    const clockTimer = setInterval(() => setNow(Date.now()), 1000)
    const moveTimer = setInterval(move, MOVE_EVERY_MS)
    return () => {
      clearInterval(clockTimer)
      clearInterval(moveTimer)
      clearTimeout(hideTimer.current)
    }
  }, [visible, move])

  const reveal = () => {
    setRevealed(true)
    clearTimeout(hideTimer.current)
    hideTimer.current = setTimeout(() => setRevealed(false), REVEAL_MS)
  }

  return (
    <Modal
      visible={visible}
      animationType="fade"
      transparent={false}
      statusBarTranslucent
      supportedOrientations={["portrait"]}
      onRequestClose={onWake}
    >
      <StatusBar hidden />
      <Pressable
        style={styles.screen}
        onPress={reveal}
        onLongPress={onWake}
        accessibilityLabel="Screen is dimmed. Tap to show the wake button, hold to wake."
      >
        <View style={[styles.line, { left: spot.x, top: spot.y }]}>
          <View style={styles.dot} />
          <Text
            style={styles.lineText}
          >{`LIVE ${clock(Math.max(0, Math.round((now - since) / 1000)))}`}</Text>
        </View>
        {revealed ? (
          <View style={styles.reveal}>
            <Text style={styles.hint}>The camera is still on</Text>
            <Pressable style={styles.wake} onPress={onWake} accessibilityRole="button">
              <Text style={styles.wakeText}>Wake screen</Text>
            </Pressable>
            <Text style={styles.hint}>Or hold anywhere</Text>
          </View>
        ) : null}
      </Pressable>
    </Modal>
  )
}

// Always black and grey, whatever the light or dark setting: this is the screen that must give off the least light.
const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: "#000000" },
  line: { position: "absolute", flexDirection: "row", alignItems: "center", gap: 6 },
  dot: { width: 7, height: 7, borderRadius: 4, backgroundColor: "#1F5F2B" },
  lineText: { color: "#3C3C42", fontSize: 13, fontVariant: ["tabular-nums"] },
  reveal: { position: "absolute", left: 0, right: 0, bottom: 56, alignItems: "center", gap: 12 },
  hint: { color: "#6A6A72", fontSize: 13 },
  wake: {
    backgroundColor: "#1D1D20",
    borderRadius: 14,
    paddingVertical: 13,
    paddingHorizontal: 36
  },
  wakeText: { color: "#A8A8B0", fontSize: 16, fontWeight: "600" }
})
