// phoneset promo code claim page. A token link opens a page (no database access on GET, so a mail
// scanner that prefetches the link cannot use it up); pressing the button POSTs the token, and the
// oldest promo code that still has enough days left is bound to it. Opening the same link again
// shows the same code. A token is signed with a secret key, so a made-up token is turned away without
// the database being touched. The same API also serves the page on rauf.app (a static site), so it answers
// cross-origin calls from the sites listed in ALLOWED_ORIGINS and from no one else.

interface D1Statement {
  bind(...values: unknown[]): D1Statement
  first<T = Record<string, unknown>>(): Promise<T | null>
  all<T = Record<string, unknown>>(): Promise<{ results: T[] }>
}
interface Env {
  DB: { prepare(sql: string): D1Statement }
  // base64url, set with `wrangler secret put CLAIM_KEY`; the same key signs the links made by claim/cli
  CLAIM_KEY?: string
  // Comma separated list of sites whose pages may call the API from a browser. Default: https://rauf.app.
  // Only set it in .dev.vars, to test with a local site.
  ALLOWED_ORIGINS?: string
}

type Claimed = { code: string; generated_on: string }
type Lookup = { expires_at: number; code: string | null; generated_on: string | null }

// A token is 32 bytes: 16 random bytes, then the first 16 bytes of HMAC-SHA256(key, DOMAIN + those 16 bytes).
// Forging one means guessing 128 bits.
const DOMAIN = "phoneset-claim-v1"

// Apple: a promo code is valid for four weeks from the day it was generated.
const CODE_DAYS = 28
// A code is only handed out while it has at least this many days left (so generated_on is at most 21 days old).
const MAX_AGE_DAYS = 21
const TOKEN = /^[A-Za-z0-9_-]{43}$/
const DEFAULT_ORIGINS = ["https://rauf.app"]
const DAY = 86_400_000

function toBase64Url(bytes: Uint8Array): string {
  let text = ""
  for (const b of bytes) text += String.fromCharCode(b)
  return btoa(text).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "")
}

function fromBase64Url(text: string): Uint8Array | null {
  try {
    const decoded = atob(text.replace(/-/g, "+").replace(/_/g, "/"))
    return Uint8Array.from(decoded, (c) => c.charCodeAt(0))
  } catch {
    return null
  }
}

// True only for a token made with our key. Runs before any database query.
async function signedByUs(token: string, keyText: string): Promise<boolean> {
  const bytes = fromBase64Url(token)
  // Re-encoding must give the same text, so one token has exactly one spelling.
  if (bytes?.length !== 32 || toBase64Url(bytes) !== token) return false
  const keyBytes = fromBase64Url(keyText)
  if (!keyBytes || keyBytes.length < 16) return false
  const key = await crypto.subtle.importKey(
    "raw",
    new Uint8Array(keyBytes),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"]
  )
  const message = new Uint8Array([...new TextEncoder().encode(DOMAIN), ...bytes.slice(0, 16)])
  const mac = new Uint8Array(await crypto.subtle.sign("HMAC", key, message))
  let diff = 0
  for (let i = 0; i < 16; i++) diff |= (mac[i] ?? 0) ^ (bytes[16 + i] ?? 0)
  return diff === 0
}

async function sha256Hex(text: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text))
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("")
}

// The day shown to the person is one day earlier than the real end: Apple does not say at which hour a code ends.
function useBefore(generatedOn: string): string {
  return new Date(Date.parse(`${generatedOn}T00:00:00Z`) + (CODE_DAYS - 1) * DAY)
    .toISOString()
    .slice(0, 10)
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      "content-type": "application/json",
      "cache-control": "no-store",
      "x-robots-tag": "noindex, nofollow"
    }
  })
}

// One query for everything the token has: its end time, and the code if it already got one.
const lookup = (env: Env, hash: string) =>
  env.DB.prepare(
    `SELECT t.expires_at AS expires_at, c.code AS code, c.generated_on AS generated_on
     FROM tokens t LEFT JOIN codes c ON c.token_hash = t.hash
     WHERE t.hash = ?`
  )
    .bind(hash)
    .first<Lookup>()

