import type { ReactNode } from "react"
import { Pressable, StyleSheet, Text, View } from "react-native"
import type { usePalette } from "./hooks"

type Palette = ReturnType<typeof usePalette>

/** A small grey caption above a group ("PICTURE"). */
export function Label({ p, text }: { readonly p: Palette; readonly text: string }) {
  return <Text style={[styles.label, { color: p.sub }]}>{text}</Text>
}

/** A rounded group of controls. */
export function Card({ p, children }: { readonly p: Palette; readonly children: ReactNode }) {
  return (
    <View style={[styles.card, { backgroundColor: p.surface, borderColor: p.line }]}>
      {children}
    </View>
  )
}

/** One pick out of a few, side by side. The one picked is raised; a hint can sit under its label. */
export function Segments({
  p,
  items
}: {
  readonly p: Palette
  readonly items: ReadonlyArray<{
    readonly key: string
    readonly label: string
    readonly hint?: string
    readonly on: boolean
    readonly disabled?: boolean
    readonly onPress: () => void
  }>
}) {
  return (
    <View style={[styles.segments, { backgroundColor: p.raised }]}>
      {items.map((it) => (
        <Pressable
          key={it.key}
          disabled={it.disabled}
          onPress={it.onPress}
          accessibilityRole="button"
          accessibilityState={{ selected: it.on, disabled: it.disabled }}
          style={[
            styles.segment,
            it.on && { backgroundColor: p.dark ? "#3A3A40" : "#FFFFFF" },
            it.disabled && { opacity: 0.4 }
          ]}
        >
          <Text style={[styles.segLabel, { color: it.on ? p.text : p.sub }]}>{it.label}</Text>
          {it.hint === undefined ? null : (
            <Text style={[styles.segHint, { color: p.sub }]}>{it.hint}</Text>
          )}
        </Pressable>
      ))}
    </View>
  )
}

/** A row that opens another screen. */
export function NavRow({
  p,
  label,
  onPress
}: {
  readonly p: Palette
  readonly label: string
  readonly onPress: () => void
}) {
  return (
    <Pressable
      style={[styles.nav, { backgroundColor: p.surface, borderColor: p.line }]}
      onPress={onPress}
      accessibilityRole="button"
    >
      <Text style={{ color: p.text, fontSize: 16 }}>{label}</Text>
      <Text style={{ color: p.sub, fontSize: 18 }}>›</Text>
    </Pressable>
  )
}

const styles = StyleSheet.create({
  label: { fontSize: 11.5, letterSpacing: 0.7, marginTop: 2 },
  card: { borderRadius: 20, borderWidth: 1, padding: 14, gap: 10 },
  segments: { flexDirection: "row", borderRadius: 12, padding: 3, gap: 2 },
  segment: { flex: 1, alignItems: "center", borderRadius: 9, paddingVertical: 7 },
  segLabel: { fontSize: 14, fontWeight: "600" },
  segHint: { fontSize: 11 },
  nav: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    borderRadius: 16,
    borderWidth: 1,
    paddingHorizontal: 14,
    paddingVertical: 14
  }
})
