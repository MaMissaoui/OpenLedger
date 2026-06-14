package gnucash

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// sampleData is a USD book with two postable accounts and one balanced
// transaction, expressed in domain types — the unit an export writes.
func sampleData() app.GnuCashData {
	return app.GnuCashData{
		Book:        domain.Book{GUID: "book1", RootAccountGUID: "root1", RootTemplateGUID: "troot1"},
		Commodities: []domain.Commodity{{GUID: "usd", Namespace: "CURRENCY", Mnemonic: "USD", Fullname: "US Dollar", Fraction: 100}},
		Accounts: []domain.Account{
			{GUID: "root1", Name: "Root Account", Type: domain.AccountRoot},
			{GUID: "troot1", Name: "Template Root", Type: domain.AccountRoot},
			{GUID: "chk", Name: "Checking", Type: domain.AccountBank, CommodityGUID: "usd", ParentGUID: "root1", Code: "1010", Description: "Main bank"},
			{GUID: "sal", Name: "Salary", Type: domain.AccountIncome, CommodityGUID: "usd", ParentGUID: "root1"},
		},
		Transactions: []domain.Transaction{{
			GUID: "tx1", CurrencyGUID: "usd", Description: "Paycheck",
			Splits: []domain.Split{
				{GUID: "s1", AccountGUID: "chk", Value: domain.MustFromNumDenom(5000, 1), Quantity: domain.MustFromNumDenom(5000, 1), LotGUID: "lot1"},
				{GUID: "s2", AccountGUID: "sal", Value: domain.MustFromNumDenom(-5000, 1), Quantity: domain.MustFromNumDenom(-5000, 1)},
			},
		}},
		Lots: []domain.Lot{{GUID: "lot1", AccountGUID: "chk", IsClosed: false}},
	}
}

// TestRoundTrip writes a book to a GnuCash SQLite file and reads it back,
// asserting the book, commodity, accounts, and balanced transaction survive the
// round trip with GUIDs and values intact.
func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.gnucash")
	ctx := context.Background()
	src := sampleData()

	if err := (Writer{}).WriteGnuCashSQLite(ctx, path, src); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := (Reader{}).ReadGnuCashSQLite(ctx, path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	if got.Book != src.Book {
		t.Errorf("book = %+v, want %+v", got.Book, src.Book)
	}
	if len(got.Commodities) != 1 || got.Commodities[0] != src.Commodities[0] {
		t.Errorf("commodities = %+v", got.Commodities)
	}
	if len(got.Accounts) != len(src.Accounts) {
		t.Fatalf("accounts = %d, want %d", len(got.Accounts), len(src.Accounts))
	}

	if len(got.Lots) != 1 || got.Lots[0] != src.Lots[0] {
		t.Errorf("lots = %+v, want %+v", got.Lots, src.Lots)
	}

	if len(got.Transactions) != 1 {
		t.Fatalf("transactions = %d, want 1", len(got.Transactions))
	}
	tx := got.Transactions[0]
	if tx.GUID != "tx1" || tx.Description != "Paycheck" {
		t.Errorf("tx = %+v", tx)
	}
	if err := tx.ValidateBalanced(); err != nil {
		t.Errorf("round-tripped transaction does not balance: %v", err)
	}
	if len(tx.Splits) != 2 {
		t.Fatalf("splits = %d, want 2", len(tx.Splits))
	}
	for _, s := range tx.Splits {
		want := src.Transactions[0].Splits[splitIndex(t, src, s.GUID)].Value
		if !s.Value.Equal(want) {
			t.Errorf("split %s value = %s, want %s", s.GUID, s.Value, want)
		}
	}
}

func splitIndex(t *testing.T, data app.GnuCashData, guid string) int {
	t.Helper()
	for i, s := range data.Transactions[0].Splits {
		if s.GUID == guid {
			return i
		}
	}
	t.Fatalf("split %s not in source", guid)
	return -1
}
