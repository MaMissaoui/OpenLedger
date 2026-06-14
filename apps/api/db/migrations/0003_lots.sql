-- +goose Up
-- +goose StatementBegin

-- ---------------------------------------------------------------------------
-- Lots group the splits of a security account that net toward zero, so cost
-- basis and realized capital gains can be tracked across buys and sells. The
-- columns mirror GnuCash's `lots` table (guid, account_guid, is_closed) so the
-- table round-trips through import/export. Each split already carries an
-- optional lot_guid (splits.lot_guid) tying it to its lot.
-- ---------------------------------------------------------------------------
CREATE TABLE lots (
    guid         CHAR(32) PRIMARY KEY,
    account_guid CHAR(32) NOT NULL,
    is_closed    INTEGER  NOT NULL DEFAULT 0
);
CREATE INDEX idx_lots_account ON lots(account_guid);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS lots;
-- +goose StatementEnd
