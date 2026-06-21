-- +goose Up
-- Book-level preferences (web-only table, not GnuCash-mirrored).
-- default_commodity_guid is nullable: an unset preference means "no default
-- selected yet" and the UI shows a placeholder.
CREATE TABLE IF NOT EXISTS book_preferences (
    book_guid               TEXT NOT NULL PRIMARY KEY REFERENCES books(guid) ON DELETE CASCADE,
    default_commodity_guid  TEXT REFERENCES commodities(guid) ON DELETE SET NULL,
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS book_preferences;
