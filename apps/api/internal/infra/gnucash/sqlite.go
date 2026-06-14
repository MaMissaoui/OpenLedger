// Package gnucash reads and writes GnuCash files, mapping them to OpenLedger's
// domain types. The schema mirrors GnuCash's SQL backend, so import is largely a
// table-by-table copy that preserves GUIDs for round-trip fidelity.
package gnucash

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	// Pure-Go SQLite driver, registered under the name "sqlite". GnuCash's
	// SQLite backend is the highest-fidelity on-disk format.
	_ "modernc.org/sqlite"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// Reader reads GnuCash SQLite files. It holds no state; a zero Reader is ready
// to use.
type Reader struct{}

// NewReader returns a GnuCash SQLite reader.
func NewReader() *Reader { return &Reader{} }

// gncTimeLayouts are the timestamp formats GnuCash's SQLite backend uses for
// post_date / enter_date. Older files use a packed form, newer ones a spaced
// form; both are UTC.
var gncTimeLayouts = []string{
	"2006-01-02 15:04:05",
	"20060102150405",
}

// ReadGnuCashSQLite opens the GnuCash SQLite database at path and reads its
// first book together with every commodity, account, and transaction (with
// splits) it contains, into domain types with GUIDs preserved. A file with no
// book yields a zero-value GnuCashData (its Book.GUID is empty), which the
// caller treats as a parse error.
func (Reader) ReadGnuCashSQLite(ctx context.Context, path string) (app.GnuCashData, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return app.GnuCashData{}, fmt.Errorf("open sqlite: %w", err)
	}
	defer func() { _ = db.Close() }()

	book, err := readBook(ctx, db)
	if err != nil {
		return app.GnuCashData{}, err
	}
	if book.GUID == "" {
		return app.GnuCashData{}, nil
	}

	commodities, err := readCommodities(ctx, db)
	if err != nil {
		return app.GnuCashData{}, err
	}
	accounts, err := readAccounts(ctx, db)
	if err != nil {
		return app.GnuCashData{}, err
	}
	transactions, err := readTransactions(ctx, db)
	if err != nil {
		return app.GnuCashData{}, err
	}

	return app.GnuCashData{
		Book:         book,
		Commodities:  commodities,
		Accounts:     accounts,
		Transactions: transactions,
	}, nil
}

func readBook(ctx context.Context, db *sql.DB) (domain.Book, error) {
	var b domain.Book
	err := db.QueryRowContext(ctx,
		`SELECT guid, root_account_guid, root_template_guid FROM books LIMIT 1`,
	).Scan(&b.GUID, &b.RootAccountGUID, &b.RootTemplateGUID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Book{}, nil
	}
	if err != nil {
		return domain.Book{}, fmt.Errorf("read book: %w", err)
	}
	return b, nil
}

