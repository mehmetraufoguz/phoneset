import { Button, FieldGroup, Text } from "@expo/ui"
import type { ReactElement } from "react"
import { Linking, Platform } from "react-native"
import {
  allowedAt,
  type EntitlementState,
  FREE_BUILD,
  type Offer,
  type Plan
} from "../core/entitlement"
import type { useColors, useStoreActions } from "./hooks"

type Colors = ReturnType<typeof useColors>

const PLAN_NAME: Record<Plan, string> = { monthly: "Monthly", sixMonths: "6 months" }
const TERMS_URL = "https://www.apple.com/legal/internet-services/itunes/dev/stdeula/"
const PRIVACY_URL = "https://rauf.app/phoneset/privacy/"

// What the person must be told about an auto-renewing subscription, in the app (the App Store review guidelines, 3.1.2,
// and Google Play's subscription policy). The price itself comes from the store, never from here.
const TERMS = Platform.select({
  ios:
    "Payment is charged to your Apple Account when you confirm the purchase. The subscription renews automatically " +
    "unless it is cancelled at least 24 hours before the end of the current period. Manage or cancel it in your Apple " +
    "Account settings. The microphone and the headphones are part of the app and need no subscription.",
  default:
    "Payment is charged to your Google Play account when you confirm the purchase. The subscription renews " +
    "automatically until you cancel it, which you can do at any time in Google Play. The microphone and the " +
    "headphones are part of the app and need no subscription."
})

export interface SubscriptionView {
  readonly state: EntitlementState
  readonly offers: ReadonlyArray<Offer>
  readonly actions: ReturnType<typeof useStoreActions>
}

/**
 * The subscription of the camera: what it buys, the plans with the store's prices, restoring a purchase, redeeming a code,
 * and the terms. A plain function, not a component: FieldGroup only lays out what it is given directly. Nothing in a
 * build made for testing without a store account (FREE_BUILD).
 */
export function subscriptionSection(sub: SubscriptionView, colors: Colors): ReactElement | null {
  if (FREE_BUILD) return null
  const { state, offers, actions } = sub
  const held = allowedAt(state, Date.now())
  const plan = state._tag === "Active" && state.plan !== null ? PLAN_NAME[state.plan] : null

  return (
    <FieldGroup.Section title="Camera subscription">
      {held ? (
        <>
          <Text>{plan === null ? "Subscribed" : `Subscribed · ${plan}`}</Text>
          <Button variant="text" label="Manage subscription" onPress={actions.manage} />
        </>
      ) : (
        <>
          <Text>
            Using the phone's camera as a webcam is a subscription. Audio is not: the microphone and
            the headphones stay as you bought them.
          </Text>
          {offers.length === 0 ? (
            <Text textStyle={{ color: colors.secondary }}>
              {state._tag === "Unknown"
                ? "Loading the plans…"
                : "The plans can't be shown right now."}
            </Text>
          ) : (
            offers.map((offer) => (
              <Button
                key={offer.plan}
                label={`${PLAN_NAME[offer.plan]} · ${offer.price}`}
                disabled={actions.busy}
                onPress={() => actions.buy(offer)}
              />
            ))
          )}
          <Button
            variant="text"
            label="Restore purchases"
            disabled={actions.busy}
            onPress={actions.restore}
          />
          <Button
            variant="text"
            label="Redeem a code"
            disabled={actions.busy}
            onPress={actions.redeemCode}
          />
        </>
      )}
      {actions.message === null ? null : (
        <Text textStyle={{ color: colors.danger }}>{actions.message}</Text>
      )}
      <FieldGroup.SectionFooter>
        <Text textStyle={{ fontSize: 12, color: colors.secondary }}>{TERMS}</Text>
      </FieldGroup.SectionFooter>
      <Button variant="text" label="Terms of Use" onPress={() => Linking.openURL(TERMS_URL)} />
      <Button variant="text" label="Privacy Policy" onPress={() => Linking.openURL(PRIVACY_URL)} />
    </FieldGroup.Section>
  )
}
