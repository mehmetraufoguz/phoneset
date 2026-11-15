import { HStack, Spacer } from "@expo/ui/swift-ui"
import {
  accessibilityAddTraits,
  contentShape,
  onTapGesture,
  shapes
} from "@expo/ui/swift-ui/modifiers"
import type { ReactElement } from "react"

/**
 * A row that opens something when it is tapped, anywhere on it. The content, then the empty space to the edge,
 * all answer a tap; a finger that is dragged across the row and let go is not a tap (a SwiftUI button would
 * count it, a tap gesture does not). The rectangular hit shape is on the row itself, since the empty space
 * draws nothing and would not answer without it.
 */
export function tappableRow(content: ReactElement, onPress: () => void): ReactElement {
  return (
    <HStack
      spacing={8}
      modifiers={[
        contentShape(shapes.rectangle()),
        onTapGesture(onPress),
        accessibilityAddTraits(["isButton"])
      ]}
    >
      {content}
      <Spacer />
    </HStack>
  )
}
