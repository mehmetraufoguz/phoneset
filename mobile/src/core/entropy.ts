import { Context } from "effect"

/** Source of cryptographic randomness. Provided by the platform. */
export class Entropy extends Context.Service<
  Entropy,
  {
    readonly bytes: (length: number) => Uint8Array
  }
>()("phoneset/Entropy") {}
