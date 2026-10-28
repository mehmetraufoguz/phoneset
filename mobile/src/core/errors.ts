import { Schema } from "effect"

/** The network failed: unreachable, closed, timed out. */
export class TransportError extends Schema.TaggedError<TransportError>()("TransportError", {
  message: Schema.String
}) {}

/** The peer broke the protocol or sent something we cannot decode. */
export class ProtocolError extends Schema.TaggedError<ProtocolError>()("ProtocolError", {
  message: Schema.String
}) {}

/** The PC refused us (wrong code, unknown phone, busy) or failed to prove itself. */
export class AuthError extends Schema.TaggedError<AuthError>()("AuthError", {
  code: Schema.String,
  message: Schema.String
}) {}

export class StorageError extends Schema.TaggedError<StorageError>()("StorageError", {
  message: Schema.String
}) {}

export class AudioError extends Schema.TaggedError<AudioError>()("AudioError", {
  message: Schema.String
}) {}

export class DiscoveryError extends Schema.TaggedError<DiscoveryError>()("DiscoveryError", {
  message: Schema.String
}) {}

/** A QR code that is not a phoneset pairing link. */
export class PairLinkError extends Schema.TaggedError<PairLinkError>()("PairLinkError", {
  message: Schema.String
}) {}

export type ConnectError =
  | TransportError
  | ProtocolError
  | AuthError
  | StorageError
  | DiscoveryError
  | AudioError

export const messageOf = (error: { readonly message: string }): string => error.message
