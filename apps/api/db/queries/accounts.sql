-- Sample sqlc queries. Run `sqlc generate` to produce type-safe Go in
-- internal/infra/db. Add query files per aggregate as the API grows.

-- name: GetAccount :one
SELECT guid, name, account_type, commodity_guid, parent_guid, code, description, hidden, placeholder
FROM accounts
WHERE guid = $1;

-- name: ListAccountsForBook :many
SELECT a.guid, a.name, a.account_type, a.commodity_guid, a.parent_guid, a.code, a.description
FROM accounts a
ORDER BY a.code, a.name;

-- name: CreateAccount :one
INSERT INTO accounts (guid, name, account_type, commodity_guid, commodity_scu, parent_guid, code, description, hidden, placeholder)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING guid;

-- name: ListSplitsForAccount :many
SELECT s.guid, s.tx_guid, s.account_guid, s.memo, s.reconcile_state,
       s.value_num, s.value_denom, s.quantity_num, s.quantity_denom,
       t.post_date, t.description
FROM splits s
JOIN transactions t ON t.guid = s.tx_guid
WHERE s.account_guid = $1
ORDER BY t.post_date, t.enter_date
LIMIT $2 OFFSET $3;
