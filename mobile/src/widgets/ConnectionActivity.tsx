import { HStack, Spacer, Text, VStack } from "@expo/ui/swift-ui"
import { font, foregroundStyle } from "@expo/ui/swift-ui/modifiers"
import { createLiveActivity } from "expo-widgets"

export type ConnectionProps = {
  pc: string
  line: string
  short: string
}

// The Live Activity on the lock screen and in the Dynamic Island while a PC is connected.
// Widget code runs in a separate process: it must be a pure function with no hooks and no
// values from module scope, which is what the "widget" directive promises.
const ConnectionActivity = (props: ConnectionProps) => {
  "widget"
  return {
    banner: (
      <HStack spacing={12}>
        <VStack spacing={2}>
          <Text modifiers={[font({ size: 17, weight: "semibold" })]}>{props.pc}</Text>
          <Text modifiers={[font({ size: 13 }), foregroundStyle("secondary")]}>{props.line}</Text>
        </VStack>
        <Spacer />
        <Text modifiers={[font({ size: 13, weight: "semibold" }), foregroundStyle("secondary")]}>
          phoneset
        </Text>
      </HStack>
    ),
    compactLeading: <Text modifiers={[font({ size: 13, weight: "semibold" })]}>phoneset</Text>,
    compactTrailing: (
      <Text modifiers={[font({ size: 13, weight: "semibold" })]}>{props.short}</Text>
    ),
    minimal: <Text modifiers={[font({ size: 13, weight: "semibold" })]}>{props.short}</Text>
  }
}

export default createLiveActivity("ConnectionActivity", ConnectionActivity)
