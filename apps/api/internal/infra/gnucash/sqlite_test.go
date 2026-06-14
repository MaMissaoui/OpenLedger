package gnucash

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// writeFixture builds a minimal GnuCash-shaped SQLite database: one USD book
// with a checking and a salary account and a single balanced transaction, then
// returns its path.
func writeFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "book.gnucash")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE books (guid TEXT PRIMARY KEY, root_account_guid TEXT, root_template_guid TEXT)`,
		`CREATE TABLE commodities (guid TEXT PRIMARY KEY, namespace TEXT, mnemonic TEXT, fullname TEXT, fraction INTEGER)`,
		`CREATE TABLE accounts (guid TEXT PRIMARY KEY, name TEXT, account_type TEXT, commodity_guid TEXT,
		     parent_guid TEXT, code TEXT, description TEXT, hidden INTEGER, placeholder INTEGER)`,
		`CREATE TABLE transactions (guid TEXT PRIMARY KEY, currency_guid TEXT, num TEXT, post_date TEXT,
		     enter_date TEXT, description TEXT)`,
		`CREATE TABLE splits (guid TEXT PRIMARY KEY, tx_guid TEXT, account_guid TEXT, memo TEXT, action TEXT,
		     reconcile_state TEXT, value_num INTEGER, value_denom INTEGER, quantity_num INTEGER,
		     quantity_denom INTEGER, lot_guid TEXT)`,

		`INSERT INTO commodities VALUES ('usd', 'CURRENCY', 'USD', 'US Dollar', 100)`,
		`INSERT INTO books VALUES ('book1', 'root1', 'troot1')`,
		`INSERT INTO accounts VALUES ('root1', 'Root Account', 'ROOT', 'usd', NULL, NULL, NULL, 0, 0)`,
		`INSERT INTO accounts VALUES ('chk', 'Checking', 'BANK', 'usd', 'root1', '1010', 'Main bank', 0, 0)`,
		`INSERT INTO accounts VALUES ('sal', 'Salary', 'INCOME', 'usd', 'root1', NULL, NULL, 0, 0)`,
		`INSERT INTO transactions VALUES ('tx1', 'usd', '', '2024-03-01 00:00:00', '2024-03-01 12:00:00', 'Paycheck')`,
		`INSERT INTO splits VALUES ('s1', 'tx1', 'chk', '', '', 'n', 500000, 100, 500000, 100, NULL)`,
		`INSERT INTO splits VALUES ('s2', 'tx1', 'sal', '', '', 'n', -500000, 100, -500000, 100, NULL)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
	return path
}

func TestReadGnuCashSQLite(t *testing.T) {
	data, err := NewReader().ReadGnuCashSQLite(context.Background(), writeFixture(t))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if data.Book.GUID != "book1" || data.Book.RootAccountGUID != "root1" || data.Book.RootTemplateGUID != "troot1" {
		t.Errorf("book = %+v", data.Book)
	}
	if len(data.Commodities) != 1 || data.Commodities[0].Mnemonic != "USD" || data.Commodities[0].Fraction != 100 {
		t.Errorf("commodities = %+v", data.Commodities)
	}
	if len(data.Accounts) != 3 {
		t.Fatalf("accounts = %d, want 3", len(data.Accounts))
	}

	// The reader should preserve type, code, parent, and the GnuCash GUIDs.
	chk := findAccount(t, data.Accounts, "chk")
	if chk.Type != domain.AccountBank || chk.Code != "1010" || chk.ParentGUID != "root1" || chk.Description != "Main bank" {
		t.Errorf("checking = %+v", chk)
	}

	if len(data.Transactions) != 1 {
		t.Fatalf("transactions = %d, want 1", len(data.Transactions))
	}
	tx := data.Transactions[0]
	if tx.GUID != "tx1" || tx.Description != "Paycheck" || tx.CurrencyGUID != "usd" {
		t.Errorf("tx = %+v", tx)
	}
	if tx.PostDate.IsZero() || tx.PostDate.Year() != 2024 {
		t.Errorf("post date = %v", tx.PostDate)
	}
	if len(tx.Splits) != 2 {
		t.Fatalf("splits = %d, want 2", len(tx.Splits))
	}
	// The imported transaction must balance: the two split values sum to zero.
	if err := tx.ValidateBalanced(); err != nil {
		t.Errorf("imported transaction does not balance: %v", err)
	}
	want := domain.MustFromNumDenom(5000, 1)
	if got := tx.Splits[0].Value; !got.Equal(want) {
		t.Errorf("split[0] value = %s, want %s", got, want)
	}
}

func TestReadGnuCashSQLiteNoBook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.gnucash")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE books (guid TEXT, root_account_guid TEXT, root_template_guid TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	db.Close()

	data, err := NewReader().ReadGnuCashSQLite(context.Background(), path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if data.Book.GUID != "" {
		t.Errorf("expected empty book, got %+v", data.Book)
	}
}

func findAccount(t *testing.T, accts []domain.Account, guid string) domain.Account {
	t.Helper()
	for _, a := range accts {
		if a.GUID == guid {
			return a
		}
	}
	t.Fatalf("account %s not found", guid)
	return domain.Account{}
}
