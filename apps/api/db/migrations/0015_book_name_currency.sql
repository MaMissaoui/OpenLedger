-- +goose Up
-- Add a human-readable name and home currency to each book (company/ledger).
-- name defaults to '' so existing rows remain valid; apps should prompt users
-- to set a name after upgrading.
ALTER TABLE books ADD COLUMN name TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN currency_guid CHAR(32) REFERENCES commodities(guid) ON DELETE SET NULL;

-- Give the organizations table a user-visible display name (separate from the
-- ldap_uid stored in the name column at provisioning time).
ALTER TABLE organizations ADD COLUMN display_name TEXT;

-- +goose Down
ALTER TABLE organizations DROP COLUMN IF EXISTS display_name;
ALTER TABLE books DROP COLUMN IF EXISTS currency_guid;
ALTER TABLE books DROP COLUMN IF EXISTS name;
