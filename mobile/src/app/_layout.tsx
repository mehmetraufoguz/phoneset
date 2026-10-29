import { Stack } from "expo-router"
import { StatusBar } from "expo-status-bar"

export default function RootLayout() {
  return (
    <>
      <StatusBar style="auto" />
      <Stack>
        <Stack.Screen name="index" options={{ title: "phoneset" }} />
        <Stack.Screen name="pair" options={{ title: "Pair a PC" }} />
        <Stack.Screen name="settings" options={{ title: "Settings" }} />
        <Stack.Screen name="help" options={{ title: "How it works" }} />
        <Stack.Screen
          name="scan"
          options={{
            title: "Scan QR code",
            presentation: "modal",
            headerBackButtonDisplayMode: "minimal"
          }}
        />
        <Stack.Screen name="session" options={{ title: "Connected PC", gestureEnabled: false }} />
      </Stack>
    </>
  )
}
