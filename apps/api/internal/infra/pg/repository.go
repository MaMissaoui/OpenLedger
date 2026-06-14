// Package pg provides PostgreSQL-backed implementations of the app repository
// ports, using pgx/v5.
package pg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// Repository implements the app repository ports over a pgx connection pool.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository wraps a pgx pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// InsertTransaction writes the transaction, its splits, and an audit_log row in
// a single DB transaction. The caller (PostingService) is responsible for
// having validated the balance invariant before this is reached.
//
// To stay faithful to GnuCash, each split's value is stored at the transaction
// currency's fraction and its quantity at the account commodity's fraction
// (e.g. denominator 100 for a currency with cents), rather than as a reduced
// rational. AtDenom errors if an amount is not exact in that smallest unit,
// which is the correct rejection for, say, $50.001.
func (r *Repository) InsertTransaction(ctx context.Context, t domain.Transaction, actor app.AuditActor) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO transactions (guid, currency_guid, num, post_date, enter_date, description)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			t.GUID, t.CurrencyGUID, t.Num, t.PostDate, t.EnterDate, t.Description,
		); err != nil {
			return fmt.Errorf("insert transaction: %w", err)
		}
		if err := r.insertSplits(ctx, tx, t); err != nil {
			return err
		}
		return r.writeTxnAudit(ctx, tx, t, actor, "post")
	})
}

// UpdateTransaction replaces a transaction's fields and splits in one DB
// transaction. The original enter_date is left untouched. It returns
// app.ErrTransactionNotFound if the GUID is unknown.
func (r *Repository) UpdateTransaction(ctx context.Context, t domain.Transaction, actor app.AuditActor) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx,
			`UPDATE transactions SET currency_guid = $2, num = $3, post_date = $4, description = $5
			 WHERE guid = $1`,
			t.GUID, t.CurrencyGUID, t.Num, t.PostDate, t.Description,
		)
		if err != nil {
			return fmt.Errorf("update transaction: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return app.ErrTransactionNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM splits WHERE tx_guid = $1`, t.GUID); err != nil {
			return fmt.Errorf("delete old splits: %w", err)
		}
		if err := r.insertSplits(ctx, tx, t); err != nil {
			return err
		}
		return r.writeTxnAudit(ctx, tx, t, actor, "edit")
	})
}

// DeleteTransaction removes a transaction and its splits in one DB transaction.
// It returns app.ErrTransactionNotFound if the GUID is unknown.
func (r *Repository) DeleteTransaction(ctx context.Context, guid string, actor app.AuditActor) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM splits WHERE tx_guid = $1`, guid); err != nil {
			return fmt.Errorf("delete splits: %w", err)
		}
		ct, err := tx.Exec(ctx, `DELETE FROM transactions WHERE guid = $1`, guid)
		if err != nil {
			return fmt.Errorf("delete transaction: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return app.ErrTransactionNotFound
		}
		detail, err := json.Marshal(map[string]any{"guid": guid})
		if err != nil {
			return fmt.Errorf("marshal audit detail: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO audit_log (actor_user_id, book_guid, action, entity_type, entity_guid, detail)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			nullable(actor.UserID), nullable(actor.BookGUID), "delete", "transaction", guid, detail,
		); err != nil {
			return fmt.Errorf("insert audit log: %w", err)
		}
		return nil
	})
}

// TransactionAccountGUIDs returns the distinct accounts a transaction's splits
// post to, or app.ErrTransactionNotFound when the transaction has no splits
// (i.e. does not exist).
func (r *Repository) TransactionAccountGUIDs(ctx context.Context, guid string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT account_guid FROM splits WHERE tx_guid = $1`, guid)
	if err != nil {
		return nil, fmt.Errorf("transaction accounts: %w", err)
	}
	defer rows.Close()

	var guids []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, fmt.Errorf("scan account guid: %w", err)
		}
		guids = append(guids, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(guids) == 0 {
		return nil, app.ErrTransactionNotFound
	}
	return guids, nil
}

