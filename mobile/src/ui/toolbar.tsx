import { Stack } from "expo-router"
import { Platform } from "react-native"

// SF Symbols only exist on iOS; the Android header draws images, so each glyph has both.
// The images are made by scripts/icons.py.
const glyphs = {
  plus: { sf: "plus", src: require("../../assets/toolbar/plus.png") },
  settings: { sf: "gearshape", src: require("../../assets/toolbar/settings.png") }
} as const

/** A header button that shows on both platforms. Use inside `<Stack.Toolbar placement=…>`. */
export function ToolbarButton({
  glyph,
  label,
  onPress
}: {
  glyph: keyof typeof glyphs
  label: string
  onPress: () => void
}) {
  const { sf, src } = glyphs[glyph]
  return (
    <Stack.Toolbar.Button accessibilityLabel={label} onPress={onPress}>
      <Stack.Toolbar.Icon {...(Platform.OS === "ios" ? { sf } : { src })} />
    </Stack.Toolbar.Button>
  )
}
