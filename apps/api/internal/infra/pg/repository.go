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

// InsertCommodity writes a commodity row. ON CONFLICT DO NOTHING means re-importing
// a GnuCash file that references a commodity already in the DB is a no-op rather
// than a duplicate-key error or a silent second row.
func (r *Repository) InsertCommodity(ctx context.Context, c domain.Commodity) error {
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO commodities (guid, namespace, mnemonic, fullname, fraction)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (namespace, mnemonic) DO NOTHING`,
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

// GetCommodity returns one commodity by GUID, or app.ErrCommodityNotFound.
func (r *Repository) GetCommodity(ctx context.Context, guid string) (domain.Commodity, error) {
	var (
		c        domain.Commodity
		fullname *string
	)
	err := r.pool.QueryRow(ctx,
		`SELECT guid, namespace, mnemonic, fullname, fraction
		 FROM commodities WHERE guid = $1`, guid).
		Scan(&c.GUID, &c.Namespace, &c.Mnemonic, &fullname, &c.Fraction)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Commodity{}, app.ErrCommodityNotFound
	}
	if err != nil {
		return domain.Commodity{}, fmt.Errorf("get commodity: %w", err)
	}
	if fullname != nil {
		c.Fullname = *fullname
	}
	return c, nil
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
		// Insert commodities; if one with the same (namespace, mnemonic) already
		// exists under a different GUID, skip the insert and remap all references
		// in accounts and transactions to the canonical existing GUID.
		guidRemap := make(map[string]string) // file-GUID → canonical DB GUID
		for _, c := range data.Commodities {
			if _, err := tx.Exec(ctx,
				`INSERT INTO commodities (guid, namespace, mnemonic, fullname, fraction)
				 VALUES ($1, $2, $3, $4, $5)
				 ON CONFLICT (namespace, mnemonic) DO NOTHING`,
				c.GUID, c.Namespace, c.Mnemonic, nullable(c.Fullname), c.Fraction,
			); err != nil {
				return importErr("insert commodity", err)
			}
			// Resolve the canonical GUID (may differ from c.GUID if DO NOTHING fired).
			var canonical string
			if err := tx.QueryRow(ctx,
				`SELECT guid FROM commodities WHERE namespace=$1 AND mnemonic=$2`,
				c.Namespace, c.Mnemonic,
			).Scan(&canonical); err != nil {
				return fmt.Errorf("resolve commodity %s/%s: %w", c.Namespace, c.Mnemonic, err)
			}
			if canonical != c.GUID {
				guidRemap[c.GUID] = canonical
			}
		}

		// Apply remap to accounts and transactions before inserting them.
		if len(guidRemap) > 0 {
			for i := range data.Accounts {
				if canon, ok := guidRemap[data.Accounts[i].CommodityGUID]; ok {
					data.Accounts[i].CommodityGUID = canon
				}
			}
			for i := range data.Transactions {
				if canon, ok := guidRemap[data.Transactions[i].CurrencyGUID]; ok {
					data.Transactions[i].CurrencyGUID = canon
				}
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

		for _, l := range data.Lots {
			closed := 0
			if l.IsClosed {
				closed = 1
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO lots (guid, account_guid, is_closed) VALUES ($1, $2, $3)`,
				l.GUID, l.AccountGUID, closed,
			); err != nil {
				return importErr("insert lot", err)
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

		// Apply commodity remap to scheduled transactions and insert them.
		for i := range data.ScheduledTransactions {
			if canon, ok := guidRemap[data.ScheduledTransactions[i].CurrencyGUID]; ok {
				data.ScheduledTransactions[i].CurrencyGUID = canon
			}
		}
		for _, s := range data.ScheduledTransactions {
			s.BookGUID = data.Book.GUID
			if _, err := tx.Exec(ctx,
				`INSERT INTO scheduled_transactions
				    (guid, book_guid, name, description, enabled, currency_guid, period, every,
				     start_date, end_date, last_posted)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
				s.GUID, s.BookGUID, s.Name, s.Description, boolToInt(s.Enabled),
				s.CurrencyGUID, string(s.Period), s.Every,
				schedDate(s.StartDate), schedDate(s.EndDate), schedDate(s.LastPostedDate),
			); err != nil {
				return importErr("insert scheduled transaction", err)
			}
			if err := insertScheduledSplits(ctx, tx, s.GUID, s.Splits); err != nil {
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

			lots, err := loadBookLots(ctx, tx, accountGUIDs)
			if err != nil {
				return err
			}

			scheds, err := loadBookScheduledTransactions(ctx, tx, book.GUID)
			if err != nil {
				return err
			}

			data = app.GnuCashData{
				Book:                  book,
				Commodities:           commodities,
				Accounts:              accounts,
				Transactions:          transactions,
				Lots:                  lots,
				ScheduledTransactions: scheds,
			}
			return nil
		})
	if err != nil {
		return app.GnuCashData{}, err
	}
	return data, nil
}

// loadBookLots reads the lots belonging to any of the book's accounts, so cost
// basis groupings survive export/import.
func loadBookLots(ctx context.Context, tx pgx.Tx, accountGUIDs []string) ([]domain.Lot, error) {
	if len(accountGUIDs) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx,
		`SELECT guid, account_guid, is_closed FROM lots WHERE account_guid = ANY($1) ORDER BY guid`,
		accountGUIDs)
	if err != nil {
		return nil, fmt.Errorf("load lots: %w", err)
	}
	defer rows.Close()

	var lots []domain.Lot
	for rows.Next() {
		var (
			l      domain.Lot
			closed int
		)
		if err := rows.Scan(&l.GUID, &l.AccountGUID, &closed); err != nil {
			return nil, fmt.Errorf("scan lot: %w", err)
		}
		l.IsClosed = closed != 0
		lots = append(lots, l)
	}
	return lots, rows.Err()
}

// loadBookScheduledTransactions loads all scheduled transactions (with their
// template splits) for a book within an existing read transaction.
func loadBookScheduledTransactions(ctx context.Context, tx pgx.Tx, bookGUID string) ([]domain.ScheduledTransaction, error) {
	rows, err := tx.Query(ctx,
		`SELECT guid, book_guid, name, description, enabled, currency_guid, period, every,
		        start_date, end_date, last_posted
		   FROM scheduled_transactions WHERE book_guid = $1 ORDER BY name`, bookGUID)
	if err != nil {
		return nil, fmt.Errorf("load scheduled transactions: %w", err)
	}
	defer rows.Close()

	var scheds []domain.ScheduledTransaction
	for rows.Next() {
		s, err := scanScheduledTransaction(rows)
		if err != nil {
			return nil, err
		}
		scheds = append(scheds, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range scheds {
		splitRows, err := tx.Query(ctx,
			`SELECT guid, account_guid, memo, value_num, value_denom
			   FROM scheduled_splits WHERE schedtx_guid = $1 ORDER BY guid`, scheds[i].GUID)
		if err != nil {
			return nil, fmt.Errorf("load scheduled splits for %s: %w", scheds[i].GUID, err)
		}
		for splitRows.Next() {
			var (
				sp           domain.ScheduledSplit
				valueNum, vD int64
			)
			if err := splitRows.Scan(&sp.GUID, &sp.AccountGUID, &sp.Memo, &valueNum, &vD); err != nil {
				splitRows.Close()
				return nil, fmt.Errorf("scan scheduled split: %w", err)
			}
			if sp.Value, err = domain.FromNumDenom(valueNum, vD); err != nil {
				splitRows.Close()
				return nil, fmt.Errorf("scheduled split %s value: %w", sp.GUID, err)
			}
			scheds[i].Splits = append(scheds[i].Splits, sp)
		}
		splitRows.Close()
		if err := splitRows.Err(); err != nil {
			return nil, err
		}
	}
	return scheds, nil
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

// securityHoldingsTree is the recursive CTE selecting every STOCK/MUTUAL
// descendant of a root account; both the share-quantity and cost-basis queries
// build on it so they cover the same set of security accounts.
const securityHoldingsTree = `
WITH RECURSIVE tree AS (
    SELECT guid, name, account_type, commodity_guid, parent_guid, code, description, hidden, placeholder
    FROM accounts WHERE parent_guid = $1
    UNION ALL
    SELECT a.guid, a.name, a.account_type, a.commodity_guid, a.parent_guid, a.code, a.description, a.hidden, a.placeholder
    FROM accounts a JOIN tree t ON a.parent_guid = t.guid
)`

// SecurityHoldings returns every STOCK/MUTUAL descendant of rootGUID with its
// share quantity (summed at the account's commodity fraction) and its cost
// basis. Cost basis is the sum of split values; because value is stored at each
// transaction's currency fraction, the values are summed per denominator in SQL
// and combined exactly with GncNumeric, so a mixed-currency cost basis stays
// exact rather than collapsing onto one denominator.
func (r *Repository) SecurityHoldings(ctx context.Context, rootGUID string) ([]app.HoldingBalance, error) {
	shareRows, err := r.pool.Query(ctx, securityHoldingsTree+`
SELECT t.guid, t.name, t.account_type, t.commodity_guid, t.parent_guid, t.code, t.description, t.hidden, t.placeholder,
       COALESCE(SUM(s.quantity_num), 0) AS shares_num,
       COALESCE(c.fraction, 1)          AS shares_denom
FROM tree t
LEFT JOIN commodities c ON c.guid = t.commodity_guid
LEFT JOIN splits s      ON s.account_guid = t.guid
WHERE t.account_type IN ('STOCK', 'MUTUAL')
GROUP BY t.guid, t.name, t.account_type, t.commodity_guid, t.parent_guid, t.code, t.description, t.hidden, t.placeholder, c.fraction
ORDER BY t.code, t.name`, rootGUID)
	if err != nil {
		return nil, fmt.Errorf("security holdings: %w", err)
	}
	balances, err := scanAccountBalances(shareRows)
	if err != nil {
		return nil, err
	}

	// Cost basis: one row per (account, value_denom); fold them into an exact
	// per-account total.
	costRows, err := r.pool.Query(ctx, securityHoldingsTree+`
SELECT s.account_guid, s.value_denom, COALESCE(SUM(s.value_num), 0)
FROM tree t
JOIN splits s ON s.account_guid = t.guid
WHERE t.account_type IN ('STOCK', 'MUTUAL')
GROUP BY s.account_guid, s.value_denom`, rootGUID)
	if err != nil {
		return nil, fmt.Errorf("security cost basis: %w", err)
	}
	defer costRows.Close()

	cost := make(map[string]domain.GncNumeric)
	for costRows.Next() {
		var (
			accountGUID string
			denom, num  int64
		)
		if err := costRows.Scan(&accountGUID, &denom, &num); err != nil {
			return nil, fmt.Errorf("scan cost basis: %w", err)
		}
		part, err := domain.FromNumDenom(num, denom)
		if err != nil {
			return nil, fmt.Errorf("cost basis for %s: %w", accountGUID, err)
		}
		cost[accountGUID] = cost[accountGUID].Add(part)
	}
	if err := costRows.Err(); err != nil {
		return nil, err
	}

	holdings := make([]app.HoldingBalance, 0, len(balances))
	for _, b := range balances {
		holdings = append(holdings, app.HoldingBalance{
			Account:    b.Account,
			Shares:     b.Balance,
			ShareScale: b.BalanceScale,
			CostBasis:  cost[b.Account.GUID],
		})
	}
	return holdings, nil
}

// LatestPrice returns a commodity's most recent quote, or ok=false when the
// commodity has no quotes.
func (r *Repository) LatestPrice(ctx context.Context, commodityGUID string) (domain.Price, bool, error) {
	var (
		p                 domain.Price
		source, priceType *string
		valueNum, valDen  int64
	)
	err := r.pool.QueryRow(ctx,
		`SELECT guid, commodity_guid, currency_guid, date, source, type, value_num, value_denom
		 FROM prices WHERE commodity_guid = $1 ORDER BY date DESC LIMIT 1`, commodityGUID,
	).Scan(&p.GUID, &p.CommodityGUID, &p.CurrencyGUID, &p.Date, &source, &priceType, &valueNum, &valDen)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Price{}, false, nil
	}
	if err != nil {
		return domain.Price{}, false, fmt.Errorf("latest price: %w", err)
	}
	if source != nil {
		p.Source = *source
	}
	if priceType != nil {
		p.Type = *priceType
	}
	if p.Value, err = domain.FromNumDenom(valueNum, valDen); err != nil {
		return domain.Price{}, false, fmt.Errorf("price %s value: %w", p.GUID, err)
	}
	return p, true, nil
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

// ListBookMembers returns every member of a book with their role, ordered by
// email for a stable members screen.
func (r *Repository) ListBookMembers(ctx context.Context, bookGUID string) ([]app.Member, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT u.id, u.email, COALESCE(u.ldap_uid, ''), m.role
		   FROM memberships m
		   JOIN users u ON u.id = m.user_id
		  WHERE m.book_guid = $1
		  ORDER BY lower(u.email)`, bookGUID)
	if err != nil {
		return nil, fmt.Errorf("list book members: %w", err)
	}
	defer rows.Close()

	var members []app.Member
	for rows.Next() {
		var m app.Member
		var role string
		if err := rows.Scan(&m.UserID, &m.Email, &m.LDAPUser, &role); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		m.Role = app.Role(role)
		members = append(members, m)
	}
	return members, rows.Err()
}

// FindUserByEmail returns the user with the given email (case-insensitive),
// or app.ErrUserNotFound if none has been provisioned.
func (r *Repository) FindUserByEmail(ctx context.Context, email string) (app.Member, error) {
	var m app.Member
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, COALESCE(ldap_uid, '') FROM users WHERE lower(email) = lower($1)`, email,
	).Scan(&m.UserID, &m.Email, &m.LDAPUser)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.Member{}, app.ErrUserNotFound
	}
	if err != nil {
		return app.Member{}, fmt.Errorf("find user by email: %w", err)
	}
	return m, nil
}

// CountBookOwners returns how many owner memberships a book has.
func (r *Repository) CountBookOwners(ctx context.Context, bookGUID string) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM memberships WHERE book_guid = $1 AND role = 'owner'`, bookGUID,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("count book owners: %w", err)
	}
	return n, nil
}

