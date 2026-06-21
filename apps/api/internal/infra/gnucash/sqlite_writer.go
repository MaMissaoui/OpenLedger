package gnucash

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// Writer writes OpenLedger books out as GnuCash SQLite files. It holds no state;
// a zero Writer is ready to use.
type Writer struct{}

// NewWriter returns a GnuCash SQLite writer.
func NewWriter() *Writer { return &Writer{} }

// gncTimeLayout is the timestamp format GnuCash's SQLite backend reads; it is
// the first (canonical) layout the reader accepts. Times are written as UTC.
const gncTimeLayout = "2006-01-02 15:04:05"

// schemaDDL creates the subset of GnuCash's SQLite tables OpenLedger populates.
// Foreign keys are intentionally not declared (GnuCash enforces integrity in its
// engine, not at the SQL layer), so insertion order is unconstrained. Column
// layouts mirror GnuCash so the file is a faithful, re-importable export.
var schemaDDL = []string{
	`CREATE TABLE books (
	    guid CHAR(32) PRIMARY KEY NOT NULL,
	    root_account_guid CHAR(32) NOT NULL,
	    root_template_guid CHAR(32) NOT NULL)`,
	`CREATE TABLE commodities (
	    guid CHAR(32) PRIMARY KEY NOT NULL,
	    namespace TEXT NOT NULL,
	    mnemonic TEXT NOT NULL,
	    fullname TEXT,
	    cusip TEXT,
	    fraction INTEGER NOT NULL,
	    quote_flag INTEGER NOT NULL,
	    quote_source TEXT,
	    quote_tz TEXT)`,
	`CREATE TABLE accounts (
	    guid CHAR(32) PRIMARY KEY NOT NULL,
	    name TEXT NOT NULL,
	    account_type TEXT NOT NULL,
	    commodity_guid CHAR(32),
	    commodity_scu INTEGER NOT NULL,
	    non_std_scu INTEGER NOT NULL,
	    parent_guid CHAR(32),
	    code TEXT,
	    description TEXT,
	    hidden INTEGER,
	    placeholder INTEGER)`,
	`CREATE TABLE transactions (
	    guid CHAR(32) PRIMARY KEY NOT NULL,
	    currency_guid CHAR(32) NOT NULL,
	    num TEXT NOT NULL,
	    post_date TEXT,
	    enter_date TEXT,
	    description TEXT)`,
	`CREATE TABLE splits (
	    guid CHAR(32) PRIMARY KEY NOT NULL,
	    tx_guid CHAR(32) NOT NULL,
	    account_guid CHAR(32) NOT NULL,
	    memo TEXT NOT NULL,
	    action TEXT NOT NULL,
	    reconcile_state TEXT NOT NULL,
	    reconcile_date TEXT,
	    value_num BIGINT NOT NULL,
	    value_denom BIGINT NOT NULL,
	    quantity_num BIGINT NOT NULL,
	    quantity_denom BIGINT NOT NULL,
	    lot_guid CHAR(32))`,
	`CREATE TABLE lots (
	    guid CHAR(32) PRIMARY KEY NOT NULL,
	    account_guid CHAR(32),
	    is_closed INTEGER NOT NULL)`,
	`CREATE TABLE slots (
	    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
	    obj_guid CHAR(32) NOT NULL,
	    name TEXT NOT NULL,
	    slot_type INTEGER NOT NULL,
	    int64_val BIGINT,
	    string_val TEXT,
	    double_val FLOAT,
	    timespec_val TEXT,
	    guid_val CHAR(32),
	    numeric_val_num BIGINT,
	    numeric_val_denom BIGINT,
	    gdate_val TEXT)`,
	`CREATE TABLE schedxactions (
	    guid CHAR(32) PRIMARY KEY NOT NULL,
	    name TEXT,
	    enabled INTEGER NOT NULL,
	    start_date TEXT,
	    end_date TEXT,
	    last_occur TEXT,
	    num_occur INTEGER NOT NULL,
	    rem_occur INTEGER NOT NULL,
	    auto_create INTEGER NOT NULL,
	    auto_notify INTEGER NOT NULL,
	    advance_creation_days INTEGER NOT NULL,
	    advance_remind_days INTEGER NOT NULL,
	    instance_count INTEGER NOT NULL,
	    template_act_guid CHAR(32) NOT NULL)`,
	`CREATE TABLE recurrences (
	    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
	    obj_guid CHAR(32) NOT NULL,
	    recurrence_mult INTEGER NOT NULL,
	    recurrence_period_type TEXT NOT NULL,
	    recurrence_period_start TEXT NOT NULL,
	    recurrence_weekend_adj TEXT NOT NULL)`,
}

