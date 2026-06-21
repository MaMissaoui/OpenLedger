-- +goose Up

-- Employees (people you reimburse via expense vouchers). Schema mirrors
-- GnuCash's employees table; book_guid is added for multi-tenancy since GnuCash
-- is single-book. Expense vouchers themselves are deferred; this is the
-- entity-of-record only.
CREATE TABLE employees (
    guid           CHAR(32)     PRIMARY KEY,
    book_guid      CHAR(32)     NOT NULL REFERENCES books(guid),
    name           TEXT         NOT NULL,
    username       TEXT         NOT NULL DEFAULT '',
    id             TEXT         NOT NULL DEFAULT '',  -- display number, e.g. EMP-0001
    notes          TEXT         NOT NULL DEFAULT '',
    active         BOOLEAN      NOT NULL DEFAULT TRUE,
    currency_guid  CHAR(32)     NOT NULL REFERENCES commodities(guid),
    addr_name      TEXT         NOT NULL DEFAULT '',
    addr_addr1     TEXT         NOT NULL DEFAULT '',
    addr_addr2     TEXT         NOT NULL DEFAULT '',
    addr_phone     TEXT         NOT NULL DEFAULT '',
    addr_email     TEXT         NOT NULL DEFAULT '',
    -- default hourly billing rate as a rational pair (GnuCash schema)
    rate_num       BIGINT       NOT NULL DEFAULT 0,
    rate_denom     BIGINT       NOT NULL DEFAULT 1,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX employees_book_idx ON employees(book_guid);
CREATE UNIQUE INDEX employees_book_id_idx ON employees(book_guid, id) WHERE id <> '';

-- Jobs group invoices/bills under a single customer or vendor. owner_type is
-- 'customer' or 'vendor'; owner_guid points at the matching row (polymorphic,
-- so no FK). Mirrors GnuCash's jobs table.
CREATE TABLE jobs (
    guid           CHAR(32)     PRIMARY KEY,
    book_guid      CHAR(32)     NOT NULL REFERENCES books(guid),
    name           TEXT         NOT NULL,
    id             TEXT         NOT NULL DEFAULT '',  -- display number, e.g. JOB-0001
    reference      TEXT         NOT NULL DEFAULT '',
    active         BOOLEAN      NOT NULL DEFAULT TRUE,
    owner_type     TEXT         NOT NULL,             -- 'customer' | 'vendor'
    owner_guid     CHAR(32)     NOT NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX jobs_book_idx ON jobs(book_guid);
CREATE INDEX jobs_owner_idx ON jobs(owner_guid);
CREATE UNIQUE INDEX jobs_book_id_idx ON jobs(book_guid, id) WHERE id <> '';

-- +goose Down
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS employees;
