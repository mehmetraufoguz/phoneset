import { Button, SwipeActions } from "@expo/ui/swift-ui"
import type { ReactElement } from "react"

/**
 * Wraps a list row so that swiping it from the right edge forgets it: a full swipe does it at once, a short one
 * shows the red button; `id` is the list key. (A plain function, not a component: FieldGroup only lays out what it is given directly.)
 */
export function swipeToForget(id: string, row: ReactElement, onForget: () => void): ReactElement {
  return (
    <SwipeActions key={id}>
      {row}
      <SwipeActions.Actions edge="trailing" allowsFullSwipe>
        {/* biome-ignore lint/a11y/useValidAriaRole: the role of a SwiftUI button, not an ARIA role */}
        <Button role="destructive" label="Forget" systemImage="trash" onPress={onForget} />
      </SwipeActions.Actions>
    </SwipeActions>
  )
}

export const canSwipeToForget = true