// insertSplits writes all of a transaction's splits, converting each amount to
// its commodity fraction (value in the transaction currency, quantity in the
// account commodity). It assumes the parent transaction row already exists.
func (r *Repository) insertSplits(ctx context.Context, tx pgx.Tx, t domain.Transaction) error {
	var currencyFraction int64
	if err := tx.QueryRow(ctx,
		`SELECT fraction FROM commodities WHERE guid = $1`, t.CurrencyGUID,
	).Scan(&currencyFraction); err != nil {
		return fmt.Errorf("lookup currency fraction for %s: %w", t.CurrencyGUID, err)
	}

	accountFractions, err := r.accountFractions(ctx, tx, t.Splits)
	if err != nil {
		return err
	}

	for _, s := range t.Splits {
		valueNum, err := s.Value.AtDenom(currencyFraction)
		if err != nil {
			return fmt.Errorf("split %s value: %w", s.GUID, err)
		}
		accFraction, ok := accountFractions[s.AccountGUID]
		if !ok {
			return fmt.Errorf("split %s: unknown account %s", s.GUID, s.AccountGUID)
		}
		qtyNum, err := s.Quantity.AtDenom(accFraction)
		if err != nil {
			return fmt.Errorf("split %s quantity: %w", s.GUID, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO splits
			   (guid, tx_guid, account_guid, memo, action, reconcile_state,
			    value_num, value_denom, quantity_num, quantity_denom, lot_guid)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			s.GUID, t.GUID, s.AccountGUID, s.Memo, s.Action, string(s.Reconcile),
			valueNum, currencyFraction, qtyNum, accFraction, nullable(s.LotGUID),
		); err != nil {
			return fmt.Errorf("insert split %s: %w", s.GUID, err)
		}
	}
	return nil
}

// writeTxnAudit appends an audit_log row describing a transaction write.
func (r *Repository) writeTxnAudit(ctx context.Context, tx pgx.Tx, t domain.Transaction, actor app.AuditActor, action string) error {
	detail, err := json.Marshal(map[string]any{
		"currency": t.CurrencyGUID,
		"splits":   len(t.Splits),
	})
	if err != nil {
		return fmt.Errorf("marshal audit detail: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO audit_log (actor_user_id, book_guid, action, entity_type, entity_guid, detail)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		nullable(actor.UserID), nullable(actor.BookGUID), action, "transaction", t.GUID, detail,
	); err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

// accountFractions returns the commodity fraction for each split's account.
func (r *Repository) accountFractions(ctx context.Context, tx pgx.Tx, splits []domain.Split) (map[string]int64, error) {
	guids := make([]string, 0, len(splits))
	for _, s := range splits {
		guids = append(guids, s.AccountGUID)
	}
	rows, err := tx.Query(ctx,
		`SELECT a.guid, c.fraction
		   FROM accounts a
		   JOIN commodities c ON c.guid = a.commodity_guid
		  WHERE a.guid = ANY($1)`, guids)
	if err != nil {
		return nil, fmt.Errorf("lookup account fractions: %w", err)
	}
	defer rows.Close()

	fractions := make(map[string]int64, len(splits))
	for rows.Next() {
		var guid string
		var fraction int64
		if err := rows.Scan(&guid, &fraction); err != nil {
			return nil, fmt.Errorf("scan account fraction: %w", err)
		}
		fractions[guid] = fraction
	}
	return fractions, rows.Err()
}

// InsertCommodity writes a commodity row.
func (r *Repository) InsertCommodity(ctx context.Context, c domain.Commodity) error {
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO commodities (guid, namespace, mnemonic, fullname, fraction)
		 VALUES ($1, $2, $3, $4, $5)`,
		c.GUID, c.Namespace, c.Mnemonic, nullable(c.Fullname), c.Fraction,
	); err != nil {
		return fmt.Errorf("insert commodity: %w", err)
	}
	return nil
}

// ListCommodities returns all commodities ordered by namespace then mnemonic.
func (r *Repository) ListCommodities(ctx context.Context) ([]domain.Commodity, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT guid, namespace, mnemonic, fullname, fraction
		 FROM commodities ORDER BY namespace, mnemonic`)
	if err != nil {
		return nil, fmt.Errorf("list commodities: %w", err)
	}
	defer rows.Close()

	var commodities []domain.Commodity
	for rows.Next() {
		var (
			c        domain.Commodity
			fullname *string
		)
		if err := rows.Scan(&c.GUID, &c.Namespace, &c.Mnemonic, &fullname, &c.Fraction); err != nil {
			return nil, fmt.Errorf("scan commodity: %w", err)
		}
		if fullname != nil {
			c.Fullname = *fullname
		}
		commodities = append(commodities, c)
	}
	return commodities, rows.Err()
}

// InsertPrice writes a price quote. The value is stored as an exact
// numerator/denominator pair (a price is a ratio, so it keeps its own
// precision rather than a commodity fraction).
func (r *Repository) InsertPrice(ctx context.Context, p domain.Price) error {
	num, denom, err := p.Value.NumDenom()
	if err != nil {
		return fmt.Errorf("price value: %w", err)
	}
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO prices (guid, commodity_guid, currency_guid, date, source, type, value_num, value_denom)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		p.GUID, p.CommodityGUID, p.CurrencyGUID, p.Date,
		nullable(p.Source), nullable(p.Type), num, denom,
	); err != nil {
		return fmt.Errorf("insert price: %w", err)
	}
	return nil
}

// ListPricesByCommodity returns a commodity's quotes, most recent first.
func (r *Repository) ListPricesByCommodity(ctx context.Context, commodityGUID string) ([]domain.Price, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT guid, commodity_guid, currency_guid, date, source, type, value_num, value_denom
		 FROM prices WHERE commodity_guid = $1 ORDER BY date DESC`, commodityGUID)
	if err != nil {
		return nil, fmt.Errorf("list prices: %w", err)
	}
	defer rows.Close()

	var prices []domain.Price
	for rows.Next() {
		var (
			p                 domain.Price
			source, priceType *string
			valueNum, valDen  int64
		)
		if err := rows.Scan(
			&p.GUID, &p.CommodityGUID, &p.CurrencyGUID, &p.Date,
			&source, &priceType, &valueNum, &valDen,
		); err != nil {
			return nil, fmt.Errorf("scan price: %w", err)
		}
		if source != nil {
			p.Source = *source
		}
		if priceType != nil {
			p.Type = *priceType
		}
		if p.Value, err = domain.FromNumDenom(valueNum, valDen); err != nil {
			return nil, fmt.Errorf("price %s value: %w", p.GUID, err)
		}
		prices = append(prices, p)
	}
	return prices, rows.Err()
}

// InsertBook writes the root account, the template root account, and the book
// row in a single DB transaction so a book never exists without its roots. When
// ownerUserID is non-empty it also inserts an owner membership.
func (r *Repository) InsertBook(ctx context.Context, b domain.Book, root, templateRoot domain.Account, ownerUserID string) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		for _, a := range []domain.Account{root, templateRoot} {
			if err := insertAccount(ctx, tx, a); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO books (guid, root_account_guid, root_template_guid)
			 VALUES ($1, $2, $3)`,
			b.GUID, b.RootAccountGUID, b.RootTemplateGUID,
		); err != nil {
			return fmt.Errorf("insert book: %w", err)
		}
		if ownerUserID != "" {
			if _, err := tx.Exec(ctx,
				`INSERT INTO memberships (user_id, book_guid, role) VALUES ($1, $2, 'owner')`,
				ownerUserID, b.GUID,
			); err != nil {
				return fmt.Errorf("insert owner membership: %w", err)
			}
		}
		return nil
	})
}

