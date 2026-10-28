import { Context, Effect, Schema, type Stream } from "effect"
import { INFO_PATH, PROTOCOL_VERSION } from "./constants"
import type { DiscoveryError } from "./errors"
import { TransportError } from "./errors"

export interface DiscoveredPc {
  readonly id: string
  readonly name: string
  readonly hosts: ReadonlyArray<string>
  readonly port: number
}

/** Finds PCs advertising _phoneset._tcp. Provided by the platform (Bonjour / NSD). */
export class Discovery extends Context.Service<
  Discovery,
  {
    /**
     * Emits the PCs currently visible, every time that set changes.
     * Scanning runs while the stream is being consumed.
     */
    readonly scan: Stream.Stream<ReadonlyArray<DiscoveredPc>, DiscoveryError>
  }
>()("phoneset/Discovery") {}

const PcInfo = Schema.Struct({
  app: Schema.Literal("phoneset"),
  v: Schema.Number,
  id: Schema.String,
  name: Schema.String
})
export type PcInfo = typeof PcInfo.Type
const decodeInfo = Schema.decodeUnknownEffect(PcInfo)

const PROBE_TIMEOUT = "3 seconds"

/** Asks one address whether a phoneset PC answers there. */
export const probe = (host: string, port: number) =>
  Effect.tryPromise({
    try: async (signal) => {
      const res = await fetch(`http://${host}:${port}${INFO_PATH}`, { signal })
      return (await res.json()) as unknown
    },
    catch: () => new TransportError({ message: `${host} didn't answer.` })
  }).pipe(
    Effect.flatMap(decodeInfo),
    Effect.mapError((e) =>
      e instanceof TransportError
        ? e
        : new TransportError({ message: `${host} isn't a phoneset PC.` })
    ),
    Effect.filterOrFail(
      (info) => info.v === PROTOCOL_VERSION,
      () => new TransportError({ message: "The PC runs an incompatible phoneset version." })
    ),
    Effect.timeoutOrElse({
      duration: PROBE_TIMEOUT,
      orElse: () => Effect.fail(new TransportError({ message: `${host} didn't answer in time.` }))
    })
  )

export interface Reachable {
  readonly host: string
  readonly info: PcInfo
}

/**
 * Tries every address at once and returns the first that answers as the expected PC.
 * A PC has several addresses (Wi-Fi, Ethernet, VPN); only some are reachable from the phone.
 */
export const firstReachable = (
  hosts: ReadonlyArray<string>,
  port: number,
  expectedId?: string
): Effect.Effect<Reachable, TransportError> =>
  Effect.raceAll(
    hosts.map((host) =>
      probe(host, port).pipe(
        Effect.filterOrFail(
          (info) => expectedId === undefined || info.id === expectedId,
          () => new TransportError({ message: `${host} is a different PC.` })
        ),
        Effect.map((info): Reachable => ({ host, info }))
      )
    )
  ).pipe(
    Effect.mapError(
      () => new TransportError({ message: "Couldn't reach the PC. Are both on the same Wi-Fi?" })
    )
  )
