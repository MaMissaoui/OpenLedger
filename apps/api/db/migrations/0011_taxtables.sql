-- +goose Up

-- Tax tables mirror GnuCash's taxtables/taxtable_entries. A table is a named set
-- of entries; each entry charges either a percentage of the taxed base or a flat
-- value, posting to one tax account (e.g. a VAT/sales-tax liability). book_guid is
-- added for multi-tenancy.
CREATE TABLE taxtables (
    guid       CHAR(32)     PRIMARY KEY,
    book_guid  CHAR(32)     NOT NULL REFERENCES books(guid),
    name       TEXT         NOT NULL DEFAULT '',
    refcount   BIGINT       NOT NULL DEFAULT 0,
    invisible  BOOLEAN      NOT NULL DEFAULT FALSE,
    parent     CHAR(32),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX taxtables_book_idx ON taxtables(book_guid);

CREATE TABLE taxtable_entries (
    guid          CHAR(32)    PRIMARY KEY,
    taxtable_guid CHAR(32)    NOT NULL REFERENCES taxtables(guid) ON DELETE CASCADE,
    account_guid  CHAR(32)    NOT NULL REFERENCES accounts(guid),
    amount_num    BIGINT      NOT NULL DEFAULT 0,
    amount_denom  BIGINT      NOT NULL DEFAULT 1,
    type          TEXT        NOT NULL,              -- 'percentage' or 'value'
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX taxtable_entries_table_idx ON taxtable_entries(taxtable_guid);

-- Invoice/bill line items may reference the tax table applied when taxable.
ALTER TABLE entries ADD COLUMN tax_table_guid CHAR(32);

-- +goose Down
ALTER TABLE entries DROP COLUMN IF EXISTS tax_table_guid;
DROP TABLE IF EXISTS taxtable_entries;
DROP TABLE IF EXISTS taxtables;