// ListBooksForUser returns the books a user has a membership on, newest first.
func (r *Repository) ListBooksForUser(ctx context.Context, userID string) ([]domain.Book, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT b.guid, b.root_account_guid, b.root_template_guid
		   FROM books b
		   JOIN memberships m ON m.book_guid = b.guid
		  WHERE m.user_id = $1
		  ORDER BY b.guid`, userID)
	if err != nil {
		return nil, fmt.Errorf("list books for user: %w", err)
	}
	defer rows.Close()

	var books []domain.Book
	for rows.Next() {
		var b domain.Book
		if err := rows.Scan(&b.GUID, &b.RootAccountGUID, &b.RootTemplateGUID); err != nil {
			return nil, fmt.Errorf("scan book: %w", err)
		}
		books = append(books, b)
	}
	return books, rows.Err()
}

// InsertAccount writes a single account row.
func (r *Repository) InsertAccount(ctx context.Context, a domain.Account) error {
	return insertAccount(ctx, r.pool, a)
}

// querier is the subset of pgx used for inserts, satisfied by both *pgxpool.Pool
// and pgx.Tx, so insertAccount works standalone or inside a transaction.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func insertAccount(ctx context.Context, q querier, a domain.Account) error {
	if _, err := q.Exec(ctx,
		`INSERT INTO accounts
		   (guid, name, account_type, commodity_guid, commodity_scu,
		    parent_guid, code, description, hidden, placeholder)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		a.GUID, a.Name, string(a.Type), nullable(a.CommodityGUID), 0,
		nullable(a.ParentGUID), a.Code, a.Description,
		boolToInt(a.Hidden), boolToInt(a.Placeholder),
	); err != nil {
		return fmt.Errorf("insert account %s: %w", a.GUID, err)
	}
	return nil
}

