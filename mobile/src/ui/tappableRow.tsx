import { Button, Row, Spacer } from "@expo/ui"
import type { ReactElement } from "react"

/** A row that opens something when it is tapped: a text button whose label runs to the edge (see wholeRow.ios.tsx). */
export function tappableRow(content: ReactElement, onPress: () => void): ReactElement {
  return (
    <Button variant="text" onPress={onPress}>
      <Row alignment="center" spacing={8}>
        {content}
        <Spacer flexible />
      </Row>
    </Button>
  )
}
