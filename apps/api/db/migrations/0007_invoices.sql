-- +goose Up

-- Invoices track money owed to (customer invoices, type='invoice') or by (vendor
-- bills, type='bill') the book. Schema mirrors GnuCash's invoices table with a
-- book_guid column added for multi-tenancy.
CREATE TABLE invoices (
    guid           CHAR(32)     PRIMARY KEY,
    book_guid      CHAR(32)     NOT NULL REFERENCES books(guid),
    id             TEXT         NOT NULL DEFAULT '',   -- display number e.g. INV-0001
    type           TEXT         NOT NULL,              -- 'invoice' or 'bill'
    owner_guid     CHAR(32)     NOT NULL,              -- customer or vendor guid
    date_opened    DATE         NOT NULL,
    date_posted    DATE,                               -- NULL = draft
    date_due       DATE,
    notes          TEXT         NOT NULL DEFAULT '',
    active         BOOLEAN      NOT NULL DEFAULT TRUE,
    currency_guid  CHAR(32)     NOT NULL REFERENCES commodities(guid),
    post_txn_guid  CHAR(32),                          -- transaction created when posted
    post_acc_guid  CHAR(32),                          -- A/R or A/P account used
    terms_guid     CHAR(32),
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX invoices_book_idx  ON invoices(book_guid);
CREATE INDEX invoices_owner_idx ON invoices(owner_guid);
CREATE INDEX invoices_type_idx  ON invoices(book_guid, type);

-- Line items for invoices and bills. Each entry contributes qty×price to the
-- invoice total and maps to an income (invoice) or expense (bill) account split
-- when the invoice is posted.
CREATE TABLE entries (
    guid           CHAR(32)     PRIMARY KEY,
    invoice_guid   CHAR(32)     NOT NULL REFERENCES invoices(guid) ON DELETE CASCADE,
    date           DATE         NOT NULL,
    description    TEXT         NOT NULL DEFAULT '',
    action         TEXT         NOT NULL DEFAULT '',
    notes          TEXT         NOT NULL DEFAULT '',
    quantity_num   BIGINT       NOT NULL DEFAULT 100,
    quantity_denom BIGINT       NOT NULL DEFAULT 100,
    account_guid   CHAR(32)     NOT NULL REFERENCES accounts(guid),
    price_num      BIGINT       NOT NULL DEFAULT 0,
    price_denom    BIGINT       NOT NULL DEFAULT 100,
    taxable        BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX entries_invoice_idx ON entries(invoice_guid);

-- +goose Down
DROP TABLE IF EXISTS entries;
DROP TABLE IF EXISTS invoices;