// WriteGnuCashSQLite creates a GnuCash SQLite database at path and writes data
// into it. Money is stored GnuCash-style: each split's value at the transaction
// currency's fraction and its quantity at the account commodity's fraction,
// matching how amounts are persisted internally. An amount that is not exact at
// its commodity fraction is an error rather than being rounded.
func (Writer) WriteGnuCashSQLite(ctx context.Context, path string, data app.GnuCashData) (err error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	for _, ddl := range schemaDDL {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
	}

	fraction := make(map[string]int64, len(data.Commodities))
	for _, c := range data.Commodities {
		fraction[c.GUID] = c.Fraction
	}
	acctCommodity := make(map[string]string, len(data.Accounts))
	for _, a := range data.Accounts {
		acctCommodity[a.GUID] = a.CommodityGUID
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = writeBook(ctx, tx, data); err != nil {
		return err
	}
	if err = writeCommodities(ctx, tx, data.Commodities); err != nil {
		return err
	}
	if err = writeAccounts(ctx, tx, data.Accounts, fraction); err != nil {
		return err
	}
	if err = writeTransactions(ctx, tx, data.Transactions, fraction, acctCommodity); err != nil {
		return err
	}
	if err = writeLots(ctx, tx, data.Lots); err != nil {
		return err
	}
	if err = writeScheduledTransactions(ctx, tx, data.ScheduledTransactions, data.Book.RootTemplateGUID, fraction); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func writeBook(ctx context.Context, tx *sql.Tx, data app.GnuCashData) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO books (guid, root_account_guid, root_template_guid) VALUES (?, ?, ?)`,
		data.Book.GUID, data.Book.RootAccountGUID, data.Book.RootTemplateGUID)
	if err != nil {
		return fmt.Errorf("write book: %w", err)
	}
	return nil
}

func writeLots(ctx context.Context, tx *sql.Tx, lots []domain.Lot) error {
	for _, l := range lots {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO lots (guid, account_guid, is_closed) VALUES (?, ?, ?)`,
			l.GUID, nullStr(l.AccountGUID), boolToInt(l.IsClosed),
		); err != nil {
			return fmt.Errorf("write lot %s: %w", l.GUID, err)
		}
	}
	return nil
}

func writeCommodities(ctx context.Context, tx *sql.Tx, commodities []domain.Commodity) error {
	for _, c := range commodities {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO commodities (guid, namespace, mnemonic, fullname, cusip, fraction, quote_flag, quote_source, quote_tz)
			 VALUES (?, ?, ?, ?, NULL, ?, 0, NULL, NULL)`,
			c.GUID, c.Namespace, c.Mnemonic, nullStr(c.Fullname), c.Fraction,
		); err != nil {
			return fmt.Errorf("write commodity %s: %w", c.GUID, err)
		}
	}
	return nil
}

func writeAccounts(ctx context.Context, tx *sql.Tx, accounts []domain.Account, fraction map[string]int64) error {
	for _, a := range accounts {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO accounts (guid, name, account_type, commodity_guid, commodity_scu, non_std_scu,
			    parent_guid, code, description, hidden, placeholder)
			 VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?)`,
			a.GUID, a.Name, string(a.Type), nullStr(a.CommodityGUID), fraction[a.CommodityGUID],
			nullStr(a.ParentGUID), a.Code, a.Description, boolToInt(a.Hidden), boolToInt(a.Placeholder),
		); err != nil {
			return fmt.Errorf("write account %s: %w", a.GUID, err)
		}
	}
	return nil
}

