package app

import (
	"context"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// TaxTableRepository persists tax tables and their entries. GetTaxTable is
// shared with InvoiceRepository (one pg implementation satisfies both).
type TaxTableRepository interface {
	CreateTaxTable(ctx context.Context, tt domain.TaxTable) (domain.TaxTable, error)
	ListTaxTables(ctx context.Context, bookGUID string) ([]domain.TaxTable, error)
	GetTaxTable(ctx context.Context, guid string) (domain.TaxTable, error)
	UpdateTaxTable(ctx context.Context, tt domain.TaxTable) (domain.TaxTable, error)
	DeleteTaxTable(ctx context.Context, guid string) error
	BookGUIDForTaxTable(ctx context.Context, guid string) (string, error)
}

// TaxTableService manages the lifecycle of tax tables for a book.
type TaxTableService struct {
	repo    TaxTableRepository
	newGUID func() string
}

// NewTaxTableService builds a TaxTableService.
func NewTaxTableService(repo TaxTableRepository) *TaxTableService {
	return &TaxTableService{repo: repo, newGUID: NewGUID}
}

// Create validates and persists a new tax table.
func (s *TaxTableService) Create(ctx context.Context, tt domain.TaxTable) (domain.TaxTable, error) {
	if tt.Name == "" {
		return domain.TaxTable{}, ErrInvalidInput
	}
	if err := tt.Validate(); err != nil {
		return domain.TaxTable{}, err
	}
	if tt.GUID == "" {
		tt.GUID = s.newGUID()
	}
	for i := range tt.Entries {
		if tt.Entries[i].Type == "" {
			tt.Entries[i].Type = domain.TaxPercentage
		}
	}
	return s.repo.CreateTaxTable(ctx, tt)
}

// List returns all tax tables for a book.
func (s *TaxTableService) List(ctx context.Context, bookGUID string) ([]domain.TaxTable, error) {
	return s.repo.ListTaxTables(ctx, bookGUID)
}

// Get returns a single tax table with its entries.
func (s *TaxTableService) Get(ctx context.Context, guid string) (domain.TaxTable, error) {
	return s.repo.GetTaxTable(ctx, guid)
}

// Update validates and replaces a tax table's fields and entries.
func (s *TaxTableService) Update(ctx context.Context, tt domain.TaxTable) (domain.TaxTable, error) {
	if tt.Name == "" {
		return domain.TaxTable{}, ErrInvalidInput
	}
	if err := tt.Validate(); err != nil {
		return domain.TaxTable{}, err
	}
	return s.repo.UpdateTaxTable(ctx, tt)
}

// Delete removes a tax table and its entries.
func (s *TaxTableService) Delete(ctx context.Context, guid string) error {
	return s.repo.DeleteTaxTable(ctx, guid)
}

// BookGUIDForTaxTable returns the book a tax table belongs to (for authz).
func (s *TaxTableService) BookGUIDForTaxTable(ctx context.Context, guid string) (string, error) {
	return s.repo.BookGUIDForTaxTable(ctx, guid)
}
