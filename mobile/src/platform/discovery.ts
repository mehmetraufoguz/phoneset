import { Cause, Effect, Layer, Queue, Stream } from "effect"
import Zeroconf from "react-native-zeroconf"
import { MDNS } from "../core/constants"
import { type DiscoveredPc, Discovery } from "../core/discovery"
import { DiscoveryError } from "../core/errors"

/** iOS DNSSD code for "Local Network access denied". */
const LOCAL_NETWORK_DENIED = "-65570"

const scan = Stream.callback<ReadonlyArray<DiscoveredPc>, DiscoveryError>((queue) =>
  Effect.gen(function* () {
    const zeroconf = yield* Effect.acquireRelease(
      Effect.sync(() => new Zeroconf()),
      (z) =>
        Effect.sync(() => {
          z.stop()
          z.removeDeviceListeners()
        })
    )

    const found = new Map<string, DiscoveredPc>()
    const publish = () => Queue.offerUnsafe(queue, [...found.values()])

    zeroconf.on("resolved", (service) => {
      const id = service.txt.id
      if (!id || service.ipv4.length === 0) return
      found.set(service.name, {
        id,
        name: service.txt.n ?? service.name,
        hosts: service.ipv4,
        port: service.port
      })
      publish()
    })
    zeroconf.on("remove", (name) => {
      if (found.delete(name)) publish()
    })
    zeroconf.on("error", (error) => {
      // A single service failing to resolve is routine; losing network access is not.
      if (String(error.code) === LOCAL_NETWORK_DENIED) {
        Queue.failCauseUnsafe(
          queue,
          Cause.fail(
            new DiscoveryError({
              message: "Allow Local Network access for phoneset in Settings to find your PC."
            })
          )
        )
      }
    })

    zeroconf.scan({ type: MDNS.type, protocol: MDNS.protocol, domain: MDNS.domain })
    publish()
  })
)

export const DiscoveryLive = Layer.succeed(Discovery, { scan })
