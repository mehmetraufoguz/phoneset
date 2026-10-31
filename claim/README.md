# Promo code claim

Gives App Store promo codes to the people you choose, without handing the codes out in public.

- `worker/`: a Cloudflare Worker with a D1 database, the API at `https://claim-phoneset.rauf.app/api/claim`.
  Pressing the button binds the oldest promo code that still has 7+ days left to the token. Asking again shows
  the same code. The database only holds codes and token hashes: no names, no emails. The Worker also has a
  plain page of its own at `/c/<token>`, a fallback.
- The page people open lives in the rauf.app site (repository `raufapp`): `src/pages/phoneset/claim.mdx` and
  `src/components/work/ClaimCode.client.tsx`, at `https://rauf.app/phoneset/claim#<token>`. It is a static page;
  its script reads the token from the part after `#` and calls the API from the browser.
- `cli/`: a small Go program (`promo`) on your Mac. It keeps the people, makes the links, writes the emails and
  shows who is at which step. It reaches the database through `wrangler`, so there is no admin endpoint.

Apple's rules this is built around: a promo code is single use, valid for 4 weeks from the day it was
generated, at most 100 per app version. People who redeem one cannot rate or review the app, and codes must
not be traded for reviews.

## Rules the tools enforce

- Codes are given oldest first. A code is only given while it has 7+ days left.
- A link works for at most 7 days. `promo mint` refuses when the links waiting would outnumber the codes that
  still have 14+ days left (7 days for the link plus 7 to use the code).
- A link that was pressed keeps its code, also after the link's own days are over. Such a link cannot be revoked.
- The page only shows the code after a button press (POST), because mail scanners open links in advance.
- The token travels after the `#` of the page address, which a browser never sends to a server, so it is not in
  this site's logs. It leaves the browser only in the button's request.
- Links are signed. A token is 16 random bytes plus 16 bytes of an HMAC made with a secret key; the Worker checks
  that first, so a made-up or altered link is turned away without any database read (a flood of fake links cannot
  use up the D1 limits). A valid link then costs one query, plus one write the first time it is pressed.
- If the key is replaced, links that were not pressed yet stop working; pressed ones keep showing their code.

## Set up (once)

```sh
cd claim/cli && go build -o promo .
./promo key init                  # makes the signing key and keeps it in the macOS Keychain

cd ../worker
wrangler login
wrangler d1 create phoneset-claim # paste the database_id it prints into wrangler.toml
wrangler d1 execute phoneset-claim --remote --file=schema.sql
wrangler deploy                   # also attaches claim-phoneset.rauf.app (the routes line in wrangler.toml)
../cli/promo key show | wrangler secret put CLAIM_KEY   # the Worker gets the same key (piped, never printed)
```

The Worker lives at https://claim-phoneset.rauf.app; the zone rauf.app has to be on the same Cloudflare account,
then Cloudflare makes the DNS record and the certificate itself (RaBalance's Workers are routed the same way).
Until the secret is set the Worker answers every press with an error. The first `promo` run writes
`docs/store/private/promo/config.json` with `baseUrl` (the Worker) and `pageUrl` (the page on rauf.app).

Publish the page before the first email goes out: push the `raufapp` repository so Cloudflare builds it, and open
`https://rauf.app/phoneset/claim` once. Leave `pageUrl` empty to send the Worker's own page instead.

The browser may only call the API from the sites in `ALLOWED_ORIGINS` (a Worker variable; the default is
`https://rauf.app`, and nothing else is needed in production).
Everything in `docs/store/private/` is git-ignored: the people file (names, emails, the raw links) lives there.
A rate limit on `/api/claim` is a good extra now that the Worker has a custom domain: Cloudflare's free plan has
rate limiting rules (counted per IP, 10 second window; the free plan can match on the path only, so the rule
covers the whole zone rauf.app); they do not apply on the `workers.dev` address.

## Use

```sh
promo codes import --generated 2026-11-01 codes.txt   # the date App Store Connect shows under Promo Codes, History
promo add --name "Ali" --email ali@example.com --channel twitch
promo mint ali          # makes the link
promo mail ali          # email text on the clipboard and in your mail app; you send it
promo list              # who is where: new, minted, mailed, claimed, expired, revoked
promo codes status      # how many codes and links are left
```

"claimed" means the person pressed the button and was shown a code. Apple does not say who redeemed a code.

## Try it without touching the real database

```sh
promo --local key init   # a separate local key; also writes claim/worker/.dev.vars (git-ignored) for the local Worker
cd claim/worker && wrangler d1 execute phoneset-claim --local --file=schema.sql && wrangler dev --local --port 8787
promo --local ...        # in another terminal; uses a separate people file, docs/store/private/promo-local
# To try the rauf.app page locally: run the site (npm run dev in raufapp, http://localhost:5173), add
# ALLOWED_ORIGINS=http://localhost:5173 to claim/worker/.dev.vars, restart the Worker, and make links with
# PROMO_PAGE_URL=http://localhost:5173/phoneset/claim promo --local mint ...
```
