import type { ConfigContext, ExpoConfig } from "expo/config"

// app.json holds the app. The iOS Live Activity (the lock screen / Dynamic Island view of a
// connection) is added here, and only on request, because it needs a widget extension that
// shares an App Group with the app, and an App Group has to be registered once in the Apple
// Developer account first (Certificates, Identifiers & Profiles > Identifiers > App Groups,
// group.com.rapp.phoneset). Without it, signing fails.
//
//   PHONESET_LIVE_ACTIVITY=1 pnpm mobile:ios
const liveActivity = process.env.PHONESET_LIVE_ACTIVITY === "1"

export default ({ config }: ConfigContext): ExpoConfig => ({
  ...(config as ExpoConfig),
  // Read by the app at run time: without the extension the Live Activity code must not even load.
  extra: { ...config.extra, liveActivity },
  ios: {
    ...config.ios,
    infoPlist: {
      ...config.ios?.infoPlist,
      ...(liveActivity ? { NSSupportsLiveActivities: true } : {})
    }
  },
  plugins: [
    ...(config.plugins ?? []),
    ...(liveActivity
      ? ([
          [
            "expo-widgets",
            {
              bundleIdentifier: "com.rapp.phoneset.widgets",
              groupIdentifier: "group.com.rapp.phoneset"
            }
          ]
        ] satisfies ExpoConfig["plugins"])
      : [])
  ]
})
