import { Layer, ManagedRuntime } from "effect"
import { Presence } from "./core/presence"
import { Session } from "./core/session"
import { SocketFactory } from "./core/socket"
import { PairedPcs } from "./core/storage"
import { AudioLive } from "./platform/audio"
import { DeviceInfoLive, EntropyLive, KeyValueLive } from "./platform/basics"
import { DiscoveryLive } from "./platform/discovery"
import { PresenceLive } from "./platform/presence"

const PlatformLive = Layer.mergeAll(
  EntropyLive,
  KeyValueLive,
  DeviceInfoLive,
  DiscoveryLive,
  AudioLive,
  PresenceLive,
  SocketFactory.layer
)

/** Everything the UI can ask for: Session, PairedPcs and the platform services. */
export const AppLayer = Presence.watcher.pipe(
  Layer.provideMerge(Session.layer),
  Layer.provideMerge(PairedPcs.layer),
  Layer.provideMerge(PlatformLive)
)

export const runtime = ManagedRuntime.make(AppLayer)
export type AppServices = Layer.Success<typeof AppLayer>
