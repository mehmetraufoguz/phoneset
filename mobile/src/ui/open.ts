import { Alert, Linking } from "react-native"

/** Opens a web page in the system browser. */
export const openUrl = (url: string): void => {
  Linking.openURL(url).catch(() => Alert.alert("Couldn't open the link", url))
}
