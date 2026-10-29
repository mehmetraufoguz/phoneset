import { Stack } from "expo-router"
import { Platform } from "react-native"

// SF Symbols only exist on iOS; the Android header draws images, so each glyph has both.
// The images are made by scripts/icons.py.
const glyphs = {
  plus: { sf: "plus", src: require("../../assets/toolbar/plus.png") },
  settings: { sf: "gearshape", src: require("../../assets/toolbar/settings.png") }
} as const

/**
 * A header button that shows on both platforms. Call it, don't render it as a component:
 * `Stack.Toolbar` reads its children by type, so a wrapper component is skipped on iOS and
 * the button never shows.
 */
export function toolbarButton(glyph: keyof typeof glyphs, label: string, onPress: () => void) {
  const { sf, src } = glyphs[glyph]
  return Platform.OS === "ios" ? (
    <Stack.Toolbar.Button icon={sf} accessibilityLabel={label} onPress={onPress} />
  ) : (
    <Stack.Toolbar.Button accessibilityLabel={label} onPress={onPress}>
      <Stack.Toolbar.Icon src={src} />
    </Stack.Toolbar.Button>
  )
}