// BookRootAccount returns a book's root account GUID, or app.ErrBookNotFound.
func (r *Repository) BookRootAccount(ctx context.Context, bookGUID string) (string, error) {
	var root string
	err := r.pool.QueryRow(ctx,
		`SELECT root_account_guid FROM books WHERE guid = $1`, bookGUID,
	).Scan(&root)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", app.ErrBookNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lookup book root: %w", err)
	}
	return root, nil
}

// ListAccountsUnderRoot returns every descendant of rootGUID (root excluded)
// via a recursive walk of the parent_guid tree, ordered by code then name.
func (r *Repository) ListAccountsUnderRoot(ctx context.Context, rootGUID string) ([]app.AccountWithBalance, error) {
	const sql = `
WITH RECURSIVE tree AS (
    SELECT guid, name, account_type, commodity_guid, parent_guid, code, description, hidden, placeholder
    FROM accounts WHERE parent_guid = $1
    UNION ALL
    SELECT a.guid, a.name, a.account_type, a.commodity_guid, a.parent_guid, a.code, a.description, a.hidden, a.placeholder
    FROM accounts a JOIN tree t ON a.parent_guid = t.guid
)
SELECT t.guid, t.name, t.account_type, t.commodity_guid, t.parent_guid, t.code, t.description, t.hidden, t.placeholder,
       COALESCE(SUM(s.quantity_num), 0) AS balance_num,
       COALESCE(c.fraction, 1)          AS balance_denom
FROM tree t
LEFT JOIN commodities c ON c.guid = t.commodity_guid
LEFT JOIN splits s      ON s.account_guid = t.guid
GROUP BY t.guid, t.name, t.account_type, t.commodity_guid, t.parent_guid, t.code, t.description, t.hidden, t.placeholder, c.fraction
ORDER BY t.code, t.name`

	rows, err := r.pool.Query(ctx, sql, rootGUID)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()
	return scanAccountBalances(rows)
}

// AccountBalances returns every descendant of rootGUID with the raw signed sum
// of its splits' quantities, restricted to transactions whose post_date falls
// within [from, to] (a nil bound is open on that side). It backs the financial
// reports; balances are in each account's own commodity.
func (r *Repository) AccountBalances(ctx context.Context, rootGUID string, from, to *time.Time) ([]app.AccountWithBalance, error) {
	const sql = `
WITH RECURSIVE tree AS (
    SELECT guid, name, account_type, commodity_guid, parent_guid, code, description, hidden, placeholder
    FROM accounts WHERE parent_guid = $1
    UNION ALL
    SELECT a.guid, a.name, a.account_type, a.commodity_guid, a.parent_guid, a.code, a.description, a.hidden, a.placeholder
    FROM accounts a JOIN tree t ON a.parent_guid = t.guid
)
SELECT t.guid, t.name, t.account_type, t.commodity_guid, t.parent_guid, t.code, t.description, t.hidden, t.placeholder,
       COALESCE(SUM(CASE WHEN tx.guid IS NOT NULL THEN s.quantity_num END), 0) AS balance_num,
       COALESCE(c.fraction, 1)                                                  AS balance_denom
FROM tree t
LEFT JOIN commodities c   ON c.guid = t.commodity_guid
LEFT JOIN splits s        ON s.account_guid = t.guid
LEFT JOIN transactions tx ON tx.guid = s.tx_guid
    AND ($2::timestamptz IS NULL OR tx.post_date >= $2)
    AND ($3::timestamptz IS NULL OR tx.post_date <= $3)
GROUP BY t.guid, t.name, t.account_type, t.commodity_guid, t.parent_guid, t.code, t.description, t.hidden, t.placeholder, c.fraction
ORDER BY t.code, t.name`

	rows, err := r.pool.Query(ctx, sql, rootGUID, from, to)
	if err != nil {
		return nil, fmt.Errorf("account balances: %w", err)
	}
	defer rows.Close()
	return scanAccountBalances(rows)
}

