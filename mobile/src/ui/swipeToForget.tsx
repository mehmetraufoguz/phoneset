import { Box, HorizontalPager, Text } from "@expo/ui/jetpack-compose"
import { background, fillMaxWidth, height, padding } from "@expo/ui/jetpack-compose/modifiers"
import type { ReactElement } from "react"

/** The height of a PC row in dp (the details button: two lines of text and its padding), so that the red page is as tall. */
const ROW_HEIGHT = 56
const DESTRUCTIVE = "#B3261E"

/**
 * Wraps a list row so that swiping it to the left forgets it, as on iOS. Expo UI has no swipe-to-dismiss for Android
 * (SwiftUI has `SwipeActions`, Compose has nothing of the kind), so the row is the first page of a two-page pager
 * and the second page is the red "Forget" one: dragging past the middle settles on it and the PC is forgotten at
 * once (the Undo row on the home screen puts it back). Letting go before the middle snaps back. `id` is the list key.
 * (A plain function, not a component: FieldGroup only lays out what it is given directly.)
 */
export function swipeToForget(id: string, row: ReactElement, onForget: () => void): ReactElement {
  return (
    <HorizontalPager
      key={id}
      modifiers={[fillMaxWidth()]}
      onSettledPageChange={(page) => {
        if (page === 1) onForget()
      }}
    >
      {row}
      <Box
        contentAlignment="centerEnd"
        modifiers={[
          fillMaxWidth(),
          height(ROW_HEIGHT),
          background(DESTRUCTIVE),
          padding(0, 0, 24, 0)
        ]}
      >
        <Text color="#FFFFFF" style={{ fontSize: 16, fontWeight: "600" }}>
          Forget
        </Text>
      </Box>
    </HorizontalPager>
  )
}

/** Whether rows can be forgotten by swiping, so that the Forget button can be left out. */
export const canSwipeToForget = true
