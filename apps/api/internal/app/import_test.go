package app

import (
	"context"
	"errors"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// stubReader returns canned data/error for the GnuCashReader port.
type stubReader struct {
	data GnuCashData
	err  error
}

func (s stubReader) ReadGnuCashSQLite(_ context.Context, _ string) (GnuCashData, error) {
	return s.data, s.err
}

// captureRepo records what ImportBook was asked to persist.
type captureRepo struct {
	got   *GnuCashData
	owner string
	err   error
}

func (r *captureRepo) ImportBook(_ context.Context, data GnuCashData, ownerUserID string) error {
	if r.err != nil {
		return r.err
	}
	cp := data
	r.got = &cp
	r.owner = ownerUserID
	return nil
}

// balancedData is a one-transaction USD book whose two split values sum to zero.
func balancedData() GnuCashData {
	return GnuCashData{
		Book:        domain.Book{GUID: "book1", RootAccountGUID: "root1", RootTemplateGUID: "troot1"},
		Commodities: []domain.Commodity{{GUID: "usd", Namespace: "CURRENCY", Mnemonic: "USD", Fraction: 100}},
		Accounts: []domain.Account{
			{GUID: "root1", Name: "Root", Type: domain.AccountRoot},
			{GUID: "chk", Name: "Checking", Type: domain.AccountBank, CommodityGUID: "usd", ParentGUID: "root1"},
			{GUID: "sal", Name: "Salary", Type: domain.AccountIncome, CommodityGUID: "usd", ParentGUID: "root1"},
		},
		Transactions: []domain.Transaction{{
			GUID: "tx1", CurrencyGUID: "usd", Description: "Paycheck",
			Splits: []domain.Split{
				{GUID: "s1", AccountGUID: "chk", Value: domain.MustFromNumDenom(5000, 1), Quantity: domain.MustFromNumDenom(5000, 1)},
				{GUID: "s2", AccountGUID: "sal", Value: domain.MustFromNumDenom(-5000, 1), Quantity: domain.MustFromNumDenom(-5000, 1)},
			},
		}},
	}
}

func TestImportSQLitePersistsBook(t *testing.T) {
	repo := &captureRepo{}
	svc := NewImportService(stubReader{data: balancedData()}, repo)

	result, err := svc.ImportSQLite(context.Background(), "/ignored.gnucash", "user-7")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if result.BookGUID != "book1" || result.Commodities != 1 || result.Accounts != 3 || result.Transactions != 1 {
		t.Errorf("result = %+v", result)
	}
	if repo.got == nil {
		t.Fatal("ImportBook was not called")
	}
	if repo.owner != "user-7" {
		t.Errorf("owner = %q, want user-7", repo.owner)
	}
}

func TestImportSQLiteRejectsUnbalanced(t *testing.T) {
	data := balancedData()
	// Break the balance: the second split no longer offsets the first.
	data.Transactions[0].Splits[1].Value = domain.MustFromNumDenom(-4000, 1)
	repo := &captureRepo{}
	svc := NewImportService(stubReader{data: data}, repo)

	_, err := svc.ImportSQLite(context.Background(), "/x", "user-1")
	if !errors.Is(err, domain.ErrUnbalanced) {
		t.Fatalf("err = %v, want ErrUnbalanced", err)
	}
	if repo.got != nil {
		t.Error("unbalanced import must not be persisted")
	}
}

func TestImportSQLiteParseError(t *testing.T) {
	svc := NewImportService(stubReader{err: errors.New("disk on fire")}, &captureRepo{})
	_, err := svc.ImportSQLite(context.Background(), "/x", "user-1")
	if !errors.Is(err, ErrImportParse) {
		t.Fatalf("err = %v, want ErrImportParse", err)
	}
}

func TestImportSQLiteEmptyBook(t *testing.T) {
	svc := NewImportService(stubReader{data: GnuCashData{}}, &captureRepo{})
	_, err := svc.ImportSQLite(context.Background(), "/x", "user-1")
	if !errors.Is(err, ErrImportParse) {
		t.Fatalf("err = %v, want ErrImportParse for a book-less file", err)
	}
}

func TestImportSQLitePropagatesRepoError(t *testing.T) {
	repo := &captureRepo{err: ErrImportConflict}
	svc := NewImportService(stubReader{data: balancedData()}, repo)
	_, err := svc.ImportSQLite(context.Background(), "/x", "user-1")
	if !errors.Is(err, ErrImportConflict) {
		t.Fatalf("err = %v, want ErrImportConflict", err)
	}
}
