package app

import (
	"context"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// VendorRepository is the persistence port for vendors.
type VendorRepository interface {
	ListVendors(ctx context.Context, bookGUID string, activeOnly bool) ([]domain.Vendor, error)
	CreateVendor(ctx context.Context, v domain.Vendor) error
	GetVendor(ctx context.Context, guid string) (domain.Vendor, error)
	UpdateVendor(ctx context.Context, v domain.Vendor) error
	DeleteVendor(ctx context.Context, guid string) error
}

// VendorService manages vendors within a book.
type VendorService struct {
	repo    VendorRepository
	authz   *AuthzService
	newGUID func() string
}

// NewVendorService creates a VendorService.
func NewVendorService(repo VendorRepository, authz *AuthzService) *VendorService {
	return &VendorService{repo: repo, authz: authz, newGUID: NewGUID}
}

// ListVendors returns all vendors for a book.
func (s *VendorService) ListVendors(ctx context.Context, bookGUID, userID string, activeOnly bool) ([]domain.Vendor, error) {
	if err := s.authz.AuthorizeBook(ctx, userID, bookGUID, AccessRead); err != nil {
		return nil, err
	}
	return s.repo.ListVendors(ctx, bookGUID, activeOnly)
}

// CreateVendor adds a new vendor to a book.
func (s *VendorService) CreateVendor(ctx context.Context, userID string, v domain.Vendor) (domain.Vendor, error) {
	if err := s.authz.AuthorizeBook(ctx, userID, v.BookGUID, AccessWrite); err != nil {
		return domain.Vendor{}, err
	}
	if v.Name == "" {
		return domain.Vendor{}, ErrInvalidInput
	}
	v.GUID = s.newGUID()
	v.Active = true
	if err := s.repo.CreateVendor(ctx, v); err != nil {
		return domain.Vendor{}, err
	}
	return s.repo.GetVendor(ctx, v.GUID)
}

// GetVendor returns a single vendor by GUID.
func (s *VendorService) GetVendor(ctx context.Context, guid, userID string) (domain.Vendor, error) {
	v, err := s.repo.GetVendor(ctx, guid)
	if err != nil {
		return domain.Vendor{}, err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, v.BookGUID, AccessRead); err != nil {
		return domain.Vendor{}, err
	}
	return v, nil
}

// UpdateVendor patches a vendor's mutable fields.
func (s *VendorService) UpdateVendor(ctx context.Context, userID string, v domain.Vendor) (domain.Vendor, error) {
	existing, err := s.repo.GetVendor(ctx, v.GUID)
	if err != nil {
		return domain.Vendor{}, err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, existing.BookGUID, AccessWrite); err != nil {
		return domain.Vendor{}, err
	}
	if v.Name == "" {
		return domain.Vendor{}, ErrInvalidInput
	}
	v.BookGUID = existing.BookGUID
	v.CreatedAt = existing.CreatedAt
	if err := s.repo.UpdateVendor(ctx, v); err != nil {
		return domain.Vendor{}, err
	}
	return s.repo.GetVendor(ctx, v.GUID)
}

// DeleteVendor removes a vendor. Returns ErrVendorNotFound if it does not exist.
func (s *VendorService) DeleteVendor(ctx context.Context, guid, userID string) error {
	existing, err := s.repo.GetVendor(ctx, guid)
	if err != nil {
		return err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, existing.BookGUID, AccessWrite); err != nil {
		return err
	}
	return s.repo.DeleteVendor(ctx, guid)
}
