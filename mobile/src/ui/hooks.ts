import { Effect, Fiber, Stream, SubscriptionRef } from "effect"
import { useEffect, useState } from "react"
import { useColorScheme } from "react-native"
import { Discovery } from "../core/discovery"
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