// UpsertMembership creates or updates a user's role on a book.
func (r *Repository) UpsertMembership(ctx context.Context, userID, bookGUID string, role app.Role) error {
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO memberships (user_id, book_guid, role) VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, book_guid) DO UPDATE SET role = EXCLUDED.role`,
		userID, bookGUID, string(role),
	); err != nil {
		return fmt.Errorf("upsert membership: %w", err)
	}
	return nil
}

// DeleteMembership removes a user's membership on a book.
func (r *Repository) DeleteMembership(ctx context.Context, userID, bookGUID string) error {
	if _, err := r.pool.Exec(ctx,
		`DELETE FROM memberships WHERE user_id = $1 AND book_guid = $2`, userID, bookGUID,
	); err != nil {
		return fmt.Errorf("delete membership: %w", err)
	}
	return nil
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

// CreateLot inserts a new, open lot for a security account.
func (r *Repository) CreateLot(ctx context.Context, lotGUID, accountGUID string) error {
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO lots (guid, account_guid, is_closed) VALUES ($1, $2, 0)`,
		lotGUID, accountGUID,
	); err != nil {
		return fmt.Errorf("create lot: %w", err)
	}
	return nil
}

// SetLotClosed marks a lot closed (its shares have been fully sold).
func (r *Repository) SetLotClosed(ctx context.Context, lotGUID string) error {
	if _, err := r.pool.Exec(ctx, `UPDATE lots SET is_closed = 1 WHERE guid = $1`, lotGUID); err != nil {
		return fmt.Errorf("close lot: %w", err)
	}
	return nil
}

// OpenLotsForAccount returns the account's open lots in FIFO order (oldest split
// first), each with its remaining shares and the cost basis still attached. A
// lot's remaining shares are the sum of its splits' quantities and its remaining
// cost the sum of their values; lots that net to zero shares are excluded. The
// quantity and value denominators are uniform within an account/currency, so the
// sums are exact.
func (r *Repository) OpenLotsForAccount(ctx context.Context, accountGUID string) ([]domain.OpenLot, error) {
	rows, err := r.pool.Query(ctx, `
SELECT l.guid,
       COALESCE(SUM(s.quantity_num), 0) AS qty_num,
       COALESCE(MAX(s.quantity_denom), 1) AS qty_denom,
       COALESCE(SUM(s.value_num), 0) AS val_num,
       COALESCE(MAX(s.value_denom), 1) AS val_denom
FROM lots l
LEFT JOIN splits s       ON s.lot_guid = l.guid
LEFT JOIN transactions t ON t.guid = s.tx_guid
WHERE l.account_guid = $1 AND l.is_closed = 0
GROUP BY l.guid
HAVING COALESCE(SUM(s.quantity_num), 0) > 0
ORDER BY MIN(t.post_date), l.guid`, accountGUID)
	if err != nil {
		return nil, fmt.Errorf("open lots: %w", err)
	}
	defer rows.Close()

	var lots []domain.OpenLot
	for rows.Next() {
		var (
			guid             string
			qtyNum, qtyDenom int64
			valNum, valDenom int64
		)
		if err := rows.Scan(&guid, &qtyNum, &qtyDenom, &valNum, &valDenom); err != nil {
			return nil, fmt.Errorf("scan lot: %w", err)
		}
		remaining, err := domain.FromNumDenom(qtyNum, qtyDenom)
		if err != nil {
			return nil, fmt.Errorf("lot %s shares: %w", guid, err)
		}
		cost, err := domain.FromNumDenom(valNum, valDenom)
		if err != nil {
			return nil, fmt.Errorf("lot %s cost: %w", guid, err)
		}
		lots = append(lots, domain.OpenLot{GUID: guid, Remaining: remaining, Cost: cost})
	}
	return lots, rows.Err()
}

