-- +goose Up
-- +goose StatementBegin

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- GnuCash-mirrored core tables. Column names and the 32-char hex GUID primary
-- keys follow GnuCash's SQL backend so import/export stays a direct mapping.
-- Monetary amounts are stored as exact numerator/denominator integer pairs.
-- ---------------------------------------------------------------------------

CREATE TABLE commodities (
    guid        CHAR(32) PRIMARY KEY,
    namespace   VARCHAR(2048) NOT NULL,
    mnemonic    VARCHAR(2048) NOT NULL,
    fullname    VARCHAR(2048),
    cusip       VARCHAR(2048),
    fraction    INTEGER       NOT NULL,
    quote_flag  INTEGER       NOT NULL DEFAULT 0,
    quote_source VARCHAR(2048),
    quote_tz    VARCHAR(2048)
);

CREATE TABLE books (
    guid                CHAR(32) PRIMARY KEY,
    root_account_guid   CHAR(32) NOT NULL,
    root_template_guid  CHAR(32) NOT NULL
);

CREATE TABLE accounts (
    guid           CHAR(32) PRIMARY KEY,
    name           VARCHAR(2048) NOT NULL,
    account_type   VARCHAR(2048) NOT NULL,
    commodity_guid CHAR(32) REFERENCES commodities(guid),
    commodity_scu  INTEGER NOT NULL DEFAULT 0,
    non_std_scu    INTEGER NOT NULL DEFAULT 0,
    parent_guid    CHAR(32) REFERENCES accounts(guid),
    code           VARCHAR(2048),
    description    VARCHAR(2048),
    hidden         INTEGER NOT NULL DEFAULT 0,
    placeholder    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_accounts_parent ON accounts(parent_guid);

CREATE TABLE transactions (
    guid          CHAR(32) PRIMARY KEY,
    currency_guid CHAR(32) NOT NULL REFERENCES commodities(guid),
    num           VARCHAR(2048) NOT NULL DEFAULT '',
    post_date     TIMESTAMPTZ,
    enter_date    TIMESTAMPTZ,
    description   VARCHAR(2048)
);
CREATE INDEX idx_transactions_post_date ON transactions(post_date);

CREATE TABLE splits (
    guid            CHAR(32) PRIMARY KEY,
    tx_guid         CHAR(32) NOT NULL REFERENCES transactions(guid) ON DELETE CASCADE,
    account_guid    CHAR(32) NOT NULL REFERENCES accounts(guid),
    memo            VARCHAR(2048) NOT NULL DEFAULT '',
    action          VARCHAR(2048) NOT NULL DEFAULT '',
    reconcile_state CHAR(1) NOT NULL DEFAULT 'n',
    reconcile_date  TIMESTAMPTZ,
    value_num       BIGINT NOT NULL,
    value_denom     BIGINT NOT NULL CHECK (value_denom <> 0),
    quantity_num    BIGINT NOT NULL,
    quantity_denom  BIGINT NOT NULL CHECK (quantity_denom <> 0),
    lot_guid        CHAR(32)
);
CREATE INDEX idx_splits_tx ON splits(tx_guid);
CREATE INDEX idx_splits_account ON splits(account_guid);

CREATE TABLE prices (
    guid          CHAR(32) PRIMARY KEY,
    commodity_guid CHAR(32) NOT NULL REFERENCES commodities(guid),
    currency_guid CHAR(32) NOT NULL REFERENCES commodities(guid),
    date          TIMESTAMPTZ NOT NULL,
    source        VARCHAR(2048),
    type          VARCHAR(2048),
    value_num     BIGINT NOT NULL,
    value_denom   BIGINT NOT NULL CHECK (value_denom <> 0)
);

-- Key/value metadata attached to any object (GnuCash's KVP "slots"). Preserved
-- verbatim on import so re-export is lossless even for fields the UI ignores.
CREATE TABLE slots (
    id          BIGSERIAL PRIMARY KEY,
    obj_guid    CHAR(32) NOT NULL,
    name        VARCHAR(4096) NOT NULL,
    slot_type   INTEGER NOT NULL,
    int64_val   BIGINT,
    string_val  VARCHAR(4096),
    double_val  DOUBLE PRECISION,
    timespec_val TIMESTAMPTZ,
    guid_val    CHAR(32),
    numeric_val_num   BIGINT,
    numeric_val_denom BIGINT
);
CREATE INDEX idx_slots_obj ON slots(obj_guid);

-- Per-table schema version tracking, as GnuCash uses.
CREATE TABLE versions (
    table_name  VARCHAR(50) PRIMARY KEY,
    table_version INTEGER NOT NULL
);

-- ---------------------------------------------------------------------------
-- Web-only tables (not part of GnuCash): multi-user, auth, audit, jobs.
-- ---------------------------------------------------------------------------

CREATE TABLE organizations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES organizations(id),
    email         TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_users_email ON users(lower(email));

-- A user's role on a particular book (ledger).
CREATE TABLE memberships (
    user_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_guid CHAR(32) NOT NULL REFERENCES books(guid) ON DELETE CASCADE,
    role      TEXT NOT NULL CHECK (role IN ('owner','admin','editor','viewer')),
    PRIMARY KEY (user_id, book_guid)
);

CREATE TABLE refresh_tokens (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);

-- Append-only record of every posting/edit for traceability.
CREATE TABLE audit_log (
    id            BIGSERIAL PRIMARY KEY,
    actor_user_id UUID REFERENCES users(id),
    book_guid     CHAR(32),
    action        TEXT NOT NULL,
    entity_type   TEXT NOT NULL,
    entity_guid   TEXT,
    detail        JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_log_book ON audit_log(book_guid);

CREATE TABLE import_jobs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    book_guid   CHAR(32),
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','succeeded','failed')),
    filename    TEXT,
    error       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS import_jobs;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS organizations;
DROP TABLE IF EXISTS versions;
DROP TABLE IF EXISTS slots;
DROP TABLE IF EXISTS prices;
DROP TABLE IF EXISTS splits;
DROP TABLE IF EXISTS transactions;
DROP TABLE IF EXISTS accounts;
DROP TABLE IF EXISTS books;
DROP TABLE IF EXISTS commodities;
-- +goose StatementEnd
