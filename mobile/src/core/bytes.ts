const encoder = new TextEncoder()
const decoder = new TextDecoder()

export const utf8 = (s: string): Uint8Array => encoder.encode(s)
export const fromUtf8 = (b: Uint8Array): string => decoder.decode(b)

export const concat = (...parts: ReadonlyArray<Uint8Array>): Uint8Array => {
  let size = 0
  for (const p of parts) size += p.length
  const out = new Uint8Array(size)
  let offset = 0
  for (const p of parts) {
    out.set(p, offset)
    offset += p.length
  }
  return out
}

export const u32be = (n: number): Uint8Array => {
  const b = new Uint8Array(4)
  new DataView(b.buffer).setUint32(0, n >>> 0, false)
  return b
}

const ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

export const toBase64 = (bytes: Uint8Array): string => {
  let out = ""
  for (let i = 0; i < bytes.length; i += 3) {
    const a = bytes[i] ?? 0
    const b = bytes[i + 1] ?? 0
    const c = bytes[i + 2] ?? 0
    const n = (a << 16) | (b << 8) | c
    out += ALPHABET[(n >> 18) & 63]
    out += ALPHABET[(n >> 12) & 63]
    out += i + 1 < bytes.length ? ALPHABET[(n >> 6) & 63] : "="
    out += i + 2 < bytes.length ? ALPHABET[n & 63] : "="
  }
  return out
}

/** Accepts standard and URL-safe alphabets, with or without padding. */
export const fromBase64 = (input: string): Uint8Array => {
  const s = input.replace(/-/g, "+").replace(/_/g, "/").replace(/=+$/, "")
  const out = new Uint8Array(Math.floor((s.length * 3) / 4))
  let acc = 0
  let bits = 0
  let o = 0
  for (const ch of s) {
    const v = ALPHABET.indexOf(ch)
    if (v < 0) throw new Error("invalid base64")
    acc = (acc << 6) | v
    bits += 6
    if (bits >= 8) {
      bits -= 8
      out[o++] = (acc >> bits) & 0xff
    }
  }
  return out
}

export const toHex = (bytes: Uint8Array): string =>
  Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("")
