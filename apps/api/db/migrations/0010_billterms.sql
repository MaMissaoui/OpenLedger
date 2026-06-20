-- +goose Up

-- Payment terms for invoices and bills, mirroring GnuCash's billterms table with
-- a book_guid column added for multi-tenancy. A term computes an invoice's due
-- date from its post date: type='days' is net duedays; type='proximo' is due on
-- duedays-of-the-month, rolling past cutoff. discount_num/discount_denom hold the
-- early-payment discount as an exact fraction (e.g. 2/100 = 2%).
CREATE TABLE billterms (
    guid           CHAR(32)     PRIMARY KEY,
    book_guid      CHAR(32)     NOT NULL REFERENCES books(guid),
    name           TEXT         NOT NULL DEFAULT '',
    description    TEXT         NOT NULL DEFAULT '',
    refcount       BIGINT       NOT NULL DEFAULT 0,
    invisible      BOOLEAN      NOT NULL DEFAULT FALSE,
    parent         CHAR(32),
    type           TEXT         NOT NULL,              -- 'days' or 'proximo'
    duedays        INTEGER      NOT NULL DEFAULT 0,
    discountdays   INTEGER      NOT NULL DEFAULT 0,
    discount_num   BIGINT       NOT NULL DEFAULT 0,
    discount_denom BIGINT       NOT NULL DEFAULT 1,
    cutoff         INTEGER      NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX billterms_book_idx ON billterms(book_guid);

-- +goose Down
DROP TABLE IF EXISTS billterms;