// RealizedGainRows returns every split posted to a "Capital Gains" INCOME
// account under rootGUID within [from, to], one row per split (oldest first),
// for the capital-gains report.
func (r *Repository) RealizedGainRows(ctx context.Context, rootGUID string, from, to *time.Time) ([]app.RealizedGainRow, error) {
	rows, err := r.pool.Query(ctx, `
WITH RECURSIVE tree AS (
    SELECT guid, name, account_type, commodity_guid
    FROM accounts WHERE parent_guid = $1
    UNION ALL
    SELECT a.guid, a.name, a.account_type, a.commodity_guid
    FROM accounts a JOIN tree t ON a.parent_guid = t.guid
)
SELECT t.post_date, t.description, a.guid, a.name,
       s.value_num, s.value_denom, COALESCE(c.fraction, 1)
FROM tree a
JOIN splits s        ON s.account_guid = a.guid
JOIN transactions t  ON t.guid = s.tx_guid
LEFT JOIN commodities c ON c.guid = a.commodity_guid
WHERE a.account_type = 'INCOME' AND a.name = 'Capital Gains'
  AND ($2::timestamptz IS NULL OR t.post_date >= $2)
  AND ($3::timestamptz IS NULL OR t.post_date <= $3)
ORDER BY t.post_date, s.guid`, rootGUID, from, to)
	if err != nil {
		return nil, fmt.Errorf("realized gains: %w", err)
	}
	defer rows.Close()

	var out []app.RealizedGainRow
	for rows.Next() {
		var (
			row              app.RealizedGainRow
			description      *string
			acctGUID, name   string
			valNum, valDenom int64
		)
		if err := rows.Scan(&row.Date, &description, &acctGUID, &name, &valNum, &valDenom, &row.Scale); err != nil {
			return nil, fmt.Errorf("scan realized gain: %w", err)
		}
		if description != nil {
			row.Description = *description
		}
		row.Account = domain.Account{GUID: acctGUID, Name: name, Type: domain.AccountIncome}
		if row.Value, err = domain.FromNumDenom(valNum, valDenom); err != nil {
			return nil, fmt.Errorf("realized gain value: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// FindOrCreateCapitalGainsAccount returns the GUID of the book's "Capital Gains"
// INCOME account for the given currency, creating it under the book root if
// missing. Realized gains and losses on security sales post here, so they flow
// into the income statement. Keyed by name and commodity, so a multi-currency
// book gets one gains account per currency.
func (r *Repository) FindOrCreateCapitalGainsAccount(ctx context.Context, anchorAccountGUID string, currency domain.Commodity) (string, error) {
	bookGUID, err := r.BookGUIDForAccount(ctx, anchorAccountGUID)
	if err != nil {
		return "", err
	}
	var guid string
	err = pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var root string
		if err := tx.QueryRow(ctx,
			`SELECT root_account_guid FROM books WHERE guid = $1`, bookGUID,
		).Scan(&root); err != nil {
			return fmt.Errorf("lookup book root: %w", err)
		}
		err := tx.QueryRow(ctx,
			`SELECT guid FROM accounts
			  WHERE parent_guid = $1 AND name = 'Capital Gains'
			    AND account_type = 'INCOME' AND commodity_guid = $2`,
			root, currency.GUID,
		).Scan(&guid)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("lookup capital gains account: %w", err)
		}
		guid = app.NewGUID()
		return insertAccount(ctx, tx, domain.Account{
			GUID:          guid,
			Name:          "Capital Gains",
			Type:          domain.AccountIncome,
			CommodityGUID: currency.GUID,
			ParentGUID:    root,
		})
	})
	if err != nil {
		return "", err
	}
	return guid, nil
}

// FindOrCreateImbalanceAccount returns the GUID of the Imbalance-MNEMONIC
// account (GnuCash's convention) under the book's root, creating it when
// absent. It is the offsetting account for uncategorised bank-import lines.
// Keyed by name and commodity, so a multi-currency book gets one per currency.
func (r *Repository) FindOrCreateImbalanceAccount(ctx context.Context, anchorAccountGUID string, currency domain.Commodity) (string, error) {
	bookGUID, err := r.BookGUIDForAccount(ctx, anchorAccountGUID)
	if err != nil {
		return "", err
	}
	name := "Imbalance-" + currency.Mnemonic
	var guid string
	err = pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var root string
		if err := tx.QueryRow(ctx,
			`SELECT root_account_guid FROM books WHERE guid = $1`, bookGUID,
		).Scan(&root); err != nil {
			return fmt.Errorf("lookup book root: %w", err)
		}
		err := tx.QueryRow(ctx,
			`SELECT guid FROM accounts
			  WHERE parent_guid = $1 AND name = $2
			    AND account_type = 'BANK' AND commodity_guid = $3`,
			root, name, currency.GUID,
		).Scan(&guid)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("lookup imbalance account: %w", err)
		}
		guid = app.NewGUID()
		return insertAccount(ctx, tx, domain.Account{
			GUID:          guid,
			Name:          name,
			Type:          domain.AccountBank,
			CommodityGUID: currency.GUID,
			ParentGUID:    root,
		})
	})
	if err != nil {
		return "", err
	}
	return guid, nil
}

// ExistingImportRefs returns the set of non-empty transaction num values posted
// to the account. Bank import stores its per-line ref (OFX FITID or a content
// hash) in num, so this set drives duplicate detection on re-import.
func (r *Repository) ExistingImportRefs(ctx context.Context, accountGUID string) (map[string]struct{}, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT t.num
		   FROM transactions t
		   JOIN splits s ON s.tx_guid = t.guid
		  WHERE s.account_guid = $1 AND t.num <> ''`, accountGUID)
	if err != nil {
		return nil, fmt.Errorf("list import refs: %w", err)
	}
	defer rows.Close()

	refs := make(map[string]struct{})
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, fmt.Errorf("scan import ref: %w", err)
		}
		refs[ref] = struct{}{}
	}
	return refs, rows.Err()
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

// schedDate formats a time.Time as a "YYYY-MM-DD" string (UTC) for storage in
// scheduled_transactions date columns, or nil for the zero time.
func schedDate(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format("2006-01-02")
}

// parseSchedDate parses a nullable "YYYY-MM-DD" string into a time.Time (UTC
// midnight), returning the zero time for a NULL/empty value.
func parseSchedDate(s *string) time.Time {
	if s == nil || *s == "" {
		return time.Time{}
	}
	t, err := time.ParseInLocation("2006-01-02", *s, time.UTC)
	if err != nil {
		return time.Time{}
	}
	return t
}

// CreateScheduledTransaction inserts a scheduled transaction and its template
// splits, assigning GUIDs when they are missing.
func (r *Repository) CreateScheduledTransaction(ctx context.Context, s domain.ScheduledTransaction) (domain.ScheduledTransaction, error) {
	return s, pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO scheduled_transactions
			    (guid, book_guid, name, description, enabled, currency_guid, period, every,
			     start_date, end_date, last_posted)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			s.GUID, s.BookGUID, s.Name, s.Description, boolToInt(s.Enabled),
			s.CurrencyGUID, string(s.Period), s.Every,
			schedDate(s.StartDate), schedDate(s.EndDate), schedDate(s.LastPostedDate),
		); err != nil {
			return fmt.Errorf("insert scheduled transaction: %w", err)
		}
		return insertScheduledSplits(ctx, tx, s.GUID, s.Splits)
	})
}

func insertScheduledSplits(ctx context.Context, tx pgx.Tx, schedGUID string, splits []domain.ScheduledSplit) error {
	for _, sp := range splits {
		vNum, vDenom, err := sp.Value.NumDenom()
		if err != nil {
			return fmt.Errorf("scheduled split %s value: %w", sp.GUID, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO scheduled_splits (guid, schedtx_guid, account_guid, memo, value_num, value_denom)
			 VALUES ($1,$2,$3,$4,$5,$6)`,
			sp.GUID, schedGUID, sp.AccountGUID, sp.Memo, vNum, vDenom,
		); err != nil {
			return fmt.Errorf("insert scheduled split %s: %w", sp.GUID, err)
		}
	}
	return nil
}

// ListScheduledTransactions returns all scheduled transactions for a book,
// including their template splits.
func (r *Repository) ListScheduledTransactions(ctx context.Context, bookGUID string) ([]domain.ScheduledTransaction, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT guid, book_guid, name, description, enabled, currency_guid, period, every,
		        start_date, end_date, last_posted
		   FROM scheduled_transactions WHERE book_guid = $1 ORDER BY name`, bookGUID)
	if err != nil {
		return nil, fmt.Errorf("list scheduled transactions: %w", err)
	}
	defer rows.Close()

	var scheds []domain.ScheduledTransaction
	for rows.Next() {
		s, err := scanScheduledTransaction(rows)
		if err != nil {
			return nil, err
		}
		scheds = append(scheds, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range scheds {
		splits, err := r.loadScheduledSplits(ctx, scheds[i].GUID)
		if err != nil {
			return nil, err
		}
		scheds[i].Splits = splits
	}
	return scheds, nil
}

// GetScheduledTransaction returns one scheduled transaction by GUID, or
// app.ErrScheduleNotFound if it does not exist.
func (r *Repository) GetScheduledTransaction(ctx context.Context, guid string) (domain.ScheduledTransaction, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT guid, book_guid, name, description, enabled, currency_guid, period, every,
		        start_date, end_date, last_posted
		   FROM scheduled_transactions WHERE guid = $1`, guid)
	s, err := scanScheduledTransaction(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ScheduledTransaction{}, app.ErrScheduleNotFound
	}
	if err != nil {
		return domain.ScheduledTransaction{}, err
	}
	s.Splits, err = r.loadScheduledSplits(ctx, guid)
	return s, err
}

