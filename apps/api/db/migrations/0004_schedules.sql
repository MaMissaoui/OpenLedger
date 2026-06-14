-- +goose Up
-- +goose StatementBegin

-- ---------------------------------------------------------------------------
-- Scheduled transactions define recurring postings (rent, salary, etc.).
-- Each row describes the recurrence rule; scheduled_splits holds the template
-- split amounts. When a scheduled transaction is due, PostingService creates a
-- real transaction from these templates.
-- ---------------------------------------------------------------------------
CREATE TABLE scheduled_transactions (
    guid          CHAR(32) PRIMARY KEY,
    book_guid     CHAR(32) NOT NULL,
    name          TEXT     NOT NULL,
    description   TEXT     NOT NULL DEFAULT '',
    enabled       INTEGER  NOT NULL DEFAULT 1,
    currency_guid CHAR(32) NOT NULL,
    period        TEXT     NOT NULL,  -- once|daily|weekly|monthly|yearly
    every         INTEGER  NOT NULL DEFAULT 1,
    start_date    TEXT     NOT NULL,  -- YYYY-MM-DD
    end_date      TEXT,               -- YYYY-MM-DD, NULL = no end
    last_posted   TEXT                -- YYYY-MM-DD, NULL = never posted
);

CREATE INDEX idx_schedtx_book ON scheduled_transactions(book_guid);

CREATE TABLE scheduled_splits (
    guid         CHAR(32) PRIMARY KEY,
    schedtx_guid CHAR(32) NOT NULL REFERENCES scheduled_transactions(guid) ON DELETE CASCADE,
    account_guid CHAR(32) NOT NULL,
    memo         TEXT     NOT NULL DEFAULT '',
    value_num    BIGINT   NOT NULL,
    value_denom  BIGINT   NOT NULL
);

CREATE INDEX idx_schedsplit_tx ON scheduled_splits(schedtx_guid);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS scheduled_splits;
DROP TABLE IF EXISTS scheduled_transactions;
-- +goose StatementEnd