// scanAccountBalances reads rows shaped as (account columns…, balance_num,
// balance_denom) into app.AccountWithBalance values.
func scanAccountBalances(rows pgx.Rows) ([]app.AccountWithBalance, error) {
	var accounts []app.AccountWithBalance
	for rows.Next() {
		var (
			a                        domain.Account
			accountType              string
			commodity, parent        *string
			hidden, placeholder      int
			balanceNum, balanceDenom int64
		)
		if err := rows.Scan(
			&a.GUID, &a.Name, &accountType, &commodity, &parent,
			&a.Code, &a.Description, &hidden, &placeholder,
			&balanceNum, &balanceDenom,
		); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		a.Type = domain.AccountType(accountType)
		if commodity != nil {
			a.CommodityGUID = *commodity
		}
		if parent != nil {
			a.ParentGUID = *parent
		}
		a.Hidden = hidden != 0
		a.Placeholder = placeholder != 0

		balance, err := domain.FromNumDenom(balanceNum, balanceDenom)
		if err != nil {
			return nil, fmt.Errorf("account %s balance: %w", a.GUID, err)
		}
		accounts = append(accounts, app.AccountWithBalance{
			Account:      a,
			Balance:      balance,
			BalanceScale: balanceDenom,
		})
	}
	return accounts, rows.Err()
}

