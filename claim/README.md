# Code claim

Gives codes to the people you choose, without handing them out in public: App Store promo codes for the app, subscription offer
codes for the camera subscription (kind `video`), and the same for Google Play. One link can carry the app code, the
subscription code, or both ("two codes per winner").

- `worker/`: a Cloudflare Worker with a D1 database, the API at `https://claim-phoneset.rauf.app/api/claim`.
  Pressing the button binds to the token one code of each kind it is for: the one that ends first and still has 7+
  days left. If one kind has no code left, nothing is given (a person never gets half). Asking again shows the same
  codes. The database only holds codes and token hashes: no names, no emails. The Worker also has a
  plain page of its own at `/c/<token>`, a fallback.
- The page people open lives in the rauf.app site (repository `raufapp`): `src/pages/phoneset/claim.mdx` and
  `src/components/work/ClaimCode.client.tsx`, at `https://rauf.app/phoneset/claim#<token>`. It is a static page;
  its script reads the token from the part after `#` and calls the API from the browser.
- `cli/`: a small Go program (`promo`) on your Mac. It keeps the people, makes the links, writes the emails and
  shows who is at which step. It reaches the database through `wrangler`, so there is no admin endpoint.

Apple's rules this is built around: an app promo code is single use, valid for 4 weeks from the day it was
generated, at most 100 per app version. People who redeem one cannot rate or review the app, and codes must
not be traded for reviews. A subscription offer code has the end date you chose when you made it in App Store Connect
(one-time-use codes up to 6 months; Google Play subscription promo codes are a free trial of 3 to 90 days; both stores'
help pages, read 2026-11-16). The page does not say how long the free time is, because the code itself decides that.

## Rules the tools enforce

- Codes are given in the order they end. A code is only given while it has 7+ days left (`valid_until`, the day the
  page shows, which is one day before the real end).
- A link works for at most 7 days. `promo mint` refuses when the links waiting for a kind of code (in one store)
  would outnumber the codes of that kind and store that still have 14+ days left (7 days for the link plus 7 to use
  the code).
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

## Upgrading a database made before the camera subscription

Once, before deploying the new Worker: `cd claim/worker && wrangler d1 execute phoneset-claim --remote --file=migrate-1-kinds.sql`.
Existing codes become `app` / `apple`; links already made keep working. A fresh database uses `schema.sql` as it is.
Deploy the Worker after the migration (the old Worker would not read the new columns' rules), then publish the page.

## Use

```sh
promo codes import --generated 2026-11-01 codes.txt   # App Store promo codes for the app: the date ASC shows under Promo Codes, History
promo codes import --kind video --valid-until 2027-05-01 offer-codes.txt   # subscription offer codes: the last day they work
promo codes import --kind video --store google --valid-until 2027-05-01 google-codes.txt   # Google Play (always --valid-until)
promo add --name "Ali" --email ali@example.com --channel twitch
promo mint ali                         # a link for the app (App Store)
promo mint ali --kinds app,video       # app code and subscription code on one link
promo mint ali --kinds video --store google   # a Google Play subscription code
promo mail ali          # email text on the clipboard and in your mail app; you send it
promo list              # who is where: new, minted, mailed, claimed, expired, revoked
promo codes status      # per kind and store: how many codes and links are left
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

## Lists of people (testers, giveaways, other apps)

People live in `people.json` on your Mac; a person can be on any number of named lists. The database never sees lists or emails.

```bash
promo group import play-testers people.csv   # CSV: name,email[,channel[,note]]; a known email is only put on the list
promo add --name "Ali" --email ali@example.com --group play-testers,giveaway
promo group ls                               # lists and their sizes
promo group export play-testers              # emails, one per line: paste or upload into Play's closed-test tester list
promo list --group play-testers              # who has a link, who pressed the button
promo mint --group play-testers --kinds app --store google   # a link for everyone on the list who has none (stops when codes run out)
```

`promo group add|rm <list> <id>...` moves people. The same list can be reused by another app: the Worker's code pool is phoneset's, but
the list and the export are not.

For the 12-testers-for-14-days rule of a personal Play account: the export gives Play the emails, the testers opt in with their own
Google account, and the paid app is installed with a promo code (or the claim link, once Play codes are imported). Whether Play's promo
codes cover closed-test installs of a paid app is not verified.
