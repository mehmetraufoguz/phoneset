import { Linking, ScrollView, Text, View } from "react-native"
import { allowedAt, FREE_BUILD } from "../core/entitlement"
import { useEntitlement, useOffers, usePalette, useStoreActions } from "../ui/hooks"
import { Card, NavRow, PrimaryButton, QuietButton } from "../ui/parts"
import { PLAN_NAME, PRIVACY_URL, TERMS, TERMS_URL } from "../ui/plans"

/**
 * The camera subscription: the state, the plans with the store's prices, restore and redeem, and the terms the stores
 * require next to a purchase (the text under the buttons stays; it is a rule, not an explanation). Nothing is sold in a
 * build made for testing without a store account (FREE_BUILD).
 */
export default function Subscription() {
  const p = usePalette()
  const state = useEntitlement()
  const offers = useOffers()
  const actions = useStoreActions()
  const held = allowedAt(state, Date.now())
  const plan = state._tag === "Active" && state.plan !== null ? PLAN_NAME[state.plan] : null

  if (FREE_BUILD) {
    return (
      <View style={{ flex: 1, backgroundColor: p.bg, padding: 16 }}>
        <Text style={{ color: p.sub, fontSize: 15 }}>The camera is free in this build.</Text>
      </View>
    )
  }

  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: p.bg }}
      contentInsetAdjustmentBehavior="automatic"
      contentContainerStyle={{ padding: 16, gap: 12 }}
    >
      <Card p={p}>
        <Text style={{ color: p.text, fontSize: 22, fontWeight: "700" }}>
          {held ? (plan === null ? "Subscribed" : `Subscribed · ${plan}`) : "Camera subscription"}
        </Text>
        <Text style={{ color: p.sub, fontSize: 14 }}>
          {held
            ? "Your phone can be a webcam."
            : "Use the phone as a webcam. Microphone and headphones stay free."}
        </Text>
      </Card>

      {held ? (
        <PrimaryButton label="Manage subscription" onPress={actions.manage} />
      ) : offers.length === 0 ? (
        <Text style={{ color: p.sub, textAlign: "center", fontSize: 14 }}>
          {state._tag === "Unknown" ? "Loading the plans…" : "The plans can't be shown right now."}
        </Text>
      ) : (
        offers.map((offer) => (
          <PrimaryButton
            key={offer.plan}
            label={`${PLAN_NAME[offer.plan]} · ${offer.price}`}
            disabled={actions.busy}
            onPress={() => actions.buy(offer)}
          />
        ))
      )}

      {held ? null : (
        <>
          <QuietButton
            p={p}
            label="Restore purchases"
            disabled={actions.busy}
            onPress={actions.restore}
          />
          <QuietButton
            p={p}
            label="Redeem a code"
            disabled={actions.busy}
            onPress={actions.redeemCode}
          />
        </>
      )}
      {actions.message === null ? null : (
        <Text style={{ color: p.danger, fontSize: 14, textAlign: "center" }}>
          {actions.message}
        </Text>
      )}

      <Text style={{ color: p.sub, fontSize: 11.5 }}>{TERMS}</Text>
      <NavRow p={p} label="Terms of Use" onPress={() => Linking.openURL(TERMS_URL)} />
      <NavRow p={p} label="Privacy Policy" onPress={() => Linking.openURL(PRIVACY_URL)} />
    </ScrollView>
  )
}
