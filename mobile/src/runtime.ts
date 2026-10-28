import { Layer, ManagedRuntime } from "effect"
import { Session } from "./core/session"
import { SocketFactory } from "./core/socket"
import { PairedPcs } from "./core/storage"
import { AudioLive } from "./platform/audio"
import { DeviceInfoLive, EntropyLive, KeyValueLive } from "./platform/basics"
import { DiscoveryLive } from "./platform/discovery"

const PlatformLive = Layer.mergeAll(
  EntropyLive,
  KeyValueLive,
  DeviceInfoLive,
  DiscoveryLive,
  AudioLive,
  SocketFactory.layer
)

/** Everything the UI can ask for: Session, PairedPcs and the platform services. */
export const AppLayer = Session.layer.pipe(
  Layer.provideMerge(PairedPcs.layer),
  Layer.provideMerge(PlatformLive)
)

export const runtime = ManagedRuntime.make(AppLayer)
export type AppServices = Layer.Success<typeof AppLayer>
