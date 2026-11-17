import { Effect, Fiber, Stream, SubscriptionRef } from "effect"
import { useEffect, useState } from "react"
import { AppState, useColorScheme } from "react-native"
import { Discovery } from "../core/discovery"
import { Entitlement, type EntitlementState, type Offer } from "../core/entitlement"
import type { StoreError } from "../core/errors"
import { Session, type SessionState } from "../core/session"
import { PairedPcs } from "../core/storage"
import { type AppServices, runtime } from "../runtime"

/** Brand tint, handed to Expo UI's Host so native controls pick it up. */
export const BRAND = "#3B6BFF"

/** Runs an effect from an event handler. Failures are logged, never thrown into React. */
export const act = (effect: Effect.Effect<unknown, unknown, AppServices>): void => {
  runtime.runFork(effect.pipe(Effect.catchCause((cause) => Effect.logError(cause))))
}

/** Subscribes to a stream for as long as the component is mounted. */
export function useStreamValue<A>(stream: Stream.Stream<A, unknown, AppServices>, initial: A): A {
  const [value, setValue] = useState(initial)
  useEffect(() => {
    const fiber = runtime.runFork(
      stream.pipe(
        Stream.runForEach((a) => Effect.sync(() => setValue(a))),
        Effect.ignore
      )
    )
    return () => {
      runtime.runFork(Fiber.interrupt(fiber))
    }
  }, [stream])
  return value
}

/** Runs an effect on mount and again whenever `refresh` changes. */
export function useEffectValue<A>(
  effect: Effect.Effect<A, unknown, AppServices>,
  initial: A,
  refresh = 0
): A {
  const [value, setValue] = useState(initial)
  // biome-ignore lint/correctness/useExhaustiveDependencies: `refresh` is the trigger, not a value read inside
  useEffect(() => {
    const fiber = runtime.runFork(
      effect.pipe(
        Effect.flatMap((a) => Effect.sync(() => setValue(a))),
        Effect.ignore
      )
    )
    return () => {
      runtime.runFork(Fiber.interrupt(fiber))
    }
  }, [effect, refresh])
  return value
}

const sessionStates: Stream.Stream<SessionState, never, AppServices> = Stream.unwrap(
  Session.use((s) => Effect.succeed(SubscriptionRef.changes(s.state)))
)
const discoveries = Stream.unwrap(Discovery.use((d) => Effect.succeed(d.scan)))
const pairedPcs = PairedPcs.use((p) => p.list)

export const useSessionState = (): SessionState =>
  useStreamValue(sessionStates, { _tag: "Idle" } as SessionState)

export const useDiscovered = () => useStreamValue(discoveries, [])

export const usePairedPcs = (refresh: number) => useEffectValue(pairedPcs, [], refresh)

/** Colours for the few places Expo UI text needs an explicit one. */
export function useColors() {
  const dark = useColorScheme() === "dark"
  return {
    secondary: dark ? "#98989F" : "#6C6C70",
    danger: dark ? "#FF6961" : "#D70015",
    ok: dark ? "#32D74B" : "#248A3D"
  }
}

const entitlementStates: Stream.Stream<EntitlementState, never, AppServices> = Stream.unwrap(
  Entitlement.use((e) => Effect.succeed(SubscriptionRef.changes(e.state)))
)

/**
 * Whether video is allowed (a subscription is held), kept up to date: asked once when the screen opens and again each
 * time the app comes back to the front (a purchase can be made, renewed or cancelled outside it).
 */
export function useEntitlement(): EntitlementState {
  const state = useStreamValue(entitlementStates, { _tag: "Unknown" } as EntitlementState)
  useEffect(() => {
    const refresh = () => {
      runtime.runFork(Entitlement.use((e) => e.refresh))
    }
    refresh()
    const sub = AppState.addEventListener("change", (s) => {
      if (s === "active") refresh()
    })
    return () => sub.remove()
  }, [])
  return state
}

const offersEffect = Entitlement.use((e) => e.offers).pipe(
  Effect.orElseSucceed(() => [] as ReadonlyArray<Offer>)
)

/** The plans on sale with their prices; empty until the store has answered (or when it cannot be reached). */
export const useOffers = (): ReadonlyArray<Offer> =>
  useEffectValue(offersEffect, [] as ReadonlyArray<Offer>)

/** The buttons of the subscription section: each runs against the store and shows its failure, never a cancel. */
export function useStoreActions() {
  const [message, setMessage] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const run = (effect: Effect.Effect<void, StoreError, AppServices>) => {
    setBusy(true)
    setMessage(null)
    runtime.runFork(
      effect.pipe(
        Effect.catch((e) =>
          Effect.sync(() => {
            if (!e.cancelled) setMessage(e.message)
          })
        ),
        Effect.ensuring(Effect.sync(() => setBusy(false)))
      )
    )
  }
  return {
    message,
    busy,
    buy: (offer: Offer) => run(Entitlement.use((e) => e.buy(offer))),
    restore: () => run(Entitlement.use((e) => e.restore)),
    redeemCode: () => run(Entitlement.use((e) => e.redeemCode)),
    manage: () => run(Entitlement.use((e) => e.manage))
  }
}
