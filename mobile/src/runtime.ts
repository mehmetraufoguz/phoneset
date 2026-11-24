import { Layer, ManagedRuntime } from "effect"
import { CameraPrefs } from "./core/cameraPrefs"
import { Entitlement } from "./core/entitlement"
import { Presence } from "./core/presence"
import { ScreenPrefs } from "./core/screenPrefs"
import { Session } from "./core/session"
import { SocketFactory } from "./core/socket"
import { PairedPcs } from "./core/storage"
import { AudioLive } from "./platform/audio"
import { DeviceInfoLive, EntropyLive, KeyValueLive } from "./platform/basics"
import { CameraLive } from "./platform/camera"
import { CryptoLive } from "./platform/crypto"
import { DiscoveryLive } from "./platform/discovery"
import { PresenceLive } from "./platform/presence"
import { StoreLive } from "./platform/store"

const PlatformLive = Layer.mergeAll(
  EntropyLive,
  CryptoLive,
  KeyValueLive,
  DeviceInfoLive,
  DiscoveryLive,
  AudioLive,
  CameraLive,
  PresenceLive,
  StoreLive,
  SocketFactory.layer
)

/** Everything the UI can ask for: Session, PairedPcs and the platform services. */
export const AppLayer = Presence.watcher.pipe(
  Layer.provideMerge(Session.layer),
  Layer.provideMerge(Entitlement.layer),
  Layer.provideMerge(CameraPrefs.layer),
  Layer.provideMerge(ScreenPrefs.layer),
  Layer.provideMerge(PairedPcs.layer),
  Layer.provideMerge(PlatformLive)
)

export const runtime = ManagedRuntime.make(AppLayer)
export type AppServices = Layer.Success<typeof AppLayer>
