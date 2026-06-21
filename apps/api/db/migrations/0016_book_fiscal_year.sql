-- +goose Up
ALTER TABLE book_preferences
    ADD COLUMN fiscal_year_start SMALLINT NOT NULL DEFAULT 1
        CHECK (fiscal_year_start BETWEEN 1 AND 12);

-- +goose Down
ALTER TABLE book_preferences DROP COLUMN IF EXISTS fiscal_year_start;
