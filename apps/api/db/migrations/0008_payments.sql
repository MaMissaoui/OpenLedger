-- +goose Up

-- Track invoice/bill payment. paid_at set when a payment transaction is posted;
-- paid_txn_guid links to the transaction that cleared the A/R or A/P balance.
ALTER TABLE invoices
  ADD COLUMN paid_at      TIMESTAMPTZ,
  ADD COLUMN paid_txn_guid CHAR(32);

-- +goose Down
ALTER TABLE invoices
  DROP COLUMN IF EXISTS paid_at,
  DROP COLUMN IF EXISTS paid_txn_guid;
