package app

import (
	"context"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// CustomerRepository is the persistence port for customers.
type CustomerRepository interface {
	ListCustomers(ctx context.Context, bookGUID string, activeOnly bool) ([]domain.Customer, error)
	CreateCustomer(ctx context.Context, c domain.Customer) error
	GetCustomer(ctx context.Context, guid string) (domain.Customer, error)
	UpdateCustomer(ctx context.Context, c domain.Customer) error
	DeleteCustomer(ctx context.Context, guid string) error
}

// CustomerService manages customers within a book.
type CustomerService struct {
	repo    CustomerRepository
	authz   *AuthzService
	newGUID func() string
}

// NewCustomerService creates a CustomerService.
func NewCustomerService(repo CustomerRepository, authz *AuthzService) *CustomerService {
	return &CustomerService{repo: repo, authz: authz, newGUID: NewGUID}
}

// ListCustomers returns all customers for a book visible to the caller.
func (s *CustomerService) ListCustomers(ctx context.Context, bookGUID, userID string, activeOnly bool) ([]domain.Customer, error) {
	if err := s.authz.AuthorizeBook(ctx, userID, bookGUID, AccessRead); err != nil {
		return nil, err
	}
	return s.repo.ListCustomers(ctx, bookGUID, activeOnly)
}

// CreateCustomer adds a new customer to a book.
func (s *CustomerService) CreateCustomer(ctx context.Context, userID string, c domain.Customer) (domain.Customer, error) {
	if err := s.authz.AuthorizeBook(ctx, userID, c.BookGUID, AccessWrite); err != nil {
		return domain.Customer{}, err
	}
	if c.Name == "" {
		return domain.Customer{}, ErrInvalidInput
	}
	c.GUID = s.newGUID()
	c.Active = true
	if err := s.repo.CreateCustomer(ctx, c); err != nil {
		return domain.Customer{}, err
	}
	return s.repo.GetCustomer(ctx, c.GUID)
}

// GetCustomer returns a single customer by GUID.
func (s *CustomerService) GetCustomer(ctx context.Context, guid, userID string) (domain.Customer, error) {
	c, err := s.repo.GetCustomer(ctx, guid)
	if err != nil {
		return domain.Customer{}, err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, c.BookGUID, AccessRead); err != nil {
		return domain.Customer{}, err
	}
	return c, nil
}

// UpdateCustomer patches a customer's mutable fields.
func (s *CustomerService) UpdateCustomer(ctx context.Context, userID string, c domain.Customer) (domain.Customer, error) {
	existing, err := s.repo.GetCustomer(ctx, c.GUID)
	if err != nil {
		return domain.Customer{}, err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, existing.BookGUID, AccessWrite); err != nil {
		return domain.Customer{}, err
	}
	if c.Name == "" {
		return domain.Customer{}, ErrInvalidInput
	}
	c.BookGUID = existing.BookGUID
	c.CreatedAt = existing.CreatedAt
	if err := s.repo.UpdateCustomer(ctx, c); err != nil {
		return domain.Customer{}, err
	}
	return s.repo.GetCustomer(ctx, c.GUID)
}

// DeleteCustomer removes a customer. Returns ErrCustomerNotFound if it does not exist.
func (s *CustomerService) DeleteCustomer(ctx context.Context, guid, userID string) error {
	existing, err := s.repo.GetCustomer(ctx, guid)
	if err != nil {
		return err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, existing.BookGUID, AccessWrite); err != nil {
		return err
	}
	return s.repo.DeleteCustomer(ctx, guid)
}