// UpdateScheduledTransaction replaces a scheduled transaction's fields and
// splits. Returns app.ErrScheduleNotFound if the GUID is unknown.
func (r *Repository) UpdateScheduledTransaction(ctx context.Context, s domain.ScheduledTransaction) (domain.ScheduledTransaction, error) {
	return s, pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx,
			`UPDATE scheduled_transactions
			    SET name=$2, description=$3, enabled=$4, currency_guid=$5, period=$6, every=$7,
			        start_date=$8, end_date=$9, last_posted=$10
			  WHERE guid=$1`,
			s.GUID, s.Name, s.Description, boolToInt(s.Enabled),
			s.CurrencyGUID, string(s.Period), s.Every,
			schedDate(s.StartDate), schedDate(s.EndDate), schedDate(s.LastPostedDate),
		)
		if err != nil {
			return fmt.Errorf("update scheduled transaction: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return app.ErrScheduleNotFound
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM scheduled_splits WHERE schedtx_guid = $1`, s.GUID,
		); err != nil {
			return fmt.Errorf("delete old scheduled splits: %w", err)
		}
		return insertScheduledSplits(ctx, tx, s.GUID, s.Splits)
	})
}

// DeleteScheduledTransaction removes a scheduled transaction and its splits.
// Returns app.ErrScheduleNotFound if the GUID is unknown.
func (r *Repository) DeleteScheduledTransaction(ctx context.Context, guid string) error {
	ct, err := r.pool.Exec(ctx,
		`DELETE FROM scheduled_transactions WHERE guid = $1`, guid)
	if err != nil {
		return fmt.Errorf("delete scheduled transaction: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return app.ErrScheduleNotFound
	}
	return nil
}

// BookGUIDForSchedule returns the book a schedule belongs to, for authz checks.
func (r *Repository) BookGUIDForSchedule(ctx context.Context, guid string) (string, error) {
	var bookGUID string
	err := r.pool.QueryRow(ctx,
		`SELECT book_guid FROM scheduled_transactions WHERE guid = $1`, guid,
	).Scan(&bookGUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", app.ErrScheduleNotFound
	}
	if err != nil {
		return "", fmt.Errorf("book for schedule: %w", err)
	}
	return bookGUID, nil
}

// MarkSchedulePosted updates last_posted to date for the given schedule.
func (r *Repository) MarkSchedulePosted(ctx context.Context, guid string, date time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE scheduled_transactions SET last_posted = $2 WHERE guid = $1`,
		guid, schedDate(date),
	)
	return err
}

// scanScheduledTransaction scans one row from scheduled_transactions.
type scannable interface {
	Scan(dest ...any) error
}

func scanScheduledTransaction(row scannable) (domain.ScheduledTransaction, error) {
	var (
		s          domain.ScheduledTransaction
		enabled    int
		period     string
		startDate  *string
		endDate    *string
		lastPosted *string
	)
	if err := row.Scan(
		&s.GUID, &s.BookGUID, &s.Name, &s.Description, &enabled, &s.CurrencyGUID,
		&period, &s.Every, &startDate, &endDate, &lastPosted,
	); err != nil {
		return domain.ScheduledTransaction{}, err
	}
	s.Enabled = enabled != 0
	s.Period = domain.RecurrencePeriod(period)
	s.StartDate = parseSchedDate(startDate)
	s.EndDate = parseSchedDate(endDate)
	s.LastPostedDate = parseSchedDate(lastPosted)
	return s, nil
}

func (r *Repository) loadScheduledSplits(ctx context.Context, schedGUID string) ([]domain.ScheduledSplit, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT guid, account_guid, memo, value_num, value_denom
		   FROM scheduled_splits WHERE schedtx_guid = $1 ORDER BY guid`, schedGUID)
	if err != nil {
		return nil, fmt.Errorf("load scheduled splits: %w", err)
	}
	defer rows.Close()

	var splits []domain.ScheduledSplit
	for rows.Next() {
		var (
			sp             domain.ScheduledSplit
			valueNum, valD int64
		)
		if err := rows.Scan(&sp.GUID, &sp.AccountGUID, &sp.Memo, &valueNum, &valD); err != nil {
			return nil, fmt.Errorf("scan scheduled split: %w", err)
		}
		if sp.Value, err = domain.FromNumDenom(valueNum, valD); err != nil {
			return nil, fmt.Errorf("scheduled split %s value: %w", sp.GUID, err)
		}
		splits = append(splits, sp)
	}
	return splits, rows.Err()
}

// boolToInt maps Go bools to the 0/1 INTEGER columns GnuCash uses for flags
// like hidden and placeholder.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// CreateBudget inserts a budget and all its amounts in one transaction.
func (r *Repository) CreateBudget(ctx context.Context, b domain.Budget) (domain.Budget, error) {
	return b, pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO budgets (guid, book_guid, name, description, period_type, num_periods, start_date)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			b.GUID, b.BookGUID, b.Name, b.Description, string(b.PeriodType), b.NumPeriods,
			schedDate(b.StartDate),
		); err != nil {
			return fmt.Errorf("insert budget: %w", err)
		}
		return insertBudgetAmounts(ctx, tx, b.GUID, b.Amounts)
	})
}

func insertBudgetAmounts(ctx context.Context, tx pgx.Tx, budgetGUID string, amounts []domain.BudgetAmount) error {
	for _, amt := range amounts {
		vNum, vDenom, err := amt.Value.NumDenom()
		if err != nil {
			return fmt.Errorf("budget amount for %s: %w", amt.AccountGUID, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO budget_amounts (budget_guid, account_guid, period_num, value_num, value_denom)
			 VALUES ($1,$2,$3,$4,$5)
			 ON CONFLICT (budget_guid, account_guid, period_num)
			 DO UPDATE SET value_num = EXCLUDED.value_num, value_denom = EXCLUDED.value_denom`,
			budgetGUID, amt.AccountGUID, amt.PeriodNum, vNum, vDenom,
		); err != nil {
			return fmt.Errorf("upsert budget amount: %w", err)
		}
	}
	return nil
}

// ListBudgets returns all budgets for a book without amounts.
func (r *Repository) ListBudgets(ctx context.Context, bookGUID string) ([]domain.Budget, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT guid, book_guid, name, description, period_type, num_periods, start_date
		   FROM budgets WHERE book_guid = $1 ORDER BY name`, bookGUID)
	if err != nil {
		return nil, fmt.Errorf("list budgets: %w", err)
	}
	defer rows.Close()
	var budgets []domain.Budget
	for rows.Next() {
		b, err := scanBudget(rows)
		if err != nil {
			return nil, err
		}
		budgets = append(budgets, b)
	}
	return budgets, rows.Err()
}

// GetBudget returns a budget with its amounts, or app.ErrBudgetNotFound.
func (r *Repository) GetBudget(ctx context.Context, guid string) (domain.Budget, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT guid, book_guid, name, description, period_type, num_periods, start_date
		   FROM budgets WHERE guid = $1`, guid)
	b, err := scanBudget(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Budget{}, domain.ErrBudgetNotFound
	}
	if err != nil {
		return domain.Budget{}, err
	}
	b.Amounts, err = r.loadBudgetAmounts(ctx, guid)
	return b, err
}

// UpdateBudget replaces a budget's fields and amounts.
func (r *Repository) UpdateBudget(ctx context.Context, b domain.Budget) (domain.Budget, error) {
	return b, pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx,
			`UPDATE budgets SET name=$2, description=$3, period_type=$4, num_periods=$5, start_date=$6
			  WHERE guid=$1`,
			b.GUID, b.Name, b.Description, string(b.PeriodType), b.NumPeriods, schedDate(b.StartDate),
		)
		if err != nil {
			return fmt.Errorf("update budget: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return domain.ErrBudgetNotFound
		}
		// Delete existing amounts and re-insert so stale entries are removed.
		if _, err := tx.Exec(ctx,
			`DELETE FROM budget_amounts WHERE budget_guid = $1`, b.GUID,
		); err != nil {
			return fmt.Errorf("clear budget amounts: %w", err)
		}
		return insertBudgetAmounts(ctx, tx, b.GUID, b.Amounts)
	})
}

