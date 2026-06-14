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

// ImportBook persists a parsed GnuCash book in a single DB transaction:
// commodities first (accounts reference them), then accounts in parent-before-
// child order (the self-referential parent_guid FK), then the book row and its
// owner membership, then every transaction with its splits. A primary-key
// collision (re-importing the same file) maps to app.ErrImportConflict.
func (r *Repository) ImportBook(ctx context.Context, data app.GnuCashData, ownerUserID string) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		for _, c := range data.Commodities {
			if _, err := tx.Exec(ctx,
				`INSERT INTO commodities (guid, namespace, mnemonic, fullname, fraction)
				 VALUES ($1, $2, $3, $4, $5)`,
				c.GUID, c.Namespace, c.Mnemonic, nullable(c.Fullname), c.Fraction,
			); err != nil {
				return importErr("insert commodity", err)
			}
		}

		if err := insertAccountsParentFirst(ctx, tx, data.Accounts); err != nil {
			return err
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO books (guid, root_account_guid, root_template_guid)
			 VALUES ($1, $2, $3)`,
			data.Book.GUID, data.Book.RootAccountGUID, data.Book.RootTemplateGUID,
		); err != nil {
			return importErr("insert book", err)
		}
		if ownerUserID != "" {
			if _, err := tx.Exec(ctx,
				`INSERT INTO memberships (user_id, book_guid, role) VALUES ($1, $2, 'owner')`,
				ownerUserID, data.Book.GUID,
			); err != nil {
				return fmt.Errorf("insert owner membership: %w", err)
			}
		}

		for _, t := range data.Transactions {
			if _, err := tx.Exec(ctx,
				`INSERT INTO transactions (guid, currency_guid, num, post_date, enter_date, description)
				 VALUES ($1, $2, $3, $4, $5, $6)`,
				t.GUID, t.CurrencyGUID, t.Num, t.PostDate, t.EnterDate, t.Description,
			); err != nil {
				return importErr("insert transaction", err)
			}
			if err := r.insertSplits(ctx, tx, t); err != nil {
				return err
			}
		}
		return nil
	})
}

// insertAccountsParentFirst writes accounts so that each parent is inserted
// before its children, satisfying the self-referential parent_guid FK. Accounts
// with no parent in the set (the root) go first. It errors if an account's
// parent is missing from the set (an orphan), which would otherwise loop forever.
func insertAccountsParentFirst(ctx context.Context, tx pgx.Tx, accounts []domain.Account) error {
	present := make(map[string]bool, len(accounts))
	for _, a := range accounts {
		present[a.GUID] = true
	}
	done := make(map[string]bool, len(accounts))
	remaining := accounts
	for len(remaining) > 0 {
		next := remaining[:0]
		progressed := false
		for _, a := range remaining {
			parentReady := a.ParentGUID == "" || !present[a.ParentGUID] || done[a.ParentGUID]
			if parentReady {
				if err := insertAccount(ctx, tx, a); err != nil {
					return importErr("insert account", err)
				}
				done[a.GUID] = true
				progressed = true
			} else {
				next = append(next, a)
			}
		}
		if !progressed {
			return fmt.Errorf("import: account tree has a cycle or orphan among %d accounts", len(remaining))
		}
		remaining = next
	}
	return nil
}

// importErr maps a primary-key/unique collision during import to
// app.ErrImportConflict and otherwise wraps the error with context.
func importErr(context string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return app.ErrImportConflict
	}
	return fmt.Errorf("%s: %w", context, err)
}

// LoadBook reads an entire book for export: the book row, its account tree
// (the regular and template roots and all their descendants), every
// transaction that touches one of those accounts together with its splits, and
// the commodities those accounts and transactions reference. It returns
// app.ErrBookNotFound for an unknown book. It is the read counterpart of
// ImportBook.
func (r *Repository) LoadBook(ctx context.Context, bookGUID string) (app.GnuCashData, error) {
	// A read-only RepeatableRead transaction gives the export a single
	// consistent snapshot, so a concurrent write between the separate account /
	// transaction / split / commodity queries can't be partially observed.
	var data app.GnuCashData
	err := pgx.BeginTxFunc(ctx, r.pool,
		pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
		func(tx pgx.Tx) error {
			var book domain.Book
			err := tx.QueryRow(ctx,
				`SELECT guid, root_account_guid, root_template_guid FROM books WHERE guid = $1`, bookGUID,
			).Scan(&book.GUID, &book.RootAccountGUID, &book.RootTemplateGUID)
			if errors.Is(err, pgx.ErrNoRows) {
				return app.ErrBookNotFound
			}
			if err != nil {
				return fmt.Errorf("load book: %w", err)
			}

			accounts, err := loadBookAccounts(ctx, tx, book.RootAccountGUID, book.RootTemplateGUID)
			if err != nil {
				return err
			}
			accountGUIDs := make([]string, len(accounts))
			for i, a := range accounts {
				accountGUIDs[i] = a.GUID
			}

			transactions, err := loadBookTransactions(ctx, tx, accountGUIDs)
			if err != nil {
				return err
			}

			commodities, err := loadCommodities(ctx, tx, referencedCommodities(accounts, transactions))
			if err != nil {
				return err
			}

			data = app.GnuCashData{
				Book:         book,
				Commodities:  commodities,
				Accounts:     accounts,
				Transactions: transactions,
			}
			return nil
		})
	if err != nil {
		return app.GnuCashData{}, err
	}
	return data, nil
}

// loadBookAccounts reads the account tree anchored at the book's two roots
// (the regular root and the template root), including the roots themselves.
func loadBookAccounts(ctx context.Context, tx pgx.Tx, rootGUID, templateRootGUID string) ([]domain.Account, error) {
	const sql = `
WITH RECURSIVE tree AS (
    SELECT guid, name, account_type, commodity_guid, parent_guid, code, description, hidden, placeholder
    FROM accounts WHERE guid = $1 OR guid = $2
    UNION ALL
    SELECT a.guid, a.name, a.account_type, a.commodity_guid, a.parent_guid, a.code, a.description, a.hidden, a.placeholder
    FROM accounts a JOIN tree t ON a.parent_guid = t.guid
)
SELECT guid, name, account_type, commodity_guid, parent_guid, code, description, hidden, placeholder
FROM tree
ORDER BY guid`

	rows, err := tx.Query(ctx, sql, rootGUID, templateRootGUID)
	if err != nil {
		return nil, fmt.Errorf("load accounts: %w", err)
	}
	defer rows.Close()

	var accounts []domain.Account
	for rows.Next() {
		var (
			a                   domain.Account
			accountType         string
			commodity, parent   *string
			hidden, placeholder int
		)
		if err := rows.Scan(
			&a.GUID, &a.Name, &accountType, &commodity, &parent,
			&a.Code, &a.Description, &hidden, &placeholder,
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
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// loadBookTransactions reads every transaction with at least one split posted to
// one of accountGUIDs, together with all of each transaction's splits.
func loadBookTransactions(ctx context.Context, tx pgx.Tx, accountGUIDs []string) ([]domain.Transaction, error) {
	if len(accountGUIDs) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx,
		`SELECT guid, currency_guid, num, post_date, enter_date, description
		   FROM transactions
		  WHERE guid IN (SELECT DISTINCT tx_guid FROM splits WHERE account_guid = ANY($1))
		  ORDER BY post_date, guid`, accountGUIDs)
	if err != nil {
		return nil, fmt.Errorf("load transactions: %w", err)
	}

	var txns []domain.Transaction
	for rows.Next() {
		var t domain.Transaction
		if err := rows.Scan(&t.GUID, &t.CurrencyGUID, &t.Num, &t.PostDate, &t.EnterDate, &t.Description); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan transaction: %w", err)
		}
		txns = append(txns, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Index by GUID after the slice is fully built so the pointers are stable.
	byGUID := make(map[string]*domain.Transaction, len(txns))
	txGUIDs := make([]string, len(txns))
	for i := range txns {
		byGUID[txns[i].GUID] = &txns[i]
		txGUIDs[i] = txns[i].GUID
	}
	if err := attachBookSplits(ctx, tx, byGUID, txGUIDs); err != nil {
		return nil, err
	}
	return txns, nil
}

func attachBookSplits(ctx context.Context, tx pgx.Tx, byGUID map[string]*domain.Transaction, txGUIDs []string) error {
	rows, err := tx.Query(ctx,
		`SELECT guid, tx_guid, account_guid, memo, action, reconcile_state,
		        value_num, value_denom, quantity_num, quantity_denom, lot_guid
		   FROM splits WHERE tx_guid = ANY($1) ORDER BY tx_guid, guid`, txGUIDs)
	if err != nil {
		return fmt.Errorf("load splits: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			s                                      domain.Split
			txGUID                                 string
			reconcile                              string
			lot                                    *string
			valueNum, valueDenom, qtyNum, qtyDenom int64
		)
		if err := rows.Scan(
			&s.GUID, &txGUID, &s.AccountGUID, &s.Memo, &s.Action, &reconcile,
			&valueNum, &valueDenom, &qtyNum, &qtyDenom, &lot,
		); err != nil {
			return fmt.Errorf("scan split: %w", err)
		}
		if reconcile != "" {
			s.Reconcile = domain.ReconcileState([]rune(reconcile)[0])
		}
		if lot != nil {
			s.LotGUID = *lot
		}
		if s.Value, err = domain.FromNumDenom(valueNum, valueDenom); err != nil {
			return fmt.Errorf("split %s value: %w", s.GUID, err)
		}
		if s.Quantity, err = domain.FromNumDenom(qtyNum, qtyDenom); err != nil {
			return fmt.Errorf("split %s quantity: %w", s.GUID, err)
		}
		if t, ok := byGUID[txGUID]; ok {
			t.Splits = append(t.Splits, s)
		}
	}
	return rows.Err()
}

// loadCommodities reads the commodities with the given GUIDs.
func loadCommodities(ctx context.Context, tx pgx.Tx, guids []string) ([]domain.Commodity, error) {
	if len(guids) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx,
		`SELECT guid, namespace, mnemonic, fullname, fraction
		   FROM commodities WHERE guid = ANY($1) ORDER BY namespace, mnemonic`, guids)
	if err != nil {
		return nil, fmt.Errorf("load commodities: %w", err)
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

// referencedCommodities returns the distinct commodity GUIDs used by the given
// accounts (their denomination) and transactions (their currency), in first-seen
// order.
func referencedCommodities(accounts []domain.Account, txns []domain.Transaction) []string {
	seen := make(map[string]bool)
	var guids []string
	add := func(g string) {
		if g != "" && !seen[g] {
			seen[g] = true
			guids = append(guids, g)
		}
	}
	for _, a := range accounts {
		add(a.CommodityGUID)
	}
	for _, t := range txns {
		add(t.CurrencyGUID)
	}
	return guids
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

// AccountGUIDForSplit returns the account a split is posted to, or
// app.ErrSplitNotFound if the split does not exist.
func (r *Repository) AccountGUIDForSplit(ctx context.Context, splitGUID string) (string, error) {
	var accountGUID string
	err := r.pool.QueryRow(ctx,
		`SELECT account_guid FROM splits WHERE guid = $1`, splitGUID,
	).Scan(&accountGUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", app.ErrSplitNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lookup split account: %w", err)
	}
	return accountGUID, nil
}

// SetSplitReconcile sets a split's reconcile state and date, returning
// app.ErrSplitNotFound if no split has that GUID. It writes only the two
// reconcile columns; amounts are untouched.
func (r *Repository) SetSplitReconcile(ctx context.Context, splitGUID string, state domain.ReconcileState, date *time.Time) error {
	ct, err := r.pool.Exec(ctx,
		`UPDATE splits SET reconcile_state = $1, reconcile_date = $2 WHERE guid = $3`,
		string(state), date, splitGUID,
	)
	if err != nil {
		return fmt.Errorf("set split reconcile: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return app.ErrSplitNotFound
	}
	return nil
}

// AccountCommodity returns the commodity an account is denominated in and
// whether the account is itself a trading account, or app.ErrAccountNotFound if
// the account is unknown (or has no commodity, which a postable account always
// does).
func (r *Repository) AccountCommodity(ctx context.Context, accountGUID string) (app.AccountCommodityInfo, error) {
	var (
		acctType string
		c        domain.Commodity
		fullname *string
	)
	err := r.pool.QueryRow(ctx,
		`SELECT a.account_type, c.guid, c.namespace, c.mnemonic, c.fullname, c.fraction
		   FROM accounts a JOIN commodities c ON c.guid = a.commodity_guid
		  WHERE a.guid = $1`, accountGUID,
	).Scan(&acctType, &c.GUID, &c.Namespace, &c.Mnemonic, &fullname, &c.Fraction)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.AccountCommodityInfo{}, app.ErrAccountNotFound
	}
	if err != nil {
		return app.AccountCommodityInfo{}, fmt.Errorf("lookup account commodity: %w", err)
	}
	if fullname != nil {
		c.Fullname = *fullname
	}
	return app.AccountCommodityInfo{
		Commodity: c,
		IsTrading: acctType == string(domain.AccountTrading),
	}, nil
}

// FindOrCreateTradingAccount returns the GUID of the Trading:NAMESPACE:MNEMONIC
// account for commodity c in the book anchorAccountGUID belongs to, creating any
// missing level of the Trading hierarchy under the book's root. The three levels
// (Trading, the namespace, the mnemonic leaf) are all TRADING accounts; only the
// leaf carries the commodity, and it is the account splits post to. The lookups
// and inserts run in one transaction.
func (r *Repository) FindOrCreateTradingAccount(ctx context.Context, anchorAccountGUID string, c domain.Commodity) (string, error) {
	bookGUID, err := r.BookGUIDForAccount(ctx, anchorAccountGUID)
	if err != nil {
		return "", err
	}
	var leaf string
	err = pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var root string
		if err := tx.QueryRow(ctx,
			`SELECT root_account_guid FROM books WHERE guid = $1`, bookGUID,
		).Scan(&root); err != nil {
			return fmt.Errorf("lookup book root: %w", err)
		}
		trading, err := findOrCreateTradingChild(ctx, tx, root, "Trading", "")
		if err != nil {
			return err
		}
		ns, err := findOrCreateTradingChild(ctx, tx, trading, c.Namespace, "")
		if err != nil {
			return err
		}
		leaf, err = findOrCreateTradingChild(ctx, tx, ns, c.Mnemonic, c.GUID)
		return err
	})
	if err != nil {
		return "", err
	}
	return leaf, nil
}

// findOrCreateTradingChild returns the GUID of the TRADING account named name
// directly under parentGUID, creating it (with the given commodity, which may be
// empty for the intermediate levels) if it does not exist.
func findOrCreateTradingChild(ctx context.Context, tx pgx.Tx, parentGUID, name, commodityGUID string) (string, error) {
	var guid string
	err := tx.QueryRow(ctx,
		`SELECT guid FROM accounts WHERE parent_guid = $1 AND name = $2 AND account_type = 'TRADING'`,
		parentGUID, name,
	).Scan(&guid)
	if err == nil {
		return guid, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("lookup trading account %q: %w", name, err)
	}
	guid = app.NewGUID()
	if err := insertAccount(ctx, tx, domain.Account{
		GUID:          guid,
		Name:          name,
		Type:          domain.AccountTrading,
		CommodityGUID: commodityGUID,
		ParentGUID:    parentGUID,
	}); err != nil {
		return "", err
	}
	return guid, nil
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

// GetTransaction loads a transaction and all its splits. It returns
// app.ErrTransactionNotFound if the GUID is unknown.
func (r *Repository) GetTransaction(ctx context.Context, guid string) (domain.Transaction, error) {
	t := domain.Transaction{GUID: guid}
	err := r.pool.QueryRow(ctx,
		`SELECT currency_guid, num, post_date, enter_date, description
		   FROM transactions WHERE guid = $1`, guid,
	).Scan(&t.CurrencyGUID, &t.Num, &t.PostDate, &t.EnterDate, &t.Description)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Transaction{}, app.ErrTransactionNotFound
	}
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("get transaction: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT guid, account_guid, memo, action, reconcile_state,
		        value_num, value_denom, quantity_num, quantity_denom, lot_guid
		   FROM splits WHERE tx_guid = $1 ORDER BY guid`, guid)
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("get transaction splits: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			s                                      domain.Split
			reconcile                              string
			lot                                    *string
			valueNum, valueDenom, qtyNum, qtyDenom int64
		)
		if err := rows.Scan(
			&s.GUID, &s.AccountGUID, &s.Memo, &s.Action, &reconcile,
			&valueNum, &valueDenom, &qtyNum, &qtyDenom, &lot,
		); err != nil {
			return domain.Transaction{}, fmt.Errorf("scan split: %w", err)
		}
		if reconcile != "" {
			s.Reconcile = domain.ReconcileState([]rune(reconcile)[0])
		}
		if lot != nil {
			s.LotGUID = *lot
		}
		if s.Value, err = domain.FromNumDenom(valueNum, valueDenom); err != nil {
			return domain.Transaction{}, err
		}
		if s.Quantity, err = domain.FromNumDenom(qtyNum, qtyDenom); err != nil {
			return domain.Transaction{}, err
		}
		t.Splits = append(t.Splits, s)
	}
	return t, rows.Err()
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
