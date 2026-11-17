import { Context, Effect, Layer, Schema, Stream, SubscriptionRef } from "effect"
import type { StoreError } from "./errors"
import { KeyValue } from "./storage"

// Video (the phone's camera as a PC webcam) is a subscription; audio is not. This file is everything the app knows about
// it that is not a store's API: what is on offer, what is held, how long a held subscription is trusted without asking
// the store again, and the service the rest of the app asks "may the camera be used?". The store itself (StoreKit 2,
// Google Play Billing, through expo-iap) is the `Store` service, provided by the platform (platform/store.ts).
//
// No account and no server of ours: the store is the only authority, asked on the phone, and a cache in the Keychain /
// Keystore keeps video working while the phone is offline.

/** The plans. The ids are what the stores call them (see docs/store/private/camera-pricing.md for the prices). */
export type Plan = "monthly" | "sixMonths"

/** Apple: one product per plan. Google Play: one subscription, `video`, with a base plan per plan. */
export const IOS_SKUS: Record<Plan, string> = {
  monthly: "com.rapp.phoneset.video.monthly",
  sixMonths: "com.rapp.phoneset.video.sixmonths"
}
export const ANDROID_SKU = "video"
export const ANDROID_BASE_PLANS: Record<Plan, string> = {
  monthly: "monthly",
  sixMonths: "sixmonths"
}

/** A plan the person can buy, with the price the store shows (never a price of ours). */
export interface Offer {
  readonly plan: Plan
  readonly sku: string
  /** Formatted by the store, in the person's currency, for example "$2.99". */
  readonly price: string
  /** Google Play: which offer of the subscription this is. */
  readonly offerToken: string | null
}

/** What the store says is held right now. `expiresAt` is null when the store does not say (Google Play). */
export interface Held {
  readonly plan: Plan | null
  readonly expiresAt: number | null
}

export type EntitlementState =
  /** Not asked yet (the app has just started and has no cached answer). */
  | { readonly _tag: "Unknown" }
  /** Video is allowed. `verifiedAt` is when the store last said so. */
  | {
      readonly _tag: "Active"
      readonly plan: Plan | null
      readonly expiresAt: number | null
      readonly verifiedAt: number
    }
  /** The store says there is no subscription (never bought, or it lapsed). */
  | { readonly _tag: "Inactive" }

/**
 * How long a subscription that the store said was active is still trusted when the store cannot be asked (no network,
 * the store app is busy): until it expires, plus this. After a renewal the store has the new expiry; a phone that was
 * offline over the renewal day must not lose video for it. For a store that does not give an expiry, the time since the
 * store last said it was active counts instead, from `verifiedAt`.
 */
export const OFFLINE_GRACE_MS = 3 * 24 * 3600 * 1000
/** With no expiry known, how long the last "active" is trusted. A subscription renews every period (a month at least). */
export const UNKNOWN_EXPIRY_TRUST_MS = 33 * 24 * 3600 * 1000

/**
 * A build for testing without a store account sets EXPO_PUBLIC_PHONESET_VIDEO_FREE=1 when it is bundled: video is allowed
 * and the subscription section stays out of the way. A store build never sets it.
 */
export const FREE_BUILD = process.env.EXPO_PUBLIC_PHONESET_VIDEO_FREE === "1"

/** Is video allowed by what was last known, at this time? Pure: the whole policy is here. */
export const allowedAt = (state: EntitlementState, now: number): boolean => {
  if (state._tag !== "Active") return false
  const end =
    state.expiresAt !== null
      ? state.expiresAt + OFFLINE_GRACE_MS
      : state.verifiedAt + UNKNOWN_EXPIRY_TRUST_MS
  return now <= end
}

/** The store, as the app needs it. Provided by the platform. */
export class Store extends Context.Service<
  Store,
  {
    /** The plans on sale, with their prices. */
    readonly offers: Effect.Effect<ReadonlyArray<Offer>, StoreError>
    /** The subscription the store says is active, or null. */
    readonly held: Effect.Effect<Held | null, StoreError>
    readonly buy: (offer: Offer) => Effect.Effect<void, StoreError>
    /** Asks the store to bring back the purchases of this Apple / Google account. */
    readonly restore: Effect.Effect<void, StoreError>
    /** Offer codes: Apple's sheet in the app, Google's redeem page in the browser. */
    readonly redeemCode: Effect.Effect<void, StoreError>
    /** The store's own page for cancelling or changing the subscription. */
    readonly manage: Effect.Effect<void, StoreError>
    /** Emits when the store reports a purchase: bought here, renewed, redeemed or restored elsewhere. */
    readonly updates: Stream.Stream<void>
  }