func writeTransactions(ctx context.Context, tx *sql.Tx, txns []domain.Transaction, fraction map[string]int64, acctCommodity map[string]string) error {
	for _, t := range txns {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO transactions (guid, currency_guid, num, post_date, enter_date, description)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			t.GUID, t.CurrencyGUID, t.Num, gncTime(t.PostDate), gncTime(t.EnterDate), t.Description,
		); err != nil {
			return fmt.Errorf("write transaction %s: %w", t.GUID, err)
		}
		currencyFraction := fraction[t.CurrencyGUID]
		if currencyFraction == 0 {
			return fmt.Errorf("transaction %s: unknown currency %s", t.GUID, t.CurrencyGUID)
		}
		for _, s := range t.Splits {
			if err := writeSplit(ctx, tx, t.GUID, s, currencyFraction, fraction, acctCommodity); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeSplit(ctx context.Context, tx *sql.Tx, txGUID string, s domain.Split, currencyFraction int64, fraction map[string]int64, acctCommodity map[string]string) error {
	valueNum, err := s.Value.AtDenom(currencyFraction)
	if err != nil {
		return fmt.Errorf("split %s value: %w", s.GUID, err)
	}
	accFraction := fraction[acctCommodity[s.AccountGUID]]
	if accFraction == 0 {
		return fmt.Errorf("split %s: unknown commodity for account %s", s.GUID, s.AccountGUID)
	}
	qtyNum, err := s.Quantity.AtDenom(accFraction)
	if err != nil {
		return fmt.Errorf("split %s quantity: %w", s.GUID, err)
	}
	reconcile := s.Reconcile
	if reconcile == 0 {
		reconcile = domain.ReconcileNew
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO splits (guid, tx_guid, account_guid, memo, action, reconcile_state, reconcile_date,
		    value_num, value_denom, quantity_num, quantity_denom, lot_guid)
		 VALUES (?, ?, ?, ?, ?, ?, NULL, ?, ?, ?, ?, ?)`,
		s.GUID, txGUID, s.AccountGUID, s.Memo, s.Action, string(reconcile),
		valueNum, currencyFraction, qtyNum, accFraction, nullStr(s.LotGUID),
	); err != nil {
		return fmt.Errorf("write split %s: %w", s.GUID, err)
	}
	return nil
}

// gncTime renders a timestamp in GnuCash's text format (UTC), or the empty
// string for the zero time.
func gncTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(gncTimeLayout)
}

// nullStr maps an empty string to SQL NULL so optional GUID/text columns are
// stored as NULL rather than an empty value.
func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// writeScheduledTransactions writes GnuCash-compatible schedxactions,
// recurrences, and template accounts/transactions for each scheduled
// transaction. Each schedule gets a synthesised template account (child of the
// template root) plus a template transaction whose splits carry the amounts;
// the marker split (pointing at the template account itself, value=0) is
// written first so the import path can locate the template by account_guid.
func writeScheduledTransactions(
	ctx context.Context,
	tx *sql.Tx,
	scheds []domain.ScheduledTransaction,
	templateRootGUID string,
	fraction map[string]int64,
) error {
	for _, s := range scheds {
		// Synthesise a template account for this scheduled transaction.
		templateActGUID := newGUID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO accounts (guid, name, account_type, commodity_guid, commodity_scu,
			    non_std_scu, parent_guid, code, description, hidden, placeholder)
			 VALUES (?, ?, 'ASSET', NULL, 1, 0, ?, NULL, NULL, 0, 0)`,
			templateActGUID, s.Name, nullStr(templateRootGUID),
		); err != nil {
			return fmt.Errorf("write template account for schedule %s: %w", s.GUID, err)
		}

		// Synthesise a template transaction and its splits.
		tmplTxGUID := newGUID()
		currFraction := fraction[s.CurrencyGUID]
		if currFraction == 0 {
			currFraction = 100 // default to cents when currency is unknown
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO transactions (guid, currency_guid, num, post_date, enter_date, description)
			 VALUES (?, ?, '', ?, ?, ?)`,
			tmplTxGUID, nullStr(s.CurrencyGUID),
			gncTime(s.StartDate), gncTime(time.Now().UTC()), s.Name,
		); err != nil {
			return fmt.Errorf("write template transaction for schedule %s: %w", s.GUID, err)
		}

		// Marker split — points at the template account with zero value.
		markerGUID := newGUID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO splits (guid, tx_guid, account_guid, memo, action, reconcile_state,
			    reconcile_date, value_num, value_denom, quantity_num, quantity_denom, lot_guid)
			 VALUES (?, ?, ?, '', '', 'n', NULL, 0, 1, 0, 1, NULL)`,
			markerGUID, tmplTxGUID, templateActGUID,
		); err != nil {
			return fmt.Errorf("write marker split for schedule %s: %w", s.GUID, err)
		}

		// Template splits (one per ScheduledSplit).
		for _, sp := range s.Splits {
			vNum, err := sp.Value.AtDenom(currFraction)
			if err != nil {
				return fmt.Errorf("schedule %s split %s value: %w", s.GUID, sp.GUID, err)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO splits (guid, tx_guid, account_guid, memo, action, reconcile_state,
				    reconcile_date, value_num, value_denom, quantity_num, quantity_denom, lot_guid)
				 VALUES (?, ?, ?, ?, '', 'n', NULL, ?, ?, ?, ?, NULL)`,
				sp.GUID, tmplTxGUID, sp.AccountGUID, sp.Memo,
				vNum, currFraction, vNum, currFraction,
			); err != nil {
				return fmt.Errorf("write template split %s for schedule %s: %w", sp.GUID, s.GUID, err)
			}
		}

		// schedxactions row.
		startStr := ""
		if !s.StartDate.IsZero() {
			startStr = s.StartDate.UTC().Format("2006-01-02 00:00:00 UTC")
		}
		endStr := ""
		if !s.EndDate.IsZero() {
			endStr = s.EndDate.UTC().Format("2006-01-02 00:00:00 UTC")
		}
		lastStr := ""
		if !s.LastPostedDate.IsZero() {
			lastStr = s.LastPostedDate.UTC().Format("2006-01-02 00:00:00 UTC")
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schedxactions (guid, name, enabled, start_date, end_date, last_occur,
			    num_occur, rem_occur, auto_create, auto_notify, advance_creation_days,
			    advance_remind_days, instance_count, template_act_guid)
			 VALUES (?, ?, ?, ?, ?, ?, 0, 0, 0, 0, 0, 0, 0, ?)`,
			s.GUID, s.Name, boolToInt(s.Enabled),
			nullStr(startStr), nullStr(endStr), nullStr(lastStr),
			templateActGUID,
		); err != nil {
			return fmt.Errorf("write schedxaction %s: %w", s.GUID, err)
		}

		// recurrences row.
		periodStart := startStr
		if periodStart == "" {
			periodStart = time.Now().UTC().Format("2006-01-02 00:00:00 UTC")
		}
		every := s.Every
		if every <= 0 {
			every = 1
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO recurrences (obj_guid, recurrence_mult, recurrence_period_type,
			    recurrence_period_start, recurrence_weekend_adj)
			 VALUES (?, ?, ?, ?, 'none')`,
			s.GUID, every, reverseGncPeriod(s.Period), periodStart,
		); err != nil {
			return fmt.Errorf("write recurrence for schedule %s: %w", s.GUID, err)
		}
	}
	return nil
}
