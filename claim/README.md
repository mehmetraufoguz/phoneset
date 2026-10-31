# Promo code claim

Gives App Store promo codes to the people you choose, without handing the codes out in public.

- `worker/`: a Cloudflare Worker with a D1 database. A link `/c/<token>` shows a page; pressing its button
  binds the oldest promo code that still has 7+ days left to that token. Opening the link again shows the same
  code. The database only holds codes and token hashes: no names, no emails.
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
wrangler deploy                   # then add a custom domain such as claim.rauf.app
../cli/promo key show | wrangler secret put CLAIM_KEY   # the Worker gets the same key (piped, never printed)
```

Until the secret is set the Worker answers every press with an error. The first `promo` run writes
`docs/store/private/promo/config.json`; set `baseUrl` there to the Worker's address.
Everything in `docs/store/private/` is git-ignored: the people file (names, emails, the raw links) lives there.
A rate limit on `/api/claim` is a good extra once the Worker has a custom domain: Cloudflare's free plan has
rate limiting rules (counted per IP, 10 second window); they do not apply on the `workers.dev` address.

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
```