>()("phoneset/Store") {}

/** What is saved between launches. */
const Cached = Schema.Struct({
  plan: Schema.NullOr(Schema.Literals(["monthly", "sixMonths"])),
  expiresAt: Schema.NullOr(Schema.Number),
  verifiedAt: Schema.Number
})
const decodeCached = Schema.decodeUnknownEffect(Schema.fromJsonString(Cached))
const encodeCached = Schema.encodeEffect(Schema.fromJsonString(Cached))
const CACHE_KEY = "phoneset.entitlement"

/**
 * May the camera be used? Asked when video is started; refreshed at launch, when the app comes back to the front and when
 * the store reports a purchase. Only a *no* from the store takes the right away; a store that cannot be reached leaves the
 * cached answer standing (until `allowedAt` says it is too old).
 */
export class Entitlement extends Context.Service<
  Entitlement,
  {
    readonly state: SubscriptionRef.SubscriptionRef<EntitlementState>
    /** Asks the store and updates the state and the cache. Never fails: a store that cannot be asked changes nothing. */
    readonly refresh: Effect.Effect<void>
    /** May video start now? Looks at the state with the clock; asks the store when the answer is "unknown". */
    readonly allowed: Effect.Effect<boolean>
    readonly offers: Effect.Effect<ReadonlyArray<Offer>, StoreError>
    readonly buy: (offer: Offer) => Effect.Effect<void, StoreError>
    readonly restore: Effect.Effect<void, StoreError>
    readonly redeemCode: Effect.Effect<void, StoreError>
    readonly manage: Effect.Effect<void, StoreError>
  }
>()("phoneset/Entitlement") {
  static readonly layer = Layer.effect(
    Entitlement,
    Effect.gen(function* () {
      const store = yield* Store
      const kv = yield* KeyValue
      const state = yield* SubscriptionRef.make<EntitlementState>({ _tag: "Unknown" })

      // What was saved last time: video works at once, without waiting for the store.
      yield* kv.get(CACHE_KEY).pipe(
        Effect.flatMap((raw) =>
          raw === undefined
            ? Effect.void
            : decodeCached(raw).pipe(
                Effect.flatMap((c) => SubscriptionRef.set(state, { _tag: "Active", ...c }))
              )
        ),
        Effect.ignore
      )

      const save = (held: Held, verifiedAt: number) =>
        encodeCached({ plan: held.plan, expiresAt: held.expiresAt, verifiedAt }).pipe(
          Effect.flatMap((json) => kv.set(CACHE_KEY, json)),
          Effect.ignore
        )

      const refresh = store.held.pipe(
        Effect.flatMap((held) => {
          if (held === null) {
            // The store answered: no subscription. The cache goes too.
            return SubscriptionRef.set(state, { _tag: "Inactive" }).pipe(
              Effect.andThen(kv.remove(CACHE_KEY).pipe(Effect.ignore))
            )
          }
          const verifiedAt = Date.now()
          return SubscriptionRef.set(state, { _tag: "Active", ...held, verifiedAt }).pipe(
            Effect.andThen(save(held, verifiedAt))
          )
        }),
        // The store could not be asked: keep what is known.
        Effect.ignore
      )

      const allowed = Effect.gen(function* () {
        if (FREE_BUILD) return true
        let s = yield* SubscriptionRef.get(state)
        if (s._tag === "Unknown") {
          yield* refresh
          s = yield* SubscriptionRef.get(state)
        }
        return allowedAt(s, Date.now())
      })

      // A purchase reported by the store (bought, renewed, redeemed) changes what is held.
      yield* store.updates.pipe(
        Stream.runForEach(() => refresh),
        Effect.forkScoped
      )

      const then = <A>(effect: Effect.Effect<A, StoreError>) =>
        effect.pipe(Effect.tap(() => refresh))

      return Entitlement.of({
        state,
        refresh,
        allowed,
        offers: store.offers,
        buy: (offer) => then(store.buy(offer)),
        restore: then(store.restore),
        redeemCode: then(store.redeemCode),
        manage: store.manage
      })
    })
  )
}
