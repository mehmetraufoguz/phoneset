import { Effect, Layer, Queue, Stream } from "effect"
import * as IAP from "expo-iap"
import { Linking, Platform } from "react-native"
import {
  ANDROID_BASE_PLANS,
  ANDROID_SKU,
  type Held,
  IOS_SKUS,
  type Offer,
  type Plan,
  Store
} from "../core/entitlement"
import { StoreError } from "../core/errors"

// The App Store and Google Play, through expo-iap (StoreKit 2 and Play Billing). Nothing is sent to a server of ours.

const PLANS: ReadonlyArray<Plan> = ["monthly", "sixMonths"]
const isIos = Platform.OS === "ios"
const SKUS = isIos ? PLANS.map((p) => IOS_SKUS[p]) : [ANDROID_SKU]
const PACKAGE = "com.rapp.phoneset"

const fail = (e: unknown, what: string) =>
  new StoreError({
    message: IAP.isUserCancelledError(e)
      ? "Cancelled."
      : `${what}: ${e instanceof Error ? e.message : String(e)}`,
    cancelled: IAP.isUserCancelledError(e)
  })

const store = <A>(what: string, run: () => Promise<A>) =>
  Effect.tryPromise({ try: run, catch: (e) => fail(e, what) })

// One connection to the store, made when it is first needed and kept.
let connection: Promise<unknown> | null = null
const connected = Effect.tryPromise({
  try: () => {
    connection ??= IAP.initConnection().catch((e) => {
      connection = null
      throw e
    })
    return connection
  },
  catch: (e) => fail(e, "The store isn't available")
})

const planOfIos = (sku: string | null | undefined): Plan | null =>
  PLANS.find((p) => IOS_SKUS[p] === sku) ?? null
const planOfAndroid = (basePlan: string | null | undefined): Plan | null =>
  PLANS.find((p) => ANDROID_BASE_PLANS[p] === basePlan) ?? null

const offers = connected.pipe(
  Effect.andThen(
    store("The plans couldn't be loaded", async () => {
      const products = (await IAP.fetchProducts({ skus: SKUS, type: "subs" })) ?? []
      const out: Offer[] = []
      for (const product of products) {
        if (product.platform === "ios") {
          const plan = planOfIos(product.id)
          if (plan !== null)
            out.push({ plan, sku: product.id, price: product.displayPrice, offerToken: null })
        } else {
          // One subscription with a base plan per plan; the base plan's own offer, not a promotion's.
          for (const plan of PLANS) {
            const offer =
              (product.subscriptionOffers ?? []).find(
                (o) => planOfAndroid(o.basePlanIdAndroid) === plan && !o.id
              ) ??
              (product.subscriptionOffers ?? []).find(
                (o) => planOfAndroid(o.basePlanIdAndroid) === plan
              )
            if (offer?.offerTokenAndroid)
              out.push({
                plan,
                sku: product.id,
                price: offer.displayPrice,
                offerToken: offer.offerTokenAndroid
              })
          }
        }
      }
      return out
    })
  )
)

const held = connected.pipe(
  Effect.andThen(
    store("The subscription couldn't be checked", async (): Promise<Held | null> => {
      const subs = await IAP.getActiveSubscriptions(SKUS)
      const active = subs.find((s) => s.isActive)
      if (!active) return null
      return {
        plan: isIos
          ? planOfIos(active.productId)
          : planOfAndroid(active.basePlanIdAndroid ?? active.currentPlanId),
        // StoreKit gives the expiry (ms); Play Billing does not.
        expiresAt: isIos ? (active.expirationDateIOS ?? null) : null
      }
    })
  )
)

const buy = (offer: Offer) =>
  connected.pipe(
    Effect.andThen(
      store("The purchase didn't go through", async () => {
        await IAP.requestPurchase({
          type: "subs",
          request: {
            apple: { sku: offer.sku },
            google: {
              skus: [offer.sku],
              subscriptionOffers: offer.offerToken
                ? [{ sku: offer.sku, offerToken: offer.offerToken }]
                : []
            }
          }
        })
      })
    )
  )

const restore = connected.pipe(
  Effect.andThen(store("The purchases couldn't be restored", () => IAP.restorePurchases())),
  Effect.asVoid
)

// iOS: Apple's sheet, in the app. Android has no such sheet: Google's redeem page opens in the browser, and the
// purchase is picked up when the app comes back.
const redeemCode = Effect.suspend(() =>
  isIos
    ? connected.pipe(
        Effect.andThen(store("The code sheet couldn't be opened", () => IAP.openRedeemOfferCode())),
        Effect.asVoid
      )
    : store("The redeem page couldn't be opened", () =>
        Linking.openURL("https://play.google.com/redeem")
      )
)

const manage = Effect.suspend(() =>
  isIos
    ? store("The subscriptions page couldn't be opened", () =>
        Linking.openURL("https://apps.apple.com/account/subscriptions")
      )
    : store("The subscriptions page couldn't be opened", () =>
        IAP.deepLinkToSubscriptions({ skuAndroid: ANDROID_SKU, packageNameAndroid: PACKAGE })
      )
)

// Every purchase the store reports (bought here, renewed, redeemed, restored) must be finished: StoreKit keeps
// re-delivering an unfinished transaction, and Google Play refunds a purchase that is not acknowledged in 3 days.
const updates = Stream.callback<void>((queue) =>
  Effect.gen(function* () {
    yield* Effect.acquireRelease(
      connected.pipe(
        Effect.andThen(
          Effect.sync(() =>
            IAP.purchaseUpdatedListener((purchase) => {
              IAP.finishTransaction({ purchase, isConsumable: false })
                .catch(() => undefined)
                .finally(() => Queue.offerUnsafe(queue, undefined))
            })
          )
        ),
        Effect.orElseSucceed(() => ({ remove: () => undefined }))
      ),
      (subscription) => Effect.sync(() => subscription.remove())
    )
  })
)

export const StoreLive = Layer.succeed(Store, {
  offers,
  held,
  buy,
  restore,
  redeemCode,
  manage,
  updates
})
