// phoneset code claim page. A token link opens a page (no database access on GET, so a mail
// scanner that prefetches the link cannot use it up); pressing the button POSTs the token, and for each kind of code
// the token is for (the app, the camera subscription, or both) the code that ends first, and still has enough days
// left, is bound to it. Opening the same link again
// shows the same code. A token is signed with a secret key, so a made-up token is turned away without
// the database being touched. The same API also serves the page on rauf.app (a static site), so it answers
// cross-origin calls from the sites listed in ALLOWED_ORIGINS and from no one else.

interface D1Statement {
  bind(...values: unknown[]): D1Statement
  first<T = Record<string, unknown>>(): Promise<T | null>
  all<T = Record<string, unknown>>(): Promise<{ results: T[] }>
  run(): Promise<unknown>
}
interface Env {
  DB: { prepare(sql: string): D1Statement }
  // base64url, set with `wrangler secret put CLAIM_KEY`; the same key signs the links made by claim/cli
  CLAIM_KEY?: string
  // Comma separated list of sites whose pages may call the API from a browser. Default: https://rauf.app.
  // Only set it in .dev.vars, to test with a local site.
  ALLOWED_ORIGINS?: string
}

type Kind = "app" | "video"
type Issued = { kind: Kind; code: string; valid_until: string; store: string }
type Token = { expires_at: number; kinds: string; store: string }

// A token is 32 bytes: 16 random bytes, then the first 16 bytes of HMAC-SHA256(key, DOMAIN + those 16 bytes).
// Forging one means guessing 128 bits.
const DOMAIN = "phoneset-claim-v1"

// A code is only handed out while it has at least this many days left.
const MIN_DAYS_LEFT = 7
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

const tokenRow = (env: Env, hash: string) =>
  env.DB.prepare("SELECT expires_at, kinds, store FROM tokens WHERE hash = ?").bind(hash).first<Token>()

const issuedTo = async (env: Env, hash: string): Promise<Issued[]> =>
  (
    await env.DB.prepare(
      "SELECT kind, code, valid_until, store FROM codes WHERE token_hash = ? ORDER BY kind"
    )
      .bind(hash)
      .all<Issued>()
  ).results

// The answer: every code the link has. "code" and "useBefore" repeat the first one, for a page that only knows one code.
function answer(codes: Issued[]): Response {
  const list = codes.map((c) => ({
    kind: c.kind,
    store: c.store,
    code: c.code,
    useBefore: c.valid_until
  }))
  const first = list[0]
  return json({ codes: list, code: first?.code, useBefore: first?.useBefore })
}

const wantedKinds = (kinds: string): Kind[] =>
  kinds.split(",").filter((k): k is Kind => k === "app" || k === "video")

async function claim(env: Env, token: string): Promise<Response> {
  if (!env.CLAIM_KEY) return json({ error: "failed" }, 500)
  // A made-up token ends here and costs no database read at all.
  if (!(await signedByUs(token, env.CLAIM_KEY))) return json({ error: "invalid" }, 404)

  const hash = await sha256Hex(token)
  const row = await tokenRow(env, hash)
  if (!row) return json({ error: "invalid" }, 404)

  const wanted = wantedKinds(row.kinds)
  const have = await issuedTo(env, hash)
  // Already claimed in full: show it again, even after the token's own seven days are over.
  if (wanted.length > 0 && wanted.every((k) => have.some((c) => c.kind === k))) return answer(have)
  if (row.expires_at < Date.now() / 1000) return json({ error: "expired" }, 410)

  const missing = wanted.filter((k) => !have.some((c) => c.kind === k))
  const taken: Issued[] = []
  try {
    for (const kind of missing) {
      // One statement per kind, so two people pressing the button at the same moment cannot get the same code.
      const { results } = await env.DB.prepare(
        `UPDATE codes SET token_hash = ?1
         WHERE code = (
          SELECT code FROM codes
          WHERE token_hash IS NULL AND kind = ?2 AND store = ?3
            AND valid_until >= date('now', '+${MIN_DAYS_LEFT} days')
          ORDER BY valid_until ASC, code ASC LIMIT 1)
         RETURNING kind, code, valid_until, store`
      )
        .bind(hash, kind, row.store)
        .all<Issued>()
      const got = results[0]
      if (!got) throw new Out()
      taken.push(got)
    }
    return answer([...have, ...taken])
  } catch (error) {
    // Not all kinds could be given: put back what was taken in this press, so a person is never left with half.
    for (const t of taken) {
      await env.DB.prepare("UPDATE codes SET token_hash = NULL WHERE code = ?1 AND token_hash = ?2")
        .bind(t.code, hash)
        .run()
    }
    if (error instanceof Out) return json({ error: "out" }, 503)
    // The unique index fired: a parallel press of the same link won, so show what it got.
    const again = await issuedTo(env, hash)
    if (wanted.length > 0 && wanted.every((k) => again.some((c) => c.kind === k))) return answer(again)
    throw error
  }
}

class Out extends Error {}

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
<p id="lead">phoneset turns your phone into the microphone, headphones and camera of your PC. This page shows what was reserved for you.</p>
<button id="go" type="button">Show my code</button>
<div id="codes"></div>
<p id="msg" class="msg"></p>
<div id="steps" class="steps">
<strong>How to redeem</strong>
<ol><li>iPhone: open the App Store app, tap your profile picture, then <em>Redeem Gift Card or Code</em>, and enter the code. In the phoneset app you can also use <em>Redeem a code</em> in Settings.</li><li>Android: open <a href="https://play.google.com/redeem" rel="noopener noreferrer">play.google.com/redeem</a> and enter the code.</li><li>A code works once, and stops working on the date shown with it.</li></ol>
<p class="small">The stores describe this for gift cards and app codes; the wording can differ a little for a developer code.</p>
<strong>Then set up your PC</strong>
<p>phoneset also needs a free program on your Windows PC or Mac: <a href="https://github.com/mehmetraufoguz/phoneset#1-on-the-pc" rel="noopener noreferrer">install steps</a>.</p>
</div></main>
<script nonce="${nonce}">
const token = location.pathname.split("/")[2] || "";
const go = document.getElementById("go"), codesEl = document.getElementById("codes"), msg = document.getElementById("msg");
const text = { invalid: "This link is not valid. Please check that you copied all of it.", expired: "This link has expired. Reply to my email and I will send you a new one.", out: "All the codes are taken for now. Reply to my email and I will sort it out.", failed: "Something went wrong. Please try again in a moment." };
go.addEventListener("click", async () => {
	go.disabled = true; msg.style.display = "none";
	try {
		const res = await fetch("/api/claim", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ token }) });
		const body = await res.json().catch(() => ({}));
		if (!res.ok) { msg.textContent = text[body.error] || text.failed; msg.style.display = "block"; go.disabled = false; return; }
		const list = Array.isArray(body.codes) ? body.codes : [{ kind: "app", code: body.code, useBefore: body.useBefore }];
		for (const c of list) {
			const label = document.createElement("p"); label.textContent = (c.kind === "video" ? "Camera subscription" : "The app") + ", use before " + c.useBefore;
			const box = document.createElement("div"); box.className = "code"; box.style.display = "block"; box.textContent = c.code;
			box.addEventListener("click", () => navigator.clipboard && navigator.clipboard.writeText(c.code));
			codesEl.append(label, box);
		}
		document.getElementById("steps").style.display = "block"; go.style.display = "none";
		document.getElementById("lead").textContent = "Here it is. Keep this page open, or copy the code.";
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
