package gnucash

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// TestXMLRoundTrip writes a book to a GnuCash XML file and reads it back,
// asserting the book id, commodity (by namespace+mnemonic+fraction), accounts,
// and balanced transaction survive the round trip. Commodity GUIDs are
// re-synthesised on read (the XML format stores no commodity GUID), so accounts
// and the transaction are matched to the commodity by its resolved GUID rather
// than the source GUID.
func TestXMLRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.gnucash")
	ctx := context.Background()
	src := sampleData()
	// Give the salary account a hidden flag so slot round-tripping is exercised.
	for i := range src.Accounts {
		if src.Accounts[i].GUID == "sal" {
			src.Accounts[i].Hidden = true
		}
	}

	if err := (Writer{}).WriteGnuCashXML(ctx, path, src); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := (Reader{}).ReadGnuCashXML(ctx, path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	if got.Book.GUID != "book1" || got.Book.RootAccountGUID != "root1" {
		t.Errorf("book = %+v", got.Book)
	}
	// The template root is not written, so the reader synthesises a fresh one
	// distinct from the real root.
	if got.Book.RootTemplateGUID == "" || got.Book.RootTemplateGUID == "root1" {
		t.Errorf("template root = %q", got.Book.RootTemplateGUID)
	}

	if len(got.Commodities) != 1 {
		t.Fatalf("commodities = %d, want 1", len(got.Commodities))
	}
	usd := got.Commodities[0]
	if usd.Namespace != "CURRENCY" || usd.Mnemonic != "USD" || usd.Fullname != "US Dollar" || usd.Fraction != 100 {
		t.Errorf("commodity = %+v", usd)
	}

	// Source has 4 accounts incl. the template root, which XML omits; the reader
	// then synthesises a new template root → 4 again, but the real ROOT (root1)
	// plus chk, sal, and the new template root.
	chk := findAccount(t, got.Accounts, "chk")
	if chk.Type != domain.AccountBank || chk.Code != "1010" || chk.ParentGUID != "root1" ||
		chk.Description != "Main bank" || chk.CommodityGUID != usd.GUID {
		t.Errorf("checking = %+v", chk)
	}
	if !findAccount(t, got.Accounts, "sal").Hidden {
		t.Error("salary account should be hidden (slot round-trip failed)")
	}
	for _, a := range got.Accounts {
		if a.GUID == "troot1" {
			t.Error("source template root should not be written to XML")
		}
	}

	if len(got.Transactions) != 1 {
		t.Fatalf("transactions = %d, want 1", len(got.Transactions))
	}
	tx := got.Transactions[0]
	if tx.GUID != "tx1" || tx.Description != "Paycheck" || tx.CurrencyGUID != usd.GUID {
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
