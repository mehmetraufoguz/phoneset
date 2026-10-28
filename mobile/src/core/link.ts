import { Effect } from "effect"
import { fromBase64 } from "./bytes"
import { DEFAULT_PORT } from "./constants"
import { PairLinkError } from "./errors"

/** What the PC's QR code carries: phoneset://pair?v=1&id=..&n=..&h=ip,ip&p=port&k=secret */
export interface PairLink {
  readonly id: string
  readonly name: string
  readonly hosts: ReadonlyArray<string>
  readonly port: number
  readonly secret: Uint8Array
}

const PREFIX = "phoneset://pair?"
const HOST = /^[A-Za-z0-9.-]+$/

export const parsePairLink = (input: string): Effect.Effect<PairLink, PairLinkError> =>
  Effect.try({
    try: () => {
      if (!input.startsWith(PREFIX)) throw new Error("not a phoneset code")
      const params = new Map<string, string>()
      for (const pair of input.slice(PREFIX.length).split("&")) {
        const eq = pair.indexOf("=")
        if (eq > 0) params.set(pair.slice(0, eq), decodeURIComponent(pair.slice(eq + 1)))
      }
      const id = params.get("id")
      const hosts = (params.get("h") ?? "").split(",").filter((h) => HOST.test(h))
      const secret = fromBase64(params.get("k") ?? "")
      const port = Number(params.get("p") ?? DEFAULT_PORT)
      if (!id || hosts.length === 0 || secret.length !== 16 || !Number.isInteger(port)) {
        throw new Error("incomplete")
      }
      return { id, name: params.get("n") ?? "PC", hosts, port, secret } satisfies PairLink
    },
    catch: () => new PairLinkError({ message: "That isn't a phoneset pairing code." })
  })