// DeleteBudget removes a budget and its amounts.
func (r *Repository) DeleteBudget(ctx context.Context, guid string) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM budgets WHERE guid = $1`, guid)
	if err != nil {
		return fmt.Errorf("delete budget: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrBudgetNotFound
	}
	return nil
}

// BookGUIDForBudget returns the book a budget belongs to.
func (r *Repository) BookGUIDForBudget(ctx context.Context, guid string) (string, error) {
	var bookGUID string
	err := r.pool.QueryRow(ctx,
		`SELECT book_guid FROM budgets WHERE guid = $1`, guid,
	).Scan(&bookGUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrBudgetNotFound
	}
	if err != nil {
		return "", fmt.Errorf("book for budget: %w", err)
	}
	return bookGUID, nil
}

func scanBudget(row scannable) (domain.Budget, error) {
	var (
		b         domain.Budget
		period    string
		startDate *string
	)
	if err := row.Scan(
		&b.GUID, &b.BookGUID, &b.Name, &b.Description, &period, &b.NumPeriods, &startDate,
	); err != nil {
		return domain.Budget{}, err
	}
	b.PeriodType = domain.BudgetPeriodType(period)
	b.StartDate = parseSchedDate(startDate)
	return b, nil
}

func (r *Repository) loadBudgetAmounts(ctx context.Context, budgetGUID string) ([]domain.BudgetAmount, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT account_guid, period_num, value_num, value_denom
		   FROM budget_amounts WHERE budget_guid = $1 ORDER BY period_num, account_guid`, budgetGUID)
	if err != nil {
		return nil, fmt.Errorf("load budget amounts: %w", err)
	}
	defer rows.Close()
	var amounts []domain.BudgetAmount
	for rows.Next() {
		var (
			amt          domain.BudgetAmount
			vNum, vDenom int64
		)
		if err := rows.Scan(&amt.AccountGUID, &amt.PeriodNum, &vNum, &vDenom); err != nil {
			return nil, fmt.Errorf("scan budget amount: %w", err)
		}
		val, err := domain.FromNumDenom(vNum, vDenom)
		if err != nil {
			return nil, fmt.Errorf("budget amount value: %w", err)
		}
		amt.Value = val
		amounts = append(amounts, amt)
	}
	return amounts, rows.Err()
}

// ── Customers ────────────────────────────────────────────────────────────────

func (r *Repository) ListCustomers(ctx context.Context, bookGUID string, activeOnly bool) ([]domain.Customer, error) {
	q := `SELECT guid, book_guid, name, id, notes, active, currency_guid,
		         addr_name, addr_addr1, addr_addr2, addr_phone, addr_email,
		         credit_num, credit_denom, COALESCE(terms_guid,''), created_at
		    FROM customers WHERE book_guid = $1`
	args := []any{bookGUID}
	if activeOnly {
		q += " AND active = TRUE"
	}
	q += " ORDER BY name"
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list customers: %w", err)
	}
	defer rows.Close()
	var out []domain.Customer
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) GetCustomer(ctx context.Context, guid string) (domain.Customer, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT guid, book_guid, name, id, notes, active, currency_guid,
		        addr_name, addr_addr1, addr_addr2, addr_phone, addr_email,
		        credit_num, credit_denom, COALESCE(terms_guid,''), created_at
		   FROM customers WHERE guid = $1`, guid)
	c, err := scanCustomer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Customer{}, domain.ErrCustomerNotFound
	}
	return c, err
}

func (r *Repository) CreateCustomer(ctx context.Context, c domain.Customer) error {
	cNum, cDenom, err := c.CreditLimit.NumDenom()
	if err != nil {
		return fmt.Errorf("credit limit: %w", err)
	}
	var termsGUID *string
	if c.TermsGUID != "" {
		termsGUID = &c.TermsGUID
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO customers
		   (guid, book_guid, name, id, notes, active, currency_guid,
		    addr_name, addr_addr1, addr_addr2, addr_phone, addr_email,
		    credit_num, credit_denom, terms_guid, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		c.GUID, c.BookGUID, c.Name, c.ID, c.Notes, c.Active, c.CurrencyGUID,
		c.Addr.Name, c.Addr.Addr1, c.Addr.Addr2, c.Addr.Phone, c.Addr.Email,
		cNum, cDenom, termsGUID, c.CreatedAt,
	)
	return err
}

func (r *Repository) UpdateCustomer(ctx context.Context, c domain.Customer) error {
	cNum, cDenom, err := c.CreditLimit.NumDenom()
	if err != nil {
		return fmt.Errorf("credit limit: %w", err)
	}
	var termsGUID *string
	if c.TermsGUID != "" {
		termsGUID = &c.TermsGUID
	}
	ct, err := r.pool.Exec(ctx,
		`UPDATE customers SET
		   name=$2, id=$3, notes=$4, active=$5, currency_guid=$6,
		   addr_name=$7, addr_addr1=$8, addr_addr2=$9, addr_phone=$10, addr_email=$11,
		   credit_num=$12, credit_denom=$13, terms_guid=$14
		 WHERE guid=$1`,
		c.GUID, c.Name, c.ID, c.Notes, c.Active, c.CurrencyGUID,
		c.Addr.Name, c.Addr.Addr1, c.Addr.Addr2, c.Addr.Phone, c.Addr.Email,
		cNum, cDenom, termsGUID,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrCustomerNotFound
	}
	return nil
}

func (r *Repository) DeleteCustomer(ctx context.Context, guid string) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM customers WHERE guid = $1`, guid)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrCustomerNotFound
	}
	return nil
}

type customerScanner interface {
	Scan(dest ...any) error
}

func scanCustomer(s customerScanner) (domain.Customer, error) {
	var (
		c            domain.Customer
		cNum, cDenom int64
	)
	err := s.Scan(
		&c.GUID, &c.BookGUID, &c.Name, &c.ID, &c.Notes, &c.Active, &c.CurrencyGUID,
		&c.Addr.Name, &c.Addr.Addr1, &c.Addr.Addr2, &c.Addr.Phone, &c.Addr.Email,
		&cNum, &cDenom, &c.TermsGUID, &c.CreatedAt,
	)
	if err != nil {
		return domain.Customer{}, err
	}
	c.CreditLimit, err = domain.FromNumDenom(cNum, cDenom)
	return c, err
}

// ── Vendors ──────────────────────────────────────────────────────────────────

func (r *Repository) ListVendors(ctx context.Context, bookGUID string, activeOnly bool) ([]domain.Vendor, error) {
	q := `SELECT guid, book_guid, name, id, notes, active, currency_guid,
		         addr_name, addr_addr1, addr_addr2, addr_phone, addr_email,
		         COALESCE(terms_guid,''), created_at
		    FROM vendors WHERE book_guid = $1`
	args := []any{bookGUID}
	if activeOnly {
		q += " AND active = TRUE"
	}
	q += " ORDER BY name"
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list vendors: %w", err)
	}
	defer rows.Close()
	var out []domain.Vendor
	for rows.Next() {
		v, err := scanVendor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) GetVendor(ctx context.Context, guid string) (domain.Vendor, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT guid, book_guid, name, id, notes, active, currency_guid,
		        addr_name, addr_addr1, addr_addr2, addr_phone, addr_email,
		        COALESCE(terms_guid,''), created_at
		   FROM vendors WHERE guid = $1`, guid)
	v, err := scanVendor(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Vendor{}, domain.ErrVendorNotFound
	}
	return v, err
}

func (r *Repository) CreateVendor(ctx context.Context, v domain.Vendor) error {
	var termsGUID *string
	if v.TermsGUID != "" {
		termsGUID = &v.TermsGUID
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO vendors
		   (guid, book_guid, name, id, notes, active, currency_guid,
		    addr_name, addr_addr1, addr_addr2, addr_phone, addr_email,
		    terms_guid, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		v.GUID, v.BookGUID, v.Name, v.ID, v.Notes, v.Active, v.CurrencyGUID,
		v.Addr.Name, v.Addr.Addr1, v.Addr.Addr2, v.Addr.Phone, v.Addr.Email,
		termsGUID, v.CreatedAt,
	)
	return err
}

func (r *Repository) UpdateVendor(ctx context.Context, v domain.Vendor) error {
	var termsGUID *string
	if v.TermsGUID != "" {
		termsGUID = &v.TermsGUID
	}
	ct, err := r.pool.Exec(ctx,
		`UPDATE vendors SET
		   name=$2, id=$3, notes=$4, active=$5, currency_guid=$6,
		   addr_name=$7, addr_addr1=$8, addr_addr2=$9, addr_phone=$10, addr_email=$11,
		   terms_guid=$12
		 WHERE guid=$1`,
		v.GUID, v.Name, v.ID, v.Notes, v.Active, v.CurrencyGUID,
		v.Addr.Name, v.Addr.Addr1, v.Addr.Addr2, v.Addr.Phone, v.Addr.Email,
		termsGUID,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrVendorNotFound
	}
	return nil
}

func (r *Repository) DeleteVendor(ctx context.Context, guid string) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM vendors WHERE guid = $1`, guid)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrVendorNotFound
	}
	return nil
}

type vendorScanner interface {
	Scan(dest ...any) error
}

func scanVendor(s vendorScanner) (domain.Vendor, error) {
	var v domain.Vendor
	err := s.Scan(
		&v.GUID, &v.BookGUID, &v.Name, &v.ID, &v.Notes, &v.Active, &v.CurrencyGUID,
		&v.Addr.Name, &v.Addr.Addr1, &v.Addr.Addr2, &v.Addr.Phone, &v.Addr.Email,
		&v.TermsGUID, &v.CreatedAt,
	)
	return v, err
}

