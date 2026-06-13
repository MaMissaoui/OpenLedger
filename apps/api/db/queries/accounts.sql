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

-- name: AccountExists :one
SELECT EXISTS (SELECT 1 FROM accounts WHERE guid = $1);

-- name: ListAccountRegister :many
-- One page of an account's register with the running balance (window sum of
-- quantity_num; exact because every split for an account shares the account
-- commodity's fraction as its denominator) and the full row count.
SELECT
    s.guid, s.tx_guid, s.memo, s.reconcile_state,
    s.value_num, s.value_denom, s.quantity_num, s.quantity_denom,
    t.post_date, t.description,
    SUM(s.quantity_num) OVER (
        ORDER BY t.post_date, t.enter_date, s.guid
        ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
    ) AS running_num,
    COUNT(*) OVER () AS total
FROM splits s
JOIN transactions t ON t.guid = s.tx_guid
WHERE s.account_guid = $1
ORDER BY t.post_date, t.enter_date, s.guid
LIMIT $2 OFFSET $3;
