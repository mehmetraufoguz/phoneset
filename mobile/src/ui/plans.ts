import { Platform } from "react-native"
import type { Plan } from "../core/entitlement"

export const PLAN_NAME: Record<Plan, string> = { monthly: "Monthly", sixMonths: "6 months" }
export const TERMS_URL = "https://www.apple.com/legal/internet-services/itunes/dev/stdeula/"
export const PRIVACY_URL = "https://rauf.app/phoneset/privacy/"

// What the person must be told about an auto-renewing subscription, in the app (the App Store review guidelines, 3.1.2,
// and Google Play's subscription policy). The price itself comes from the store, never from here.
export const TERMS = Platform.select({
  ios:
    "Payment is charged to your Apple Account when you confirm the purchase. The subscription renews automatically " +
    "unless it is cancelled at least 24 hours before the end of the current period. Manage or cancel it in your Apple " +
    "Account settings. The microphone and the headphones are part of the app and need no subscription.",
  default:
    "Payment is charged to your Google Play account when you confirm the purchase. The subscription renews " +
    "automatically until you cancel it, which you can do at any time in Google Play. The microphone and the " +
    "headphones are part of the app and need no subscription."
})
