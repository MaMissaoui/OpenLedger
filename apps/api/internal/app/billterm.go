package app

import (
	"context"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// BillTermRepository persists payment terms. GetBillTerm is shared with
// InvoiceRepository (one pg implementation satisfies both).
type BillTermRepository interface {
	CreateBillTerm(ctx context.Context, t domain.BillTerm) (domain.BillTerm, error)
	ListBillTerms(ctx context.Context, bookGUID string) ([]domain.BillTerm, error)
	GetBillTerm(ctx context.Context, guid string) (domain.BillTerm, error)
	UpdateBillTerm(ctx context.Context, t domain.BillTerm) (domain.BillTerm, error)
	DeleteBillTerm(ctx context.Context, guid string) error
	BookGUIDForBillTerm(ctx context.Context, guid string) (string, error)
}

// BillTermService manages the lifecycle of payment terms for a book.
type BillTermService struct {
	repo    BillTermRepository
	newGUID func() string
}

// NewBillTermService builds a BillTermService.
func NewBillTermService(repo BillTermRepository) *BillTermService {
	return &BillTermService{repo: repo, newGUID: NewGUID}
}

// Create validates and persists a new payment term.
func (s *BillTermService) Create(ctx context.Context, t domain.BillTerm) (domain.BillTerm, error) {
	if t.Name == "" {
		return domain.BillTerm{}, ErrInvalidInput
	}
	if err := t.Validate(); err != nil {
		return domain.BillTerm{}, err
	}
	if t.GUID == "" {
		t.GUID = s.newGUID()
	}
	return s.repo.CreateBillTerm(ctx, t)
}

// List returns all payment terms for a book.
func (s *BillTermService) List(ctx context.Context, bookGUID string) ([]domain.BillTerm, error) {
	return s.repo.ListBillTerms(ctx, bookGUID)
}

// Get returns a single payment term.
func (s *BillTermService) Get(ctx context.Context, guid string) (domain.BillTerm, error) {
	return s.repo.GetBillTerm(ctx, guid)
}

// Update validates and replaces a payment term's fields.
func (s *BillTermService) Update(ctx context.Context, t domain.BillTerm) (domain.BillTerm, error) {
	if t.Name == "" {
		return domain.BillTerm{}, ErrInvalidInput
	}
	if err := t.Validate(); err != nil {
		return domain.BillTerm{}, err
	}
	return s.repo.UpdateBillTerm(ctx, t)
}

// Delete removes a payment term.
func (s *BillTermService) Delete(ctx context.Context, guid string) error {
	return s.repo.DeleteBillTerm(ctx, guid)
}

// BookGUIDForBillTerm returns the book a term belongs to (for authz).
func (s *BillTermService) BookGUIDForBillTerm(ctx context.Context, guid string) (string, error) {
	return s.repo.BookGUIDForBillTerm(ctx, guid)
}
