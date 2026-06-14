-- +goose Up
-- +goose StatementBegin

-- ---------------------------------------------------------------------------
-- Budgets define per-account planned amounts for a sequence of equal-length
-- periods (monthly, quarterly, or yearly). budget_amounts ties a specific
-- dollar amount to (budget, account, period_num) tuples, mirroring GnuCash's
-- budget/budget_amounts tables.
-- ---------------------------------------------------------------------------
CREATE TABLE budgets (
    guid        CHAR(32) PRIMARY KEY,
    book_guid   CHAR(32) NOT NULL,
    name        TEXT     NOT NULL,
    description TEXT     NOT NULL DEFAULT '',
    period_type TEXT     NOT NULL,  -- monthly|quarterly|yearly
    num_periods INTEGER  NOT NULL DEFAULT 12,
    start_date  TEXT     NOT NULL   -- YYYY-MM-DD (first day of period 0)
);

CREATE INDEX idx_budgets_book ON budgets(book_guid);

CREATE TABLE budget_amounts (
    id           SERIAL PRIMARY KEY,
    budget_guid  CHAR(32) NOT NULL REFERENCES budgets(guid) ON DELETE CASCADE,
    account_guid CHAR(32) NOT NULL,
    period_num   INTEGER  NOT NULL,
    value_num    BIGINT   NOT NULL,
    value_denom  BIGINT   NOT NULL,
    UNIQUE (budget_guid, account_guid, period_num)
);

CREATE INDEX idx_budget_amounts_budget ON budget_amounts(budget_guid);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS budget_amounts;
DROP TABLE IF EXISTS budgets;
-- +goose StatementEnd
