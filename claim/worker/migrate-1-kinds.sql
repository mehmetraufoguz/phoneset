-- Run ONCE on a database made with the first schema (codes: code, generated_on, token_hash; tokens: hash, expires_at):
--   wrangler d1 execute phoneset-claim --remote --file=migrate-1-kinds.sql
-- Existing rows become kind "app", store "apple"; valid_until is generated_on + 27 days, the day the page
-- used to show. A fresh database does not need this file, schema.sql already has the columns.
ALTER TABLE codes ADD COLUMN kind TEXT NOT NULL DEFAULT 'app' CHECK (kind IN ('app', 'video'));
ALTER TABLE codes ADD COLUMN store TEXT NOT NULL DEFAULT 'apple' CHECK (store IN ('apple', 'google'));
ALTER TABLE codes ADD COLUMN valid_until TEXT NOT NULL DEFAULT '1970-01-01';
UPDATE codes SET valid_until = date(generated_on, '+27 days');
ALTER TABLE tokens ADD COLUMN kinds TEXT NOT NULL DEFAULT 'app' CHECK (kinds IN ('app', 'video', 'app,video'));
ALTER TABLE tokens ADD COLUMN store TEXT NOT NULL DEFAULT 'apple' CHECK (store IN ('apple', 'google'));
DROP INDEX IF EXISTS codes_token;
DROP INDEX IF EXISTS codes_free;
CREATE UNIQUE INDEX IF NOT EXISTS codes_token_kind ON codes (token_hash, kind) WHERE token_hash IS NOT NULL;
CREATE INDEX IF NOT EXISTS codes_free ON codes (kind, store, valid_until) WHERE token_hash IS NULL;