// ── Employees ────────────────────────────────────────────────────────────────

const employeeCols = `guid, book_guid, name, username, id, notes, active, currency_guid,
		addr_name, addr_addr1, addr_addr2, addr_phone, addr_email,
		rate_num, rate_denom, created_at`

func (r *Repository) ListEmployees(ctx context.Context, bookGUID string, activeOnly bool) ([]domain.Employee, error) {
	q := `SELECT ` + employeeCols + ` FROM employees WHERE book_guid = $1`
	args := []any{bookGUID}
	if activeOnly {
		q += " AND active = TRUE"
	}
	q += " ORDER BY name"
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list employees: %w", err)
	}
	defer rows.Close()
	var out []domain.Employee
	for rows.Next() {
		e, err := scanEmployee(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) GetEmployee(ctx context.Context, guid string) (domain.Employee, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+employeeCols+` FROM employees WHERE guid = $1`, guid)
	e, err := scanEmployee(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Employee{}, domain.ErrEmployeeNotFound
	}
	return e, err
}

func (r *Repository) CreateEmployee(ctx context.Context, e domain.Employee) error {
	rNum, rDenom, err := e.Rate.NumDenom()
	if err != nil {
		return fmt.Errorf("rate: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO employees
		   (guid, book_guid, name, username, id, notes, active, currency_guid,
		    addr_name, addr_addr1, addr_addr2, addr_phone, addr_email,
		    rate_num, rate_denom, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		e.GUID, e.BookGUID, e.Name, e.Username, e.ID, e.Notes, e.Active, e.CurrencyGUID,
		e.Addr.Name, e.Addr.Addr1, e.Addr.Addr2, e.Addr.Phone, e.Addr.Email,
		rNum, rDenom, e.CreatedAt,
	)
	return err
}

func (r *Repository) UpdateEmployee(ctx context.Context, e domain.Employee) error {
	rNum, rDenom, err := e.Rate.NumDenom()
	if err != nil {
		return fmt.Errorf("rate: %w", err)
	}
	ct, err := r.pool.Exec(ctx,
		`UPDATE employees SET
		   name=$2, username=$3, id=$4, notes=$5, active=$6, currency_guid=$7,
		   addr_name=$8, addr_addr1=$9, addr_addr2=$10, addr_phone=$11, addr_email=$12,
		   rate_num=$13, rate_denom=$14
		 WHERE guid=$1`,
		e.GUID, e.Name, e.Username, e.ID, e.Notes, e.Active, e.CurrencyGUID,
		e.Addr.Name, e.Addr.Addr1, e.Addr.Addr2, e.Addr.Phone, e.Addr.Email,
		rNum, rDenom,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrEmployeeNotFound
	}
	return nil
}

func (r *Repository) DeleteEmployee(ctx context.Context, guid string) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM employees WHERE guid = $1`, guid)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrEmployeeNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEmployee(s rowScanner) (domain.Employee, error) {
	var (
		e            domain.Employee
		rNum, rDenom int64
	)
	err := s.Scan(
		&e.GUID, &e.BookGUID, &e.Name, &e.Username, &e.ID, &e.Notes, &e.Active, &e.CurrencyGUID,
		&e.Addr.Name, &e.Addr.Addr1, &e.Addr.Addr2, &e.Addr.Phone, &e.Addr.Email,
		&rNum, &rDenom, &e.CreatedAt,
	)
	if err != nil {
		return domain.Employee{}, err
	}
	e.Rate, err = domain.FromNumDenom(rNum, rDenom)
	return e, err
}

// ── Jobs ─────────────────────────────────────────────────────────────────────

const jobCols = `guid, book_guid, name, id, reference, active, owner_type, owner_guid, created_at`

func (r *Repository) ListJobs(ctx context.Context, bookGUID string, activeOnly bool) ([]domain.Job, error) {
	q := `SELECT ` + jobCols + ` FROM jobs WHERE book_guid = $1`
	args := []any{bookGUID}
	if activeOnly {
		q += " AND active = TRUE"
	}
	q += " ORDER BY name"
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()
	var out []domain.Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (r *Repository) GetJob(ctx context.Context, guid string) (domain.Job, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+jobCols+` FROM jobs WHERE guid = $1`, guid)
	j, err := scanJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Job{}, domain.ErrJobNotFound
	}
	return j, err
}

func (r *Repository) CreateJob(ctx context.Context, j domain.Job) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO jobs
		   (guid, book_guid, name, id, reference, active, owner_type, owner_guid, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		j.GUID, j.BookGUID, j.Name, j.ID, j.Reference, j.Active, j.OwnerType, j.OwnerGUID, j.CreatedAt,
	)
	return err
}

func (r *Repository) UpdateJob(ctx context.Context, j domain.Job) error {
	ct, err := r.pool.Exec(ctx,
		`UPDATE jobs SET name=$2, id=$3, reference=$4, active=$5 WHERE guid=$1`,
		j.GUID, j.Name, j.ID, j.Reference, j.Active,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrJobNotFound
	}
	return nil
}

func (r *Repository) DeleteJob(ctx context.Context, guid string) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM jobs WHERE guid = $1`, guid)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrJobNotFound
	}
	return nil
}

func scanJob(s rowScanner) (domain.Job, error) {
	var j domain.Job
	err := s.Scan(
		&j.GUID, &j.BookGUID, &j.Name, &j.ID, &j.Reference, &j.Active,
		&j.OwnerType, &j.OwnerGUID, &j.CreatedAt,
	)
	return j, err
}

// ── Invoices ─────────────────────────────────────────────────────────────────

const invoiceCols = `guid, book_guid, id, type, owner_guid,
	       date_opened, date_posted, date_due, notes, active, currency_guid,
	       COALESCE(post_txn_guid,''), COALESCE(post_acc_guid,''), COALESCE(terms_guid,''),
	       paid_at, COALESCE(paid_txn_guid,''), created_at`

func (r *Repository) ListInvoices(ctx context.Context, bookGUID, invoiceType string) ([]domain.Invoice, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+invoiceCols+`
		FROM invoices
		WHERE book_guid=$1 AND type=$2
		ORDER BY date_opened DESC, created_at DESC`, bookGUID, invoiceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Invoice
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}

	// Batch-load entries so callers can compute totals without N+1 queries.
	guids := make([]string, len(out))
	for i, inv := range out {
		guids[i] = inv.GUID
	}
	entryRows, err := r.pool.Query(ctx, `
		SELECT guid, invoice_guid, date, description, action, notes,
		       quantity_num, quantity_denom, account_guid, price_num, price_denom,
		       taxable, tax_table_guid, created_at
		FROM entries WHERE invoice_guid = ANY($1)`, guids)
	if err != nil {
		return nil, err
	}
	defer entryRows.Close()
	byInvoice := make(map[string][]domain.InvoiceEntry)
	for entryRows.Next() {
		e, err := scanEntry(entryRows)
		if err != nil {
			return nil, err
		}
		byInvoice[e.InvoiceGUID] = append(byInvoice[e.InvoiceGUID], e)
	}
	if err := entryRows.Err(); err != nil {
		return nil, err
	}
	for i, inv := range out {
		out[i].Entries = byInvoice[inv.GUID]
	}
	return out, nil
}

func (r *Repository) CreateInvoice(ctx context.Context, inv domain.Invoice) error {
	var termsGUID *string
	if inv.TermsGUID != "" {
		termsGUID = &inv.TermsGUID
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO invoices
		  (guid, book_guid, id, type, owner_guid,
		   date_opened, notes, active, currency_guid, terms_guid)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		inv.GUID, inv.BookGUID, inv.ID, string(inv.Type), inv.OwnerGUID,
		inv.DateOpened.Format("2006-01-02"), inv.Notes, inv.Active, inv.CurrencyGUID, termsGUID)
	return err
}

func (r *Repository) GetInvoice(ctx context.Context, guid string) (domain.Invoice, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+invoiceCols+` FROM invoices WHERE guid=$1`, guid)
	inv, err := scanInvoice(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Invoice{}, domain.ErrInvoiceNotFound
	}
	return inv, err
}

// GetBillTerm returns a payment term by GUID, or domain.ErrBillTermNotFound.
func (r *Repository) GetBillTerm(ctx context.Context, guid string) (domain.BillTerm, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+billTermCols+` FROM billterms WHERE guid=$1`, guid)
	t, err := scanBillTerm(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.BillTerm{}, domain.ErrBillTermNotFound
	}
	return t, err
}

const billTermCols = `guid, book_guid, name, description, type, duedays, discountdays, discount_num, discount_denom, cutoff`

