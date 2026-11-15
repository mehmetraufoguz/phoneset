import type { ReactElement } from "react"

/** Android has no swipe action in the UI library: the row keeps its Forget button and is returned as it is. */
export function swipeToForget(_id: string, row: ReactElement, _onForget: () => void): ReactElement {
  return row
}

/** Whether rows can be forgotten by swiping, so that the Forget button can be left out. */
export const canSwipeToForget = false
