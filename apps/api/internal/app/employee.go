package app

import (
	"context"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// EmployeeRepository is the persistence port for employees.
type EmployeeRepository interface {
	ListEmployees(ctx context.Context, bookGUID string, activeOnly bool) ([]domain.Employee, error)
	CreateEmployee(ctx context.Context, e domain.Employee) error
	GetEmployee(ctx context.Context, guid string) (domain.Employee, error)
	UpdateEmployee(ctx context.Context, e domain.Employee) error
	DeleteEmployee(ctx context.Context, guid string) error
}

// EmployeeService manages employees within a book.
type EmployeeService struct {
	repo    EmployeeRepository
	authz   *AuthzService
	newGUID func() string
}

// NewEmployeeService creates an EmployeeService.
func NewEmployeeService(repo EmployeeRepository, authz *AuthzService) *EmployeeService {
	return &EmployeeService{repo: repo, authz: authz, newGUID: NewGUID}
}

// ListEmployees returns all employees for a book.
func (s *EmployeeService) ListEmployees(ctx context.Context, bookGUID, userID string, activeOnly bool) ([]domain.Employee, error) {
	if err := s.authz.AuthorizeBook(ctx, userID, bookGUID, AccessRead); err != nil {
		return nil, err
	}
	return s.repo.ListEmployees(ctx, bookGUID, activeOnly)
}

// CreateEmployee adds a new employee to a book.
func (s *EmployeeService) CreateEmployee(ctx context.Context, userID string, e domain.Employee) (domain.Employee, error) {
	if err := s.authz.AuthorizeBook(ctx, userID, e.BookGUID, AccessWrite); err != nil {
		return domain.Employee{}, err
	}
	if e.Name == "" {
		return domain.Employee{}, ErrInvalidInput
	}
	e.GUID = s.newGUID()
	e.Active = true
	if err := s.repo.CreateEmployee(ctx, e); err != nil {
		return domain.Employee{}, err
	}
	return s.repo.GetEmployee(ctx, e.GUID)
}

// GetEmployee returns a single employee by GUID.
func (s *EmployeeService) GetEmployee(ctx context.Context, guid, userID string) (domain.Employee, error) {
	e, err := s.repo.GetEmployee(ctx, guid)
	if err != nil {
		return domain.Employee{}, err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, e.BookGUID, AccessRead); err != nil {
		return domain.Employee{}, err
	}
	return e, nil
}

// UpdateEmployee patches an employee's mutable fields.
func (s *EmployeeService) UpdateEmployee(ctx context.Context, userID string, e domain.Employee) (domain.Employee, error) {
	existing, err := s.repo.GetEmployee(ctx, e.GUID)
	if err != nil {
		return domain.Employee{}, err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, existing.BookGUID, AccessWrite); err != nil {
		return domain.Employee{}, err
	}
	if e.Name == "" {
		return domain.Employee{}, ErrInvalidInput
	}
	e.BookGUID = existing.BookGUID
	e.CreatedAt = existing.CreatedAt
	if err := s.repo.UpdateEmployee(ctx, e); err != nil {
		return domain.Employee{}, err
	}
	return s.repo.GetEmployee(ctx, e.GUID)
}

// DeleteEmployee removes an employee. Returns ErrEmployeeNotFound if it does not exist.
func (s *EmployeeService) DeleteEmployee(ctx context.Context, guid, userID string) error {
	existing, err := s.repo.GetEmployee(ctx, guid)
	if err != nil {
		return err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, existing.BookGUID, AccessWrite); err != nil {
		return err
	}
	return s.repo.DeleteEmployee(ctx, guid)
}
