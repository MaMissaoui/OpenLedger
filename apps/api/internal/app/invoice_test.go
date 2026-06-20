package app

import (
	"context"
	"testing"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// fakeInvoiceRepo implements InvoiceRepository, TransactionRepository, and
// MembershipRepository so a real InvoiceService can be exercised end to end,
// capturing the transaction PostInvoice posts.
type fakeInvoiceRepo struct {
	invoice   domain.Invoice
	entries   []domain.InvoiceEntry
	taxTables map[string]domain.TaxTable
	posted    *domain.Transaction
}

// InvoiceRepository
func (f *fakeInvoiceRepo) GetInvoice(context.Context, string) (domain.Invoice, error) {
	return f.invoice, nil
}
func (f *fakeInvoiceRepo) ListEntries(context.Context, string) ([]domain.InvoiceEntry, error) {
	return f.entries, nil
}
func (f *fakeInvoiceRepo) GetTaxTable(_ context.Context, guid string) (domain.TaxTable, error) {
	tt, ok := f.taxTables[guid]
	if !ok {
		return domain.TaxTable{}, domain.ErrTaxTableNotFound
	}
	return tt, nil
}
func (f *fakeInvoiceRepo) GetBillTerm(context.Context, string) (domain.BillTerm, error) {
	return domain.BillTerm{}, domain.ErrBillTermNotFound
}
func (f *fakeInvoiceRepo) MarkInvoicePosted(context.Context, string, string, string, *time.Time, *time.Time) error {
	return nil
}
func (f *fakeInvoiceRepo) ListInvoices(context.Context, string, string) ([]domain.Invoice, error) {
	return nil, nil
}
func (f *fakeInvoiceRepo) CreateInvoice(context.Context, domain.Invoice) error { return nil }
func (f *fakeInvoiceRepo) UpdateInvoice(context.Context, domain.Invoice) error { return nil }
func (f *fakeInvoiceRepo) DeleteInvoice(context.Context, string) error         { return nil }
func (f *fakeInvoiceRepo) MarkInvoicePaid(context.Context, string, string, time.Time) error {
	return nil
}
func (f *fakeInvoiceRepo) ARAgingRows(context.Context, string) ([]AgingRow, error) { return nil, nil }
func (f *fakeInvoiceRepo) APAgingRows(context.Context, string) ([]AgingRow, error) { return nil, nil }
func (f *fakeInvoiceRepo) CreateEntry(context.Context, domain.InvoiceEntry) error  { return nil }
func (f *fakeInvoiceRepo) GetEntry(context.Context, string) (domain.InvoiceEntry, error) {
	return domain.InvoiceEntry{}, nil
}
func (f *fakeInvoiceRepo) UpdateEntry(context.Context, domain.InvoiceEntry) error { return nil }
func (f *fakeInvoiceRepo) DeleteEntry(context.Context, string) error              { return nil }

// TransactionRepository — captures the posted transaction.
func (f *fakeInvoiceRepo) InsertTransaction(_ context.Context, tx domain.Transaction, _ AuditActor) error {
	f.posted = &tx
	return nil
}
func (f *fakeInvoiceRepo) UpdateTransaction(context.Context, domain.Transaction, AuditActor) error {
	return nil
}
func (f *fakeInvoiceRepo) DeleteTransaction(context.Context, string, AuditActor) error { return nil }
func (f *fakeInvoiceRepo) TransactionAccountGUIDs(context.Context, string) ([]string, error) {
	return nil, nil
}

// MembershipRepository — grant write access.
func (f *fakeInvoiceRepo) UserBookRole(context.Context, string, string) (Role, bool, error) {
	return RoleOwner, true, nil
}
func (f *fakeInvoiceRepo) BookGUIDForAccount(context.Context, string) (string, error) {
	return "book-1", nil
}
func (f *fakeInvoiceRepo) AccountGUIDForSplit(context.Context, string) (string, error) {
	return "", nil
}

// splitFor returns the value posted to accountGUID across the transaction.
func splitFor(tx *domain.Transaction, accountGUID string) (domain.GncNumeric, bool) {
	for _, s := range tx.Splits {
		if s.AccountGUID == accountGUID {
			return s.Value, true
		}
	}
	return domain.Zero(), false
}

func TestPostInvoiceAppliesTax(t *testing.T) {
	// A customer invoice with one $100 taxable line and a 10% VAT table should
	// post: A/R debit 110, income credit -100, VAT credit -10.
	fr := &fakeInvoiceRepo{
		invoice: domain.Invoice{
			GUID: "inv-1", BookGUID: "book-1", Type: domain.InvoiceTypeCustomer,
			CurrencyGUID: "usd",
		},
		entries: []domain.InvoiceEntry{{
			GUID: "e-1", AccountGUID: "income-acc",
			Quantity: domain.MustFromNumDenom(1, 1), Price: domain.MustFromNumDenom(100, 1),
			Taxable: true, TaxTableGUID: "vat",
		}},
		taxTables: map[string]domain.TaxTable{
			"vat": {GUID: "vat", Entries: []domain.TaxTableEntry{
				{AccountGUID: "vat-acc", Type: domain.TaxPercentage, Amount: domain.MustFromNumDenom(10, 1)},
			}},
		},
	}
	svc := NewInvoiceService(fr, NewPostingService(fr), NewAuthzService(fr))

	if _, err := svc.PostInvoice(context.Background(), "user-1", "inv-1", PostRequest{
		PostDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), PostAccGUID: "ar-acc",
	}); err != nil {
		t.Fatalf("PostInvoice: %v", err)
	}
	if fr.posted == nil {
		t.Fatal("no transaction was posted")
	}

	wants := map[string]domain.GncNumeric{
		"ar-acc":     domain.MustFromNumDenom(110, 1),
		"income-acc": domain.MustFromNumDenom(-100, 1),
		"vat-acc":    domain.MustFromNumDenom(-10, 1),
	}
	for acc, want := range wants {
		got, ok := splitFor(fr.posted, acc)
		if !ok {
			t.Errorf("no split posted to %s", acc)
			continue
		}
		if got.Cmp(want) != 0 {
			t.Errorf("%s: got %v, want %v", acc, got, want)
		}
	}

	// Sanity: the posted transaction balances (PostingService would have
	// rejected it otherwise, but assert explicitly).
	sum := domain.Zero()
	for _, s := range fr.posted.Splits {
		sum = sum.Add(s.Value)
	}
	if !sum.IsZero() {
		t.Errorf("posted splits do not balance: sum=%v", sum)
	}
}