func scanBillTerm(row pgx.Row) (domain.BillTerm, error) {
	var (
		t         domain.BillTerm
		typ       string
		discNum   int64
		discDenom int64
	)
	if err := row.Scan(&t.GUID, &t.BookGUID, &t.Name, &t.Description, &typ,
		&t.DueDays, &t.DiscountDays, &discNum, &discDenom, &t.Cutoff); err != nil {
		return domain.BillTerm{}, err
	}
	t.Type = domain.BillTermType(typ)
	disc, err := domain.FromNumDenom(discNum, discDenom)
	if err != nil {
		return domain.BillTerm{}, err
	}
	t.Discount = disc
	return t, nil
}

// CreateBillTerm inserts a new payment term.
func (r *Repository) CreateBillTerm(ctx context.Context, t domain.BillTerm) (domain.BillTerm, error) {
	discNum, discDenom, err := t.Discount.NumDenom()
	if err != nil {
		return domain.BillTerm{}, fmt.Errorf("discount: %w", err)
	}
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO billterms (guid, book_guid, name, description, type, duedays, discountdays, discount_num, discount_denom, cutoff)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		t.GUID, t.BookGUID, t.Name, t.Description, string(t.Type),
		t.DueDays, t.DiscountDays, discNum, discDenom, t.Cutoff); err != nil {
		return domain.BillTerm{}, fmt.Errorf("insert bill term: %w", err)
	}
	return t, nil
}

// ListBillTerms returns a book's payment terms, ordered by name.
func (r *Repository) ListBillTerms(ctx context.Context, bookGUID string) ([]domain.BillTerm, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+billTermCols+` FROM billterms WHERE book_guid=$1 ORDER BY name`, bookGUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var terms []domain.BillTerm
	for rows.Next() {
		t, err := scanBillTerm(rows)
		if err != nil {
			return nil, err
		}
		terms = append(terms, t)
	}
	return terms, rows.Err()
}

// UpdateBillTerm replaces a payment term's fields, or domain.ErrBillTermNotFound.
func (r *Repository) UpdateBillTerm(ctx context.Context, t domain.BillTerm) (domain.BillTerm, error) {
	discNum, discDenom, err := t.Discount.NumDenom()
	if err != nil {
		return domain.BillTerm{}, fmt.Errorf("discount: %w", err)
	}
	ct, err := r.pool.Exec(ctx, `
		UPDATE billterms SET
		  name=$2, description=$3, type=$4, duedays=$5, discountdays=$6,
		  discount_num=$7, discount_denom=$8, cutoff=$9
		WHERE guid=$1`,
		t.GUID, t.Name, t.Description, string(t.Type), t.DueDays, t.DiscountDays,
		discNum, discDenom, t.Cutoff)
	if err != nil {
		return domain.BillTerm{}, err
	}
	if ct.RowsAffected() == 0 {
		return domain.BillTerm{}, domain.ErrBillTermNotFound
	}
	return t, nil
}

// DeleteBillTerm removes a payment term, or domain.ErrBillTermNotFound.
func (r *Repository) DeleteBillTerm(ctx context.Context, guid string) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM billterms WHERE guid=$1`, guid)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrBillTermNotFound
	}
	return nil
}

// BookGUIDForBillTerm returns the book a payment term belongs to, for authz.
func (r *Repository) BookGUIDForBillTerm(ctx context.Context, guid string) (string, error) {
	var bookGUID string
	err := r.pool.QueryRow(ctx, `SELECT book_guid FROM billterms WHERE guid=$1`, guid).Scan(&bookGUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrBillTermNotFound
	}
	return bookGUID, err
}

// loadTaxTableEntries returns a tax table's entries, ordered by creation.
func (r *Repository) loadTaxTableEntries(ctx context.Context, taxtableGUID string) ([]domain.TaxTableEntry, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT account_guid, amount_num, amount_denom, type
		 FROM taxtable_entries WHERE taxtable_guid=$1 ORDER BY created_at`, taxtableGUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []domain.TaxTableEntry
	for rows.Next() {
		var (
			e              domain.TaxTableEntry
			amtNum, amtDen int64
			typ            string
		)
		if err := rows.Scan(&e.AccountGUID, &amtNum, &amtDen, &typ); err != nil {
			return nil, err
		}
		if e.Amount, err = domain.FromNumDenom(amtNum, amtDen); err != nil {
			return nil, err
		}
		e.Type = domain.TaxEntryType(typ)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetTaxTable returns a tax table with its entries, or domain.ErrTaxTableNotFound.
func (r *Repository) GetTaxTable(ctx context.Context, guid string) (domain.TaxTable, error) {
	var tt domain.TaxTable
	err := r.pool.QueryRow(ctx,
		`SELECT guid, book_guid, name FROM taxtables WHERE guid=$1`, guid).
		Scan(&tt.GUID, &tt.BookGUID, &tt.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.TaxTable{}, domain.ErrTaxTableNotFound
	}
	if err != nil {
		return domain.TaxTable{}, err
	}
	if tt.Entries, err = r.loadTaxTableEntries(ctx, guid); err != nil {
		return domain.TaxTable{}, err
	}
	return tt, nil
}

// ListTaxTables returns a book's tax tables (with entries), ordered by name.
func (r *Repository) ListTaxTables(ctx context.Context, bookGUID string) ([]domain.TaxTable, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT guid, book_guid, name FROM taxtables WHERE book_guid=$1 ORDER BY name`, bookGUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []domain.TaxTable
	for rows.Next() {
		var tt domain.TaxTable
		if err := rows.Scan(&tt.GUID, &tt.BookGUID, &tt.Name); err != nil {
			return nil, err
		}
		tables = append(tables, tt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range tables {
		if tables[i].Entries, err = r.loadTaxTableEntries(ctx, tables[i].GUID); err != nil {
			return nil, err
		}
	}
	return tables, nil
}

func (r *Repository) insertTaxTableEntries(ctx context.Context, tx pgx.Tx, taxtableGUID string, entries []domain.TaxTableEntry) error {
	for _, e := range entries {
		num, den, err := e.Amount.NumDenom()
		if err != nil {
			return fmt.Errorf("tax entry amount: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO taxtable_entries (guid, taxtable_guid, account_guid, amount_num, amount_denom, type)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			app.NewGUID(), taxtableGUID, e.AccountGUID, num, den, string(e.Type)); err != nil {
			return fmt.Errorf("insert tax entry: %w", err)
		}
	}
	return nil
}

// CreateTaxTable inserts a tax table and its entries in one transaction.
func (r *Repository) CreateTaxTable(ctx context.Context, tt domain.TaxTable) (domain.TaxTable, error) {
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO taxtables (guid, book_guid, name) VALUES ($1, $2, $3)`,
			tt.GUID, tt.BookGUID, tt.Name); err != nil {
			return fmt.Errorf("insert tax table: %w", err)
		}
		return r.insertTaxTableEntries(ctx, tx, tt.GUID, tt.Entries)
	})
	if err != nil {
		return domain.TaxTable{}, err
	}
	return tt, nil
}

// UpdateTaxTable replaces a tax table's name and entries, or domain.ErrTaxTableNotFound.
func (r *Repository) UpdateTaxTable(ctx context.Context, tt domain.TaxTable) (domain.TaxTable, error) {
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx, `UPDATE taxtables SET name=$2 WHERE guid=$1`, tt.GUID, tt.Name)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return domain.ErrTaxTableNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM taxtable_entries WHERE taxtable_guid=$1`, tt.GUID); err != nil {
			return err
		}
		return r.insertTaxTableEntries(ctx, tx, tt.GUID, tt.Entries)
	})
	if err != nil {
		return domain.TaxTable{}, err
	}
	return tt, nil
}

// DeleteTaxTable removes a tax table and its entries, or domain.ErrTaxTableNotFound.
func (r *Repository) DeleteTaxTable(ctx context.Context, guid string) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM taxtables WHERE guid=$1`, guid)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrTaxTableNotFound
	}
	return nil
}

// BookGUIDForTaxTable returns the book a tax table belongs to, for authz.
func (r *Repository) BookGUIDForTaxTable(ctx context.Context, guid string) (string, error) {
	var bookGUID string
	err := r.pool.QueryRow(ctx, `SELECT book_guid FROM taxtables WHERE guid=$1`, guid).Scan(&bookGUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrTaxTableNotFound
	}
	return bookGUID, err
}

func (r *Repository) UpdateInvoice(ctx context.Context, inv domain.Invoice) error {
	var termsGUID *string
	if inv.TermsGUID != "" {
		termsGUID = &inv.TermsGUID
	}
	ct, err := r.pool.Exec(ctx, `
		UPDATE invoices SET
		  id=$2, owner_guid=$3,
		  date_opened=$4, date_due=$5, notes=$6, active=$7,
		  currency_guid=$8, terms_guid=$9
		WHERE guid=$1 AND date_posted IS NULL`,
		inv.GUID, inv.ID, inv.OwnerGUID,
		inv.DateOpened.Format("2006-01-02"), inv.DateDue, inv.Notes, inv.Active,
		inv.CurrencyGUID, termsGUID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrInvoiceNotFound
	}
	return nil
}

