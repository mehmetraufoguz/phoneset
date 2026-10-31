-- The Worker only knows which token got which promo code. Names and email addresses live on the
-- owner's Mac (claim/cli), never here.
CREATE TABLE IF NOT EXISTS codes (
	code TEXT PRIMARY KEY,
	generated_on TEXT NOT NULL CHECK (generated_on GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
	token_hash TEXT
) WITHOUT ROWID;

-- One code per token, and a token that already has a code is never given another one.
CREATE UNIQUE INDEX IF NOT EXISTS codes_token ON codes (token_hash) WHERE token_hash IS NOT NULL;
CREATE INDEX IF NOT EXISTS codes_free ON codes (generated_on) WHERE token_hash IS NULL;

CREATE TABLE IF NOT EXISTS tokens (
	hash TEXT PRIMARY KEY,
	expires_at INTEGER NOT NULL
) WITHOUT ROWID;