func readCommodities(ctx context.Context, db *sql.DB) ([]domain.Commodity, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT guid, namespace, mnemonic, fullname, fraction FROM commodities`)
	if err != nil {
		return nil, fmt.Errorf("read commodities: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var commodities []domain.Commodity
	for rows.Next() {
		var (
			c        domain.Commodity
			fullname sql.NullString
		)
		if err := rows.Scan(&c.GUID, &c.Namespace, &c.Mnemonic, &fullname, &c.Fraction); err != nil {
			return nil, fmt.Errorf("scan commodity: %w", err)
		}
		c.Fullname = fullname.String
		commodities = append(commodities, c)
	}
	return commodities, rows.Err()
}

func readAccounts(ctx context.Context, db *sql.DB) ([]domain.Account, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT guid, name, account_type, commodity_guid, parent_guid, code, description, hidden, placeholder
		   FROM accounts`)
	if err != nil {
		return nil, fmt.Errorf("read accounts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var accounts []domain.Account
	for rows.Next() {
		var (
			a                              domain.Account
			accountType                    string
			commodity, parent, code, descr sql.NullString
			hidden, placeholder            sql.NullInt64
		)
		if err := rows.Scan(
			&a.GUID, &a.Name, &accountType, &commodity, &parent, &code, &descr, &hidden, &placeholder,
		); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		a.Type = domain.AccountType(accountType)
		a.CommodityGUID = commodity.String
		a.ParentGUID = parent.String
		a.Code = code.String
		a.Description = descr.String
		a.Hidden = hidden.Int64 != 0
		a.Placeholder = placeholder.Int64 != 0
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func readTransactions(ctx context.Context, db *sql.DB) ([]domain.Transaction, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT guid, currency_guid, num, post_date, enter_date, description
		   FROM transactions ORDER BY post_date, guid`)
	if err != nil {
		return nil, fmt.Errorf("read transactions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var (
		txns   []domain.Transaction
		byGUID = map[string]*domain.Transaction{}
	)
	for rows.Next() {
		var (
			t               domain.Transaction
			num             sql.NullString
			postDate, enter sql.NullString
			description     sql.NullString
		)
		if err := rows.Scan(&t.GUID, &t.CurrencyGUID, &num, &postDate, &enter, &description); err != nil {
			return nil, fmt.Errorf("scan transaction: %w", err)
		}
		t.Num = num.String
		t.Description = description.String
		if t.PostDate, err = parseGncTime(postDate.String); err != nil {
			return nil, fmt.Errorf("transaction %s post_date: %w", t.GUID, err)
		}
		if t.EnterDate, err = parseGncTime(enter.String); err != nil {
			return nil, fmt.Errorf("transaction %s enter_date: %w", t.GUID, err)
		}
		txns = append(txns, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Index by GUID after the slice is fully built so the pointers are stable.
	for i := range txns {
		byGUID[txns[i].GUID] = &txns[i]
	}
	if err := attachSplits(ctx, db, byGUID); err != nil {
		return nil, err
	}
	return txns, nil
}

func attachSplits(ctx context.Context, db *sql.DB, byGUID map[string]*domain.Transaction) error {
	rows, err := db.QueryContext(ctx,
		`SELECT guid, tx_guid, account_guid, memo, action, reconcile_state,
		        value_num, value_denom, quantity_num, quantity_denom, lot_guid
		   FROM splits ORDER BY tx_guid, guid`)
	if err != nil {
		return fmt.Errorf("read splits: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			s                                      domain.Split
			txGUID                                 string
			memo, action, reconcile, lot           sql.NullString
			valueNum, valueDenom, qtyNum, qtyDenom int64
		)
		if err := rows.Scan(
			&s.GUID, &txGUID, &s.AccountGUID, &memo, &action, &reconcile,
			&valueNum, &valueDenom, &qtyNum, &qtyDenom, &lot,
		); err != nil {
			return fmt.Errorf("scan split: %w", err)
		}
		s.Memo = memo.String
		s.Action = action.String
		if reconcile.String != "" {
			s.Reconcile = domain.ReconcileState([]rune(reconcile.String)[0])
		}
		s.LotGUID = lot.String
		if s.Value, err = domain.FromNumDenom(valueNum, valueDenom); err != nil {
			return fmt.Errorf("split %s value: %w", s.GUID, err)
		}
		if s.Quantity, err = domain.FromNumDenom(qtyNum, qtyDenom); err != nil {
			return fmt.Errorf("split %s quantity: %w", s.GUID, err)
		}
		// Splits whose parent transaction is absent are skipped (e.g. template
		// splits referencing the template root, which we don't import yet).
		if t, ok := byGUID[txGUID]; ok {
			t.Splits = append(t.Splits, s)
		}
	}
	return rows.Err()
}

// parseGncTime parses a GnuCash timestamp string, returning the zero time for an
// empty value. Times are interpreted as UTC.
func parseGncTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range gncTimeLayouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised timestamp %q", s)
}
