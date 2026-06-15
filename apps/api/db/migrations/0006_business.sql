-- +goose Up

-- Customers (accounts-receivable contacts). Schema mirrors GnuCash's customers
-- table; book_guid is added for multi-tenancy since GnuCash is single-book.
CREATE TABLE customers (
    guid           CHAR(32)     PRIMARY KEY,
    book_guid      CHAR(32)     NOT NULL REFERENCES books(guid),
    name           TEXT         NOT NULL,
    id             TEXT         NOT NULL DEFAULT '',  -- display number, e.g. CUST-0001
    notes          TEXT         NOT NULL DEFAULT '',
    active         BOOLEAN      NOT NULL DEFAULT TRUE,
    currency_guid  CHAR(32)     NOT NULL REFERENCES commodities(guid),
    -- billing address
    addr_name      TEXT         NOT NULL DEFAULT '',
    addr_addr1     TEXT         NOT NULL DEFAULT '',
    addr_addr2     TEXT         NOT NULL DEFAULT '',
    addr_phone     TEXT         NOT NULL DEFAULT '',
    addr_email     TEXT         NOT NULL DEFAULT '',
    -- credit limit as rational pair (GnuCash schema)
    credit_num     BIGINT       NOT NULL DEFAULT 0,
    credit_denom   BIGINT       NOT NULL DEFAULT 100,
    -- deferred to Phase 4d
    terms_guid     CHAR(32),
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX customers_book_idx ON customers(book_guid);
CREATE UNIQUE INDEX customers_book_id_idx ON customers(book_guid, id) WHERE id <> '';

-- Vendors (accounts-payable contacts).
CREATE TABLE vendors (
    guid           CHAR(32)     PRIMARY KEY,
    book_guid      CHAR(32)     NOT NULL REFERENCES books(guid),
    name           TEXT         NOT NULL,
    id             TEXT         NOT NULL DEFAULT '',
    notes          TEXT         NOT NULL DEFAULT '',
    active         BOOLEAN      NOT NULL DEFAULT TRUE,
    currency_guid  CHAR(32)     NOT NULL REFERENCES commodities(guid),
    addr_name      TEXT         NOT NULL DEFAULT '',
    addr_addr1     TEXT         NOT NULL DEFAULT '',
    addr_addr2     TEXT         NOT NULL DEFAULT '',
    addr_phone     TEXT         NOT NULL DEFAULT '',
    addr_email     TEXT         NOT NULL DEFAULT '',
    terms_guid     CHAR(32),
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX vendors_book_idx ON vendors(book_guid);
CREATE UNIQUE INDEX vendors_book_id_idx ON vendors(book_guid, id) WHERE id <> '';

-- +goose Down
DROP TABLE IF EXISTS vendors;
DROP TABLE IF EXISTS customers;
