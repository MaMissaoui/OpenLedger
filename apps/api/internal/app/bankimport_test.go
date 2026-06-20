package app

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

type fakeStatementReader struct {
	txns []StatementTxn
	err  error
}

func (f fakeStatementReader) Read(io.Reader) ([]StatementTxn, error) { return f.txns, f.err }

// fakeBankRepo satisfies both BankImportRepository and the PostingService's
// TransactionRepository so one fake backs the whole import.
type fakeBankRepo struct {
	currency   domain.Commodity
	accountErr error
	existing   map[string]struct{}
	posted     []domain.Transaction
}

func (f *fakeBankRepo) AccountCommodity(context.Context, string) (AccountCommodityInfo, error) {
	if f.accountErr != nil {
		return AccountCommodityInfo{}, f.accountErr
	}
	return AccountCommodityInfo{Commodity: f.currency}, nil
}

func (f *fakeBankRepo) FindOrCreateImbalanceAccount(context.Context, string, domain.Commodity) (string, error) {
	return "imbalance", nil
}

func (f *fakeBankRepo) ExistingImportRefs(context.Context, string) (map[string]struct{}, error) {
	if f.existing == nil {
		return map[string]struct{}{}, nil
	}
	return f.existing, nil
}

func (f *fakeBankRepo) InsertTransaction(_ context.Context, tx domain.Transaction, _ AuditActor) error {
	f.posted = append(f.posted, tx)
	return nil
}
func (f *fakeBankRepo) UpdateTransaction(context.Context, domain.Transaction, AuditActor) error {
	return nil
}
func (f *fakeBankRepo) DeleteTransaction(context.Context, string, AuditActor) error { return nil }
func (f *fakeBankRepo) TransactionAccountGUIDs(context.Context, string) ([]string, error) {
	return nil, nil
}

func usdCurrency() domain.Commodity {
	return domain.Commodity{GUID: "usd", Namespace: domain.NamespaceCurrency, Mnemonic: "USD"}
}

func sampleTxns() []StatementTxn {
	debit, _ := domain.FromDecimalString("-50.00")
	credit, _ := domain.FromDecimalString("1000.00")
	return []StatementTxn{
		{Date: time.Date(2024, 6, 19, 0, 0, 0, 0, time.UTC), Amount: debit, Memo: "Safeway", FITID: "F1"},
		{Date: time.Date(2024, 6, 20, 0, 0, 0, 0, time.UTC), Amount: credit, Memo: "Employer", FITID: "F2"},
	}
}

func newBankImport(repo *fakeBankRepo, txns []StatementTxn) *BankImportService {
	return NewBankImportService(
		NewPostingService(repo), repo,
		map[string]StatementReader{"ofx": fakeStatementReader{txns: txns}},
	)
}

func TestBankImport(t *testing.T) {
	repo := &fakeBankRepo{currency: usdCurrency()}
	svc := newBankImport(repo, sampleTxns())

	res, err := svc.Import(context.Background(), "checking", "ofx", nil, AuditActor{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Imported != 2 || res.Skipped != 0 {
		t.Fatalf("result = %+v, want imported 2 skipped 0", res)
	}
	if len(repo.posted) != 2 {
		t.Fatalf("posted %d txns, want 2", len(repo.posted))
	}

	tx := repo.posted[0]
	if tx.CurrencyGUID != "usd" || tx.Num != "F1" || tx.Description != "Safeway" {
		t.Errorf("tx = %+v, want USD currency, num F1, desc Safeway", tx)
	}
	if len(tx.Splits) != 2 {
		t.Fatalf("tx has %d splits, want 2", len(tx.Splits))
	}
	if tx.Splits[0].AccountGUID != "checking" || tx.Splits[0].Value.DecimalString(2) != "-50.00" {
		t.Errorf("split0 = %+v, want checking -50.00", tx.Splits[0])
	}
	if tx.Splits[1].AccountGUID != "imbalance" || tx.Splits[1].Value.DecimalString(2) != "50.00" {
		t.Errorf("split1 = %+v, want imbalance 50.00", tx.Splits[1])
	}
	// The two value legs must net to zero (the posting invariant).
	if !tx.Splits[0].Value.Add(tx.Splits[1].Value).IsZero() {
		t.Error("imported transaction does not balance")
	}
}

func TestBankImportSkipsDuplicates(t *testing.T) {
	repo := &fakeBankRepo{currency: usdCurrency(), existing: map[string]struct{}{"F1": {}}}
	svc := newBankImport(repo, sampleTxns())

	res, err := svc.Import(context.Background(), "checking", "ofx", nil, AuditActor{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Imported != 1 || res.Skipped != 1 {
		t.Fatalf("result = %+v, want imported 1 skipped 1", res)
	}
	if len(repo.posted) != 1 || repo.posted[0].Num != "F2" {
		t.Errorf("posted = %+v, want only F2", repo.posted)
	}
}

func TestBankImportUnsupportedFormat(t *testing.T) {
	repo := &fakeBankRepo{currency: usdCurrency()}
	svc := newBankImport(repo, sampleTxns())
	_, err := svc.Import(context.Background(), "checking", "csv", nil, AuditActor{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestBankImportRejectsNonCurrencyAccount(t *testing.T) {
	repo := &fakeBankRepo{currency: domain.Commodity{GUID: "aapl", Namespace: "STOCK", Mnemonic: "AAPL"}}
	svc := newBankImport(repo, sampleTxns())
	_, err := svc.Import(context.Background(), "brokerage", "ofx", nil, AuditActor{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestBankImportParseError(t *testing.T) {
	repo := &fakeBankRepo{currency: usdCurrency()}
	svc := NewBankImportService(NewPostingService(repo), repo,
		map[string]StatementReader{"ofx": fakeStatementReader{err: errors.New("bad file")}})
	_, err := svc.Import(context.Background(), "checking", "ofx", nil, AuditActor{})
	if !errors.Is(err, ErrImportParse) {
		t.Fatalf("err = %v, want ErrImportParse", err)
	}
}

func TestBankImportAccountNotFound(t *testing.T) {
	repo := &fakeBankRepo{currency: usdCurrency(), accountErr: ErrAccountNotFound}
	svc := newBankImport(repo, sampleTxns())
	_, err := svc.Import(context.Background(), "missing", "ofx", nil, AuditActor{})
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("err = %v, want ErrAccountNotFound", err)
	}
}