// FindOrCreateLDAPUser returns the UUID for an LDAP-authenticated user. On the
// first call for a given ldapUID it creates an organization and user row so the
// rest of the membership/authz system has a stable identifier to work with.
func (r *Repository) FindOrCreateLDAPUser(ctx context.Context, ldapUID, email string) (string, error) {
	var userID string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE ldap_uid = $1`, ldapUID,
	).Scan(&userID)
	if err == nil {
		return userID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("lookup LDAP user: %w", err)
	}

	if err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var orgID string
		if err := tx.QueryRow(ctx,
			`INSERT INTO organizations (name) VALUES ($1) RETURNING id`, ldapUID,
		).Scan(&orgID); err != nil {
			return fmt.Errorf("create org for %s: %w", ldapUID, err)
		}
		return tx.QueryRow(ctx,
			`INSERT INTO users (org_id, email, ldap_uid) VALUES ($1, $2, $3) RETURNING id`,
			orgID, email, ldapUID,
		).Scan(&userID)
	}); err != nil {
		return "", fmt.Errorf("provision LDAP user %s: %w", ldapUID, err)
	}
	return userID, nil
}

// UserBookRole returns the user's role on the book and whether a membership row
// exists at all (false with no error means the user has no membership).
func (r *Repository) UserBookRole(ctx context.Context, userID, bookGUID string) (app.Role, bool, error) {
	var role string
	err := r.pool.QueryRow(ctx,
		`SELECT role FROM memberships WHERE user_id = $1 AND book_guid = $2`, userID, bookGUID,
	).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("check book role: %w", err)
	}
	return app.Role(role), true, nil
}

// BookGUIDForAccount returns the book an account belongs to by walking up the
// parent_guid chain to the root account, which a book references via
// root_account_guid. Returns app.ErrAccountNotFound if the account does not
// exist (or, defensively, is not attached to any book's root).
func (r *Repository) BookGUIDForAccount(ctx context.Context, accountGUID string) (string, error) {
	const sql = `
WITH RECURSIVE up AS (
    SELECT guid, parent_guid FROM accounts WHERE guid = $1
    UNION ALL
    SELECT a.guid, a.parent_guid
    FROM accounts a JOIN up ON a.guid = up.parent_guid
)
SELECT b.guid FROM books b JOIN up ON b.root_account_guid = up.guid`

	var bookGUID string
	err := r.pool.QueryRow(ctx, sql, accountGUID).Scan(&bookGUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", app.ErrAccountNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve book for account: %w", err)
	}
	return bookGUID, nil
}

// AccountExists reports whether an account with the given GUID exists.
func (r *Repository) AccountExists(ctx context.Context, guid string) (bool, error) {
	var one int
	err := r.pool.QueryRow(ctx, `SELECT 1 FROM accounts WHERE guid = $1`, guid).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

const registerSQL = `
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
LIMIT $2 OFFSET $3`

// ListAccountRegister returns one page of an account's splits ordered by date,
// each with the running balance (in the account's commodity) from the start of
// the ledger. The running balance is a window sum of quantity_num, which is
// exact because every split for an account shares the account commodity's
// fraction as its denominator. total is the full (unpaginated) row count.
func (r *Repository) ListAccountRegister(ctx context.Context, guid string, limit, offset int) ([]app.RegisterEntry, int64, error) {
	rows, err := r.pool.Query(ctx, registerSQL, guid, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query register: %w", err)
	}
	defer rows.Close()

	var (
		entries []app.RegisterEntry
		total   int64
	)
	for rows.Next() {
		var (
			e                                                          app.RegisterEntry
			reconcile                                                  string
			valueNum, valueDenom, qtyNum, qtyDenom, runningNum, rowTot int64
		)
		if err := rows.Scan(
			&e.SplitGUID, &e.TxGUID, &e.Memo, &reconcile,
			&valueNum, &valueDenom, &qtyNum, &qtyDenom,
			&e.PostDate, &e.Description, &runningNum, &rowTot,
		); err != nil {
			return nil, 0, fmt.Errorf("scan register row: %w", err)
		}
		total = rowTot
		e.ValueScale = valueDenom
		e.QuantityScale = qtyDenom
		if reconcile != "" {
			e.Reconcile = domain.ReconcileState([]rune(reconcile)[0])
		}
		if e.Value, err = domain.FromNumDenom(valueNum, valueDenom); err != nil {
			return nil, 0, err
		}
		if e.Quantity, err = domain.FromNumDenom(qtyNum, qtyDenom); err != nil {
			return nil, 0, err
		}
		if e.Balance, err = domain.FromNumDenom(runningNum, qtyDenom); err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}

// uniqueViolation is the Postgres SQLSTATE for a unique-constraint violation.
const uniqueViolation = "23505"

// CreateOrgAndUser creates an organization and its first user in one DB
// transaction, returning their generated UUIDs. A duplicate email maps to
// app.ErrEmailTaken.
func (r *Repository) CreateOrgAndUser(ctx context.Context, orgName, email, passwordHash string) (string, string, error) {
	var userID, orgID string
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`INSERT INTO organizations (name) VALUES ($1) RETURNING id`, orgName,
		).Scan(&orgID); err != nil {
			return fmt.Errorf("insert organization: %w", err)
		}
		if err := tx.QueryRow(ctx,
			`INSERT INTO users (org_id, email, password_hash) VALUES ($1, $2, $3) RETURNING id`,
			orgID, email, passwordHash,
		).Scan(&userID); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
				return app.ErrEmailTaken
			}
			return fmt.Errorf("insert user: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", "", err
	}
	return userID, orgID, nil
}

// UserByEmail returns login credentials for an email (case-insensitive), or
// app.ErrInvalidCredentials when no such user exists.
func (r *Repository) UserByEmail(ctx context.Context, email string) (app.UserCredentials, error) {
	var c app.UserCredentials
	err := r.pool.QueryRow(ctx,
		`SELECT id, org_id, password_hash FROM users WHERE lower(email) = lower($1)`, email,
	).Scan(&c.UserID, &c.OrgID, &c.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.UserCredentials{}, app.ErrInvalidCredentials
	}
	if err != nil {
		return app.UserCredentials{}, fmt.Errorf("lookup user: %w", err)
	}
	return c, nil
}

// StoreRefreshToken records a hashed refresh token.
func (r *Repository) StoreRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt,
	); err != nil {
		return fmt.Errorf("store refresh token: %w", err)
	}
	return nil
}

// RotateRefreshToken revokes oldHash (if active and unexpired) and stores
// newHash for the same user, all in one transaction. The old row is locked
// FOR UPDATE so concurrent reuse can't double-spend it. Returns
// app.ErrInvalidRefresh if oldHash is not currently valid.
func (r *Repository) RotateRefreshToken(ctx context.Context, oldHash, newHash string, newExpiresAt time.Time) (string, string, error) {
	var userID, orgID string
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT user_id FROM refresh_tokens
			  WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()
			  FOR UPDATE`, oldHash,
		).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrInvalidRefresh
		}
		if err != nil {
			return fmt.Errorf("lookup refresh token: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1`, oldHash,
		); err != nil {
			return fmt.Errorf("revoke old refresh token: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
			userID, newHash, newExpiresAt,
		); err != nil {
			return fmt.Errorf("store rotated refresh token: %w", err)
		}
		if err := tx.QueryRow(ctx, `SELECT org_id FROM users WHERE id = $1`, userID).Scan(&orgID); err != nil {
			return fmt.Errorf("lookup user org: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", "", err
	}
	return userID, orgID, nil
}

// RevokeRefreshToken marks a refresh token revoked. Idempotent.
func (r *Repository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`,
		tokenHash,
	); err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

// nullable maps an empty string to SQL NULL, so optional CHAR/UUID columns are
// stored as NULL rather than an empty/invalid value.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// boolToInt maps Go bools to the 0/1 INTEGER columns GnuCash uses for flags
// like hidden and placeholder.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
