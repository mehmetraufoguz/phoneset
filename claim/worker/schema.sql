-- The Worker only knows which token got which code. Names and email addresses live on the
-- owner's Mac (claim/cli), never here.
--
-- A code has a kind (the app itself, or one subscription for the camera, "video") and a store (apple or google).
-- valid_until is the last day the page shows ("use it before"); it is one day earlier than the real end, because
-- a store does not say at which hour a code ends.
CREATE TABLE IF NOT EXISTS codes (
	code TEXT PRIMARY KEY,
	generated_on TEXT NOT NULL CHECK (generated_on GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
	token_hash TEXT,
	kind TEXT NOT NULL DEFAULT 'app' CHECK (kind IN ('app', 'video')),
	store TEXT NOT NULL DEFAULT 'apple' CHECK (store IN ('apple', 'google')),
	valid_until TEXT NOT NULL CHECK (valid_until GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]')
) WITHOUT ROWID;

-- One code of each kind per token, and a token that already has a code of a kind is never given another one.
CREATE UNIQUE INDEX IF NOT EXISTS codes_token_kind ON codes (token_hash, kind) WHERE token_hash IS NOT NULL;
CREATE INDEX IF NOT EXISTS codes_free ON codes (kind, store, valid_until) WHERE token_hash IS NULL;

-- kinds: "app", "video" or "app,video"; store: which store the person uses.
CREATE TABLE IF NOT EXISTS tokens (
	hash TEXT PRIMARY KEY,
	expires_at INTEGER NOT NULL,
	kinds TEXT NOT NULL DEFAULT 'app' CHECK (kinds IN ('app', 'video', 'app,video')),
	store TEXT NOT NULL DEFAULT 'apple' CHECK (store IN ('apple', 'google'))
) WITHOUT ROWID;
