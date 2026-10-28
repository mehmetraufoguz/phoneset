import { Effect, Layer } from "effect"
import { getRandomBytes } from "expo-crypto"
import * as Device from "expo-device"
import * as SecureStore from "expo-secure-store"
import { Platform } from "react-native"
import { Entropy } from "../core/entropy"
import { StorageError } from "../core/errors"
import { DeviceInfo, KeyValue } from "../core/storage"

export const EntropyLive = Layer.succeed(Entropy, {
  bytes: (length) => getRandomBytes(length)
})

// Pairing tokens are credentials: keep them in the Keychain / Keystore, on this device only.
const options: SecureStore.SecureStoreOptions = {
  keychainAccessible: SecureStore.WHEN_UNLOCKED_THIS_DEVICE_ONLY
}

const storage = <A>(run: () => Promise<A>) =>
  Effect.tryPromise({
    try: run,
    catch: () => new StorageError({ message: "Secure storage isn't available." })
  })

export const KeyValueLive = Layer.succeed(KeyValue, {
  get: (key) =>
    storage(() => SecureStore.getItemAsync(key, options)).pipe(
      Effect.map((value) => value ?? undefined)
    ),
  set: (key, value) => storage(() => SecureStore.setItemAsync(key, value, options)),
  remove: (key) => storage(() => SecureStore.deleteItemAsync(key, options))
})

export const DeviceInfoLive = Layer.succeed(DeviceInfo, {
  name: Device.deviceName ?? Device.modelName ?? "Phone",
  platform: Platform.OS === "ios" ? "ios" : "android"
})