const shown = (row: { code: string | null; generated_on: string | null }) =>
  row.code && row.generated_on
    ? json({ code: row.code, useBefore: useBefore(row.generated_on) })
    : null

async function claim(env: Env, token: string): Promise<Response> {
  if (!env.CLAIM_KEY) return json({ error: "failed" }, 500)
  // A made-up token ends here and costs no database read at all.
  if (!(await signedByUs(token, env.CLAIM_KEY))) return json({ error: "invalid" }, 404)

  const hash = await sha256Hex(token)
  const row = await lookup(env, hash)
  if (!row) return json({ error: "invalid" }, 404)

  // Already claimed: show it again, even after the token's own seven days are over.
  const done = shown(row)
  if (done) return done
  if (row.expires_at < Date.now() / 1000) return json({ error: "expired" }, 410)

  try {
    // One statement, so two people pressing the button at the same moment cannot get the same code.
    const { results } = await env.DB.prepare(
      `UPDATE codes SET token_hash = ?1
			 WHERE code = (
				SELECT code FROM codes
				WHERE token_hash IS NULL AND generated_on >= date('now', '-${MAX_AGE_DAYS} days')
				ORDER BY generated_on ASC, code ASC LIMIT 1)
			 RETURNING code, generated_on`
    )
      .bind(hash)
      .all<Claimed>()
    const got = results[0]
    if (!got) return json({ error: "out" }, 503)
    return json({ code: got.code, useBefore: useBefore(got.generated_on) })
  } catch (error) {
    // The unique index on token_hash fired: a parallel press of the same link won, so show its code.
    const again = await lookup(env, hash)
    const answer = again && shown(again)
    if (answer) return answer
    throw error
  }
}