func (r *Repository) DeleteInvoice(ctx context.Context, guid string) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM invoices WHERE guid=$1 AND date_posted IS NULL`, guid)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrInvoiceNotFound
	}
	return nil
}

func (r *Repository) MarkInvoicePosted(ctx context.Context, guid, txnGUID, accGUID string, datePosted, dateDue *time.Time) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE invoices SET
		  date_posted=$2, date_due=$3, post_txn_guid=$4, post_acc_guid=$5
		WHERE guid=$1 AND date_posted IS NULL`,
		guid, datePosted, dateDue, txnGUID, accGUID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrInvoiceNotFound
	}
	return nil
}

type invoiceScanner interface {
	Scan(dest ...any) error
}

func scanInvoice(s invoiceScanner) (domain.Invoice, error) {
	var inv domain.Invoice
	var invType string
	var datePosted, dateDue, paidAt *time.Time
	err := s.Scan(
		&inv.GUID, &inv.BookGUID, &inv.ID, &invType, &inv.OwnerGUID,
		&inv.DateOpened, &datePosted, &dateDue, &inv.Notes, &inv.Active, &inv.CurrencyGUID,
		&inv.PostTxnGUID, &inv.PostAccGUID, &inv.TermsGUID,
		&paidAt, &inv.PaidTxnGUID, &inv.CreatedAt,
	)
	if err != nil {
		return domain.Invoice{}, err
	}
	inv.Type = domain.InvoiceType(invType)
	inv.DatePosted = datePosted
	inv.DateDue = dateDue
	inv.PaidAt = paidAt
	return inv, nil
}

func (r *Repository) MarkInvoicePaid(ctx context.Context, guid, txnGUID string, paidAt time.Time) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE invoices SET paid_at=$2, paid_txn_guid=$3
		WHERE guid=$1 AND date_posted IS NOT NULL AND paid_at IS NULL`,
		guid, paidAt, txnGUID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrInvoiceNotFound
	}
	return nil
}

func (r *Repository) ARAgingRows(ctx context.Context, bookGUID string) ([]app.AgingRow, error) {
	return r.agingRows(ctx, bookGUID, "invoice")
}

func (r *Repository) APAgingRows(ctx context.Context, bookGUID string) ([]app.AgingRow, error) {
	return r.agingRows(ctx, bookGUID, "bill")
}

func (r *Repository) agingRows(ctx context.Context, bookGUID, invType string) ([]app.AgingRow, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+invoiceCols+`
		FROM invoices
		WHERE book_guid=$1 AND type=$2 AND date_posted IS NOT NULL AND paid_at IS NULL
		ORDER BY date_posted`, bookGUID, invType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var invoices []domain.Invoice
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		invoices = append(invoices, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(invoices) == 0 {
		return nil, nil
	}

	// Load all entries for these invoices in one query.
	guids := make([]string, len(invoices))
	for i, inv := range invoices {
		guids[i] = inv.GUID
	}
	entryRows, err := r.pool.Query(ctx, `
		SELECT guid, invoice_guid, date, description, action, notes,
		       quantity_num, quantity_denom, account_guid, price_num, price_denom,
		       taxable, tax_table_guid, created_at
		FROM entries WHERE invoice_guid = ANY($1)`, guids)
	if err != nil {
		return nil, err
	}
	defer entryRows.Close()
	byInvoice := make(map[string][]domain.InvoiceEntry)
	for entryRows.Next() {
		e, err := scanEntry(entryRows)
		if err != nil {
			return nil, err
		}
		byInvoice[e.InvoiceGUID] = append(byInvoice[e.InvoiceGUID], e)
	}
	if err := entryRows.Err(); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	result := make([]app.AgingRow, 0, len(invoices))
	for _, inv := range invoices {
		total := domain.Zero()
		for _, e := range byInvoice[inv.GUID] {
			total = total.Add(e.LineTotal())
		}
		var dueDate time.Time
		if inv.DateDue != nil {
			dueDate = *inv.DateDue
		} else if inv.DatePosted != nil {
			dueDate = inv.DatePosted.Add(30 * 24 * time.Hour)
		}
		daysOverdue := int(now.Sub(dueDate).Hours() / 24)
		result = append(result, app.AgingRow{
			Invoice:     inv,
			Total:       total,
			DaysOverdue: daysOverdue,
		})
	}
	return result, nil
}

// ── Entries ───────────────────────────────────────────────────────────────────

func (r *Repository) ListEntries(ctx context.Context, invoiceGUID string) ([]domain.InvoiceEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT guid, invoice_guid, date, description, action, notes,
		       quantity_num, quantity_denom, account_guid, price_num, price_denom,
		       taxable, tax_table_guid, created_at
		FROM entries WHERE invoice_guid=$1
		ORDER BY created_at`, invoiceGUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.InvoiceEntry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) CreateEntry(ctx context.Context, e domain.InvoiceEntry) error {
	qNum, qDenom, err := e.Quantity.NumDenom()
	if err != nil {
		return err
	}
	pNum, pDenom, err := e.Price.NumDenom()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO entries
		  (guid, invoice_guid, date, description, action, notes,
		   quantity_num, quantity_denom, account_guid, price_num, price_denom, taxable, tax_table_guid)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		e.GUID, e.InvoiceGUID, e.Date.Format("2006-01-02"),
		e.Description, e.Action, e.Notes,
		qNum, qDenom, e.AccountGUID, pNum, pDenom, e.Taxable, nullable(e.TaxTableGUID))
	return err
}

func (r *Repository) GetEntry(ctx context.Context, guid string) (domain.InvoiceEntry, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT guid, invoice_guid, date, description, action, notes,
		       quantity_num, quantity_denom, account_guid, price_num, price_denom,
		       taxable, tax_table_guid, created_at
		FROM entries WHERE guid=$1`, guid)
	e, err := scanEntry(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.InvoiceEntry{}, domain.ErrEntryNotFound
	}
	return e, err
}

func (r *Repository) UpdateEntry(ctx context.Context, e domain.InvoiceEntry) error {
	qNum, qDenom, err := e.Quantity.NumDenom()
	if err != nil {
		return err
	}
	pNum, pDenom, err := e.Price.NumDenom()
	if err != nil {
		return err
	}
	ct, err := r.pool.Exec(ctx, `
		UPDATE entries SET
		  date=$2, description=$3, action=$4, notes=$5,
		  quantity_num=$6, quantity_denom=$7, account_guid=$8,
		  price_num=$9, price_denom=$10, taxable=$11, tax_table_guid=$12
		WHERE guid=$1`,
		e.GUID, e.Date.Format("2006-01-02"),
		e.Description, e.Action, e.Notes,
		qNum, qDenom, e.AccountGUID, pNum, pDenom, e.Taxable, nullable(e.TaxTableGUID))
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrEntryNotFound
	}
	return nil
}

func (r *Repository) DeleteEntry(ctx context.Context, guid string) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM entries WHERE guid=$1`, guid)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrEntryNotFound
	}
	return nil
}

type entryScanner interface {
	Scan(dest ...any) error
}

func scanEntry(s entryScanner) (domain.InvoiceEntry, error) {
	var e domain.InvoiceEntry
	var qNum, qDenom, pNum, pDenom int64
	var taxTableGUID *string
	err := s.Scan(
		&e.GUID, &e.InvoiceGUID, &e.Date, &e.Description, &e.Action, &e.Notes,
		&qNum, &qDenom, &e.AccountGUID, &pNum, &pDenom,
		&e.Taxable, &taxTableGUID, &e.CreatedAt,
	)
	if err != nil {
		return domain.InvoiceEntry{}, err
	}
	if taxTableGUID != nil {
		e.TaxTableGUID = *taxTableGUID
	}
	e.Quantity, err = domain.FromNumDenom(qNum, qDenom)
	if err != nil {
		return domain.InvoiceEntry{}, err
	}
	e.Price, err = domain.FromNumDenom(pNum, pDenom)
	if err != nil {
		return domain.InvoiceEntry{}, err
	}
	return e, nil
}

// GetBookPreferences returns the stored preferences for a book. If no row
// exists yet it returns a zero-value BookPreferences, not an error.
func (r *Repository) GetBookPreferences(ctx context.Context, bookGUID string) (app.BookPreferences, error) {
	var p app.BookPreferences
	var guid *string
	err := r.pool.QueryRow(ctx,
		`SELECT default_commodity_guid FROM book_preferences WHERE book_guid = $1`, bookGUID,
	).Scan(&guid)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, fmt.Errorf("get book preferences: %w", err)
	}
	if guid != nil {
		p.DefaultCommodityGUID = *guid
	}
	return p, nil
}

// UpsertBookPreferences writes (or overwrites) the preferences for a book.
func (r *Repository) UpsertBookPreferences(ctx context.Context, bookGUID string, prefs app.BookPreferences) error {
	var guid *string
	if prefs.DefaultCommodityGUID != "" {
		guid = &prefs.DefaultCommodityGUID
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO book_preferences (book_guid, default_commodity_guid, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (book_guid) DO UPDATE
		  SET default_commodity_guid = EXCLUDED.default_commodity_guid,
		      updated_at             = EXCLUDED.updated_at`,
		bookGUID, guid)
	if err != nil {
		return fmt.Errorf("upsert book preferences: %w", err)
	}
	return nil
}
