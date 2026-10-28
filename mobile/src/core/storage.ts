import { Context, Effect, Layer, Schema } from "effect"
import { toHex } from "./bytes"
import { Entropy } from "./entropy"
import { StorageError } from "./errors"

/** Secret-safe key/value storage. Provided by the platform (Keychain / Keystore). */
export class KeyValue extends Context.Service<
  KeyValue,
  {
    readonly get: (key: string) => Effect.Effect<string | undefined, StorageError>
    readonly set: (key: string, value: string) => Effect.Effect<void, StorageError>
    readonly remove: (key: string) => Effect.Effect<void, StorageError>
  }
>()("phoneset/KeyValue") {}

/** What this phone calls itself. Provided by the platform. */
export class DeviceInfo extends Context.Service<
  DeviceInfo,
  {
    readonly name: string
    readonly platform: "ios" | "android"
  }
>()("phoneset/DeviceInfo") {}

export const PairedPc = Schema.Struct({
  id: Schema.String,
  name: Schema.String,
  /** base64; also the HMAC key that proves this phone to the PC. */
  token: Schema.String,
  hosts: Schema.Array(Schema.String),
  port: Schema.Number,
  pairedAt: Schema.Number
})
export type PairedPc = typeof PairedPc.Type

const StoredPcs = Schema.fromJsonString(Schema.Array(PairedPc))
const decodeStored = Schema.decodeUnknownEffect(StoredPcs)
const encodeStored = Schema.encodeEffect(StoredPcs)

export interface PhoneIdentity {
  readonly deviceId: string
  readonly name: string
  readonly platform: "ios" | "android"
}

const PCS_KEY = "phoneset.pcs"
const DEVICE_KEY = "phoneset.deviceId"

/** The PCs this phone has paired with, and this phone's own identity. */
export class PairedPcs extends Context.Service<
  PairedPcs,
  {
    readonly list: Effect.Effect<ReadonlyArray<PairedPc>, StorageError>
    readonly save: (pc: PairedPc) => Effect.Effect<void, StorageError>
    readonly remove: (id: string) => Effect.Effect<void, StorageError>
    readonly identity: Effect.Effect<PhoneIdentity, StorageError>
  }
>()("phoneset/PairedPcs") {
  static readonly layer = Layer.effect(
    PairedPcs,
    Effect.gen(function* () {
      const kv = yield* KeyValue
      const entropy = yield* Entropy
      const device = yield* DeviceInfo

      const list = Effect.gen(function* () {
        const raw = yield* kv.get(PCS_KEY)
        if (raw === undefined) return [] as ReadonlyArray<PairedPc>
        return yield* decodeStored(raw)
      }).pipe(
        Effect.mapError((e) =>
          e instanceof StorageError
            ? e
            : new StorageError({ message: "Saved PCs couldn't be read." })
        )
      )

      const write = (pcs: ReadonlyArray<PairedPc>) =>
        encodeStored(pcs).pipe(
          Effect.mapError(() => new StorageError({ message: "Saved PCs couldn't be written." })),
          Effect.flatMap((json) => kv.set(PCS_KEY, json))
        )

      const save = (pc: PairedPc) =>
        Effect.flatMap(list, (pcs) => write([...pcs.filter((p) => p.id !== pc.id), pc]))

      const remove = (id: string) =>
        Effect.flatMap(list, (pcs) => write(pcs.filter((p) => p.id !== id)))

      const identity = Effect.gen(function* () {
        let deviceId = yield* kv.get(DEVICE_KEY)
        if (deviceId === undefined) {
          deviceId = toHex(entropy.bytes(16))
          yield* kv.set(DEVICE_KEY, deviceId)
        }
        return { deviceId, name: device.name, platform: device.platform } satisfies PhoneIdentity
      })

      return PairedPcs.of({ list, save, remove, identity })
    })
  )
}