function page(nonce: string): Response {
  const html = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex, nofollow"><meta name="referrer" content="no-referrer"><title>phoneset</title>
<style nonce="${nonce}">
:root{color-scheme:light}*{box-sizing:border-box}body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;background:#3B6BFF;font:17px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;color:#0B1633}
main{width:100%;max-width:520px;background:#fff;border-radius:24px;padding:32px;box-shadow:0 30px 80px rgba(11,22,51,.35)}
h1{margin:0 0 8px;font-size:30px;letter-spacing:-.02em}p{margin:0 0 16px;color:#44526F}
button{font:inherit;font-weight:700;border:0;border-radius:14px;padding:14px 22px;background:#3B6BFF;color:#fff;cursor:pointer}button:disabled{opacity:.6;cursor:default}
.code{display:none;margin:20px 0;padding:18px;border-radius:14px;background:#F4F7FF;font:700 28px/1.2 Menlo,Consolas,monospace;letter-spacing:.06em;text-align:center;word-break:break-all}
.msg{display:none;margin:16px 0 0;color:#B3261E;font-weight:600}.steps{display:none;margin-top:8px}.steps ol{margin:8px 0 16px;padding-left:22px;color:#44526F}
.small{font-size:14px}a{color:#2850E0}
</style></head><body><main>
<h1>Your phoneset code</h1>
<p id="lead">phoneset turns your iPhone into the microphone and headphones of your PC. This page shows your one free copy of the app.</p>
<button id="go" type="button">Show my code</button>
<div id="code" class="code" aria-live="polite"></div>
<p id="msg" class="msg"></p>
<div id="steps" class="steps">
<p id="valid"></p>
<strong>Redeem it on your iPhone</strong>
<ol><li>Open the App Store app and tap your profile picture (or Sign in) at the top.</li><li>Tap <em>Redeem Gift Card or Code</em>, then enter the code (or use the camera).</li><li>Tap Redeem. The code works once.</li></ol>
<p class="small">Apple describes this for gift cards and app codes; the wording can differ a little for a developer code.</p>
<strong>Then set up your PC</strong>
<p>phoneset also needs a free program on your Windows PC or Mac: <a href="https://github.com/mehmetraufoguz/phoneset#1-on-the-pc" rel="noopener noreferrer">install steps</a>. The app is for iPhone (iOS 16.4 or later).</p>
</div></main>
<script nonce="${nonce}">
const token = location.pathname.split("/")[2] || "";
const go = document.getElementById("go"), codeEl = document.getElementById("code"), msg = document.getElementById("msg");
const text = { invalid: "This link is not valid. Please check that you copied all of it.", expired: "This link has expired. Reply to my email and I will send you a new one.", out: "All the codes are taken for now. Reply to my email and I will sort it out.", failed: "Something went wrong. Please try again in a moment." };
go.addEventListener("click", async () => {
	go.disabled = true; msg.style.display = "none";
	try {
		const res = await fetch("/api/claim", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ token }) });
		const body = await res.json().catch(() => ({}));
		if (!res.ok) { msg.textContent = text[body.error] || text.failed; msg.style.display = "block"; go.disabled = false; return; }
		codeEl.textContent = body.code; codeEl.style.display = "block";
		document.getElementById("valid").textContent = "Use it before " + body.useBefore + ". After that Apple no longer accepts it.";
		document.getElementById("steps").style.display = "block"; go.style.display = "none";
		document.getElementById("lead").textContent = "Here it is. Keep this page open, or copy the code.";
		codeEl.addEventListener("click", () => navigator.clipboard && navigator.clipboard.writeText(body.code));
	} catch { msg.textContent = text.failed; msg.style.display = "block"; go.disabled = false; }
});
</script></body></html>`
  return new Response(html, {
    headers: {
      "content-type": "text/html; charset=utf-8",
      "cache-control": "no-store",
      "x-robots-tag": "noindex, nofollow",
      "referrer-policy": "no-referrer",
      "x-content-type-options": "nosniff",
      "content-security-policy": `default-src 'none'; style-src 'nonce-${nonce}'; script-src 'nonce-${nonce}'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`
    }
  })
}

function allowedOrigins(env: Env): string[] {
  const listed = (env.ALLOWED_ORIGINS ?? "")
    .split(",")
    .map((o) => o.trim())
    .filter(Boolean)
  return listed.length ? listed : DEFAULT_ORIGINS
}

function withHeaders(response: Response, extra: Record<string, string>): Response {
  for (const [name, value] of Object.entries(extra)) response.headers.set(name, value)
  return response
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url)
    if (request.method === "GET" && /^\/c\/[A-Za-z0-9_-]{43}$/.test(url.pathname)) {
      return page(crypto.randomUUID().replace(/-/g, ""))
    }
    if (
      url.pathname === "/api/claim" &&
      (request.method === "POST" || request.method === "OPTIONS")
    ) {
      // A browser sends Origin on every cross-site call. The Worker's own page (same origin) and tools like
      // curl are fine; any other site is turned away. This keeps other sites' pages from reading answers; the
      // signed token is what actually protects the codes.
      const origin = request.headers.get("origin")
      const crossSite = origin !== null && origin !== url.origin
      if (crossSite && !allowedOrigins(env).includes(origin)) return json({ error: "invalid" }, 403)
      const cors: Record<string, string> = { vary: "origin" }
      if (crossSite) cors["access-control-allow-origin"] = origin

      if (request.method === "OPTIONS") {
        return withHeaders(new Response(null, { status: 204 }), {
          ...cors,
          "access-control-allow-methods": "POST",
          "access-control-allow-headers": "content-type",
          "access-control-max-age": "86400",
          "cache-control": "no-store"
        })
      }
      const body = (await request.json().catch(() => null)) as { token?: unknown } | null
      if (!body || typeof body.token !== "string" || !TOKEN.test(body.token))
        return withHeaders(json({ error: "invalid" }, 400), cors)
      try {
        return withHeaders(await claim(env, body.token), cors)
      } catch {
        // Never show a stack trace; the page tells the person to try again.
        return withHeaders(json({ error: "failed" }, 500), cors)
      }
    }
    return new Response("Not found", { status: 404, headers: { "cache-control": "no-store" } })
  }
}
