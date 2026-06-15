package app

import (
	"context"
	"fmt"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// InvoiceRepository is the persistence port for invoices and their line items.
type InvoiceRepository interface {
	ListInvoices(ctx context.Context, bookGUID, invoiceType string) ([]domain.Invoice, error)
	CreateInvoice(ctx context.Context, inv domain.Invoice) error
	GetInvoice(ctx context.Context, guid string) (domain.Invoice, error)
	UpdateInvoice(ctx context.Context, inv domain.Invoice) error
	DeleteInvoice(ctx context.Context, guid string) error
	MarkInvoicePosted(ctx context.Context, guid, txnGUID, accGUID string, datePosted, dateDue *time.Time) error

	ListEntries(ctx context.Context, invoiceGUID string) ([]domain.InvoiceEntry, error)
	CreateEntry(ctx context.Context, e domain.InvoiceEntry) error
	GetEntry(ctx context.Context, guid string) (domain.InvoiceEntry, error)
	UpdateEntry(ctx context.Context, e domain.InvoiceEntry) error
	DeleteEntry(ctx context.Context, guid string) error
}

// InvoiceService manages the lifecycle of invoices (customer) and bills (vendor).
// Posting an invoice creates a balanced transaction via PostingService.
type InvoiceService struct {
	repo    InvoiceRepository
	posting *PostingService
	authz   *AuthzService
	newGUID func() string
	now     func() time.Time
}

func NewInvoiceService(repo InvoiceRepository, posting *PostingService, authz *AuthzService) *InvoiceService {
	return &InvoiceService{repo: repo, posting: posting, authz: authz, newGUID: NewGUID, now: time.Now}
}

func (s *InvoiceService) ListInvoices(ctx context.Context, userID, bookGUID string, invType domain.InvoiceType) ([]domain.Invoice, error) {
	if err := s.authz.AuthorizeBook(ctx, userID, bookGUID, AccessRead); err != nil {
		return nil, err
	}
	return s.repo.ListInvoices(ctx, bookGUID, string(invType))
}

func (s *InvoiceService) CreateInvoice(ctx context.Context, userID string, inv domain.Invoice) (domain.Invoice, error) {
	if err := s.authz.AuthorizeBook(ctx, userID, inv.BookGUID, AccessWrite); err != nil {
		return domain.Invoice{}, err
	}
	inv.GUID = s.newGUID()
	if inv.DateOpened.IsZero() {
		inv.DateOpened = s.now().UTC().Truncate(24 * time.Hour)
	}
	if err := s.repo.CreateInvoice(ctx, inv); err != nil {
		return domain.Invoice{}, err
	}
	return inv, nil
}

func (s *InvoiceService) GetInvoice(ctx context.Context, userID, guid string) (domain.Invoice, error) {
	inv, err := s.repo.GetInvoice(ctx, guid)
	if err != nil {
		return domain.Invoice{}, err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, inv.BookGUID, AccessRead); err != nil {
		return domain.Invoice{}, err
	}
	entries, err := s.repo.ListEntries(ctx, guid)
	if err != nil {
		return domain.Invoice{}, err
	}
	inv.Entries = entries
	return inv, nil
}

func (s *InvoiceService) UpdateInvoice(ctx context.Context, userID string, inv domain.Invoice) (domain.Invoice, error) {
	existing, err := s.repo.GetInvoice(ctx, inv.GUID)
	if err != nil {
		return domain.Invoice{}, err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, existing.BookGUID, AccessWrite); err != nil {
		return domain.Invoice{}, err
	}
	if existing.IsPosted() {
		return domain.Invoice{}, domain.ErrInvoiceAlreadyPosted
	}
	inv.BookGUID = existing.BookGUID
	if err := s.repo.UpdateInvoice(ctx, inv); err != nil {
		return domain.Invoice{}, err
	}
	return inv, nil
}

func (s *InvoiceService) DeleteInvoice(ctx context.Context, userID, guid string) error {
	existing, err := s.repo.GetInvoice(ctx, guid)
	if err != nil {
		return err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, existing.BookGUID, AccessWrite); err != nil {
		return err
	}
	if existing.IsPosted() {
		return domain.ErrInvoiceAlreadyPosted
	}
	return s.repo.DeleteInvoice(ctx, guid)
}

func (s *InvoiceService) AddEntry(ctx context.Context, userID string, e domain.InvoiceEntry) (domain.InvoiceEntry, error) {
	inv, err := s.repo.GetInvoice(ctx, e.InvoiceGUID)
	if err != nil {
		return domain.InvoiceEntry{}, err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, inv.BookGUID, AccessWrite); err != nil {
		return domain.InvoiceEntry{}, err
	}
	if inv.IsPosted() {
		return domain.InvoiceEntry{}, domain.ErrInvoiceAlreadyPosted
	}
	e.GUID = s.newGUID()
	if e.Date.IsZero() {
		e.Date = inv.DateOpened
	}
	if err := s.repo.CreateEntry(ctx, e); err != nil {
		return domain.InvoiceEntry{}, err
	}
	return e, nil
}

func (s *InvoiceService) UpdateEntry(ctx context.Context, userID string, e domain.InvoiceEntry) (domain.InvoiceEntry, error) {
	existing, err := s.repo.GetEntry(ctx, e.GUID)
	if err != nil {
		return domain.InvoiceEntry{}, err
	}
	inv, err := s.repo.GetInvoice(ctx, existing.InvoiceGUID)
	if err != nil {
		return domain.InvoiceEntry{}, err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, inv.BookGUID, AccessWrite); err != nil {
		return domain.InvoiceEntry{}, err
	}
	if inv.IsPosted() {
		return domain.InvoiceEntry{}, domain.ErrInvoiceAlreadyPosted
	}
	e.InvoiceGUID = existing.InvoiceGUID
	if err := s.repo.UpdateEntry(ctx, e); err != nil {
		return domain.InvoiceEntry{}, err
	}
	return e, nil
}

func (s *InvoiceService) DeleteEntry(ctx context.Context, userID, guid string) error {
	existing, err := s.repo.GetEntry(ctx, guid)
	if err != nil {
		return err
	}
	inv, err := s.repo.GetInvoice(ctx, existing.InvoiceGUID)
	if err != nil {
		return err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, inv.BookGUID, AccessWrite); err != nil {
		return err
	}
	if inv.IsPosted() {
		return domain.ErrInvoiceAlreadyPosted
	}
	return s.repo.DeleteEntry(ctx, guid)
}

// PostRequest carries the parameters for finalizing an invoice.
type PostRequest struct {
	PostDate    time.Time
	DueDate     *time.Time
	PostAccGUID string // A/R account for invoices, A/P for bills
}

// PostInvoice finalizes an invoice: builds a balanced transaction via
// PostingService then marks the invoice posted with the resulting txn GUID.
// For customer invoices the A/R account is debited and each entry's income
// account is credited; for vendor bills each entry's expense account is debited
// and the A/P account is credited.
func (s *InvoiceService) PostInvoice(ctx context.Context, userID, guid string, req PostRequest) (domain.Invoice, error) {
	inv, err := s.repo.GetInvoice(ctx, guid)
	if err != nil {
		return domain.Invoice{}, err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, inv.BookGUID, AccessWrite); err != nil {
		return domain.Invoice{}, err
	}
	if inv.IsPosted() {
		return domain.Invoice{}, domain.ErrInvoiceAlreadyPosted
	}

	entries, err := s.repo.ListEntries(ctx, guid)
	if err != nil {
		return domain.Invoice{}, err
	}
	if len(entries) == 0 {
		return domain.Invoice{}, domain.ErrInvoiceNoEntries
	}

	total := domain.Zero()
	for _, e := range entries {
		total = total.Add(e.LineTotal())
	}

	splits := make([]domain.Split, 0, len(entries)+1)
	if inv.Type == domain.InvoiceTypeCustomer {
		// Debit A/R, credit each income account.
		splits = append(splits, domain.Split{
			AccountGUID: req.PostAccGUID,
			Value:       total,
			Quantity:    total,
		})
		for _, e := range entries {
			lt := e.LineTotal()
			splits = append(splits, domain.Split{
				AccountGUID: e.AccountGUID,
				Value:       lt.Neg(),
				Quantity:    lt.Neg(),
			})
		}
	} else {
		// Debit each expense account, credit A/P.
		for _, e := range entries {
			lt := e.LineTotal()
			splits = append(splits, domain.Split{
				AccountGUID: e.AccountGUID,
				Value:       lt,
				Quantity:    lt,
			})
		}
		splits = append(splits, domain.Split{
			AccountGUID: req.PostAccGUID,
			Value:       total.Neg(),
			Quantity:    total.Neg(),
		})
	}

	postDate := req.PostDate
	if postDate.IsZero() {
		postDate = s.now().UTC().Truncate(24 * time.Hour)
	}

	label := "Invoice"
	if inv.Type == domain.InvoiceTypeBill {
		label = "Bill"
	}
	description := label
	if inv.ID != "" {
		description = fmt.Sprintf("%s %s", label, inv.ID)
	}

	tx := domain.Transaction{
		CurrencyGUID: inv.CurrencyGUID,
		PostDate:     postDate,
		Description:  description,
		Splits:       splits,
	}
	actor := AuditActor{UserID: userID, BookGUID: inv.BookGUID}
	posted, err := s.posting.Post(ctx, tx, actor)
	if err != nil {
		return domain.Invoice{}, err
	}

	if err := s.repo.MarkInvoicePosted(ctx, guid, posted.GUID, req.PostAccGUID, &postDate, req.DueDate); err != nil {
		return domain.Invoice{}, err
	}

	inv.DatePosted = &postDate
	inv.DateDue = req.DueDate
	inv.PostTxnGUID = posted.GUID
	inv.PostAccGUID = req.PostAccGUID
	inv.Entries = entries
	return inv, nil
}
