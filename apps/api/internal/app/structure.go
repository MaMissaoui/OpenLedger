package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// ErrBookNotFound is returned when an operation references a book that does not
// exist.
var ErrBookNotFound = errors.New("book not found")

// ErrInvalidInput is returned when a request fails a domain validation rule
// (missing required field, unknown account type, …). Handlers map it to 400.
var ErrInvalidInput = errors.New("invalid input")

// StructureRepository persists and reads the chart-of-accounts structure:
// commodities, books, and accounts.
type StructureRepository interface {
	InsertCommodity(ctx context.Context, c domain.Commodity) error
	// InsertBook writes a book together with its root and template-root accounts
	// in one DB transaction. When ownerUserID is non-empty it also records an
	// owner membership linking that user to the new book.
	InsertBook(ctx context.Context, b domain.Book, root, templateRoot domain.Account, ownerUserID string) error
	InsertAccount(ctx context.Context, a domain.Account) error
	// ListBooksForUser returns the books a user has a membership on.
	ListBooksForUser(ctx context.Context, userID string) ([]domain.Book, error)
	// BookRootAccount returns a book's root_account_guid, or ErrBookNotFound.
	BookRootAccount(ctx context.Context, bookGUID string) (string, error)
	// ListAccountsUnderRoot returns every descendant of rootGUID (the root
	// itself excluded), each with its balance, ordered by code then name.
	ListAccountsUnderRoot(ctx context.Context, rootGUID string) ([]AccountWithBalance, error)
}

// AccountWithBalance pairs an account with the sum of its own splits' quantity
// (in the account's own commodity). BalanceScale is the commodity fraction, so
// the transport layer can render the amount at its natural denominator, mirroring
// the register. An account with no splits has a zero balance. The balance is the
// account's own; subtree roll-ups for placeholder parents are not included.
type AccountWithBalance struct {
	Account      domain.Account
	Balance      domain.GncNumeric
	BalanceScale int64
}

// StructureService creates and reads books, commodities, and accounts — the
// scaffolding a ledger needs before any transaction can be posted.
type StructureService struct {
	repo    StructureRepository
	newGUID func() string
}

// NewStructureService builds a StructureService backed by repo.
func NewStructureService(repo StructureRepository) *StructureService {
	return &StructureService{repo: repo, newGUID: NewGUID}
}

// CreateCommodity creates a commodity. Mnemonic is required; Namespace defaults
// to CURRENCY and Fraction to 100 (cents) when unset.
func (s *StructureService) CreateCommodity(ctx context.Context, c domain.Commodity) (domain.Commodity, error) {
	if c.Mnemonic == "" {
		return domain.Commodity{}, fmt.Errorf("%w: mnemonic is required", ErrInvalidInput)
	}
	if c.Namespace == "" {
		c.Namespace = domain.NamespaceCurrency
	}
	if c.Fraction == 0 {
		c.Fraction = 100
	}
	if c.Fraction < 1 {
		return domain.Commodity{}, fmt.Errorf("%w: fraction must be positive", ErrInvalidInput)
	}
	c.GUID = s.newGUID()
	if err := s.repo.InsertCommodity(ctx, c); err != nil {
		return domain.Commodity{}, err
	}
	return c, nil
}

// CreateBook creates a book with a fresh root account and template root. When
// ownerUserID is non-empty the creator is recorded as the book's owner. The
// returned book's RootAccountGUID is the parent for top-level accounts.
func (s *StructureService) CreateBook(ctx context.Context, ownerUserID string) (domain.Book, error) {
	root := domain.Account{GUID: s.newGUID(), Name: "Root Account", Type: domain.AccountRoot}
	templateRoot := domain.Account{GUID: s.newGUID(), Name: "Template Root", Type: domain.AccountRoot}
	book := domain.Book{
		GUID:             s.newGUID(),
		RootAccountGUID:  root.GUID,
		RootTemplateGUID: templateRoot.GUID,
	}
	if err := s.repo.InsertBook(ctx, book, root, templateRoot, ownerUserID); err != nil {
		return domain.Book{}, err
	}
	return book, nil
}

// ListBooks returns the books the given user owns or is a member of.
func (s *StructureService) ListBooks(ctx context.Context, userID string) ([]domain.Book, error) {
	return s.repo.ListBooksForUser(ctx, userID)
}

// CreateAccount adds an account to a book. ParentGUID defaults to the book's
// root account when empty. A non-root account requires a commodity.
func (s *StructureService) CreateAccount(ctx context.Context, bookGUID string, a domain.Account) (domain.Account, error) {
	if bookGUID == "" {
		return domain.Account{}, fmt.Errorf("%w: bookGuid is required", ErrInvalidInput)
	}
	if a.Name == "" {
		return domain.Account{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if !a.Type.IsValid() {
		return domain.Account{}, fmt.Errorf("%w: unknown account type %q", ErrInvalidInput, a.Type)
	}
	if a.Type != domain.AccountRoot && a.CommodityGUID == "" {
		return domain.Account{}, fmt.Errorf("%w: commodityGuid is required", ErrInvalidInput)
	}

	root, err := s.repo.BookRootAccount(ctx, bookGUID)
	if err != nil {
		return domain.Account{}, err
	}
	if a.ParentGUID == "" {
		a.ParentGUID = root
	}

	a.GUID = s.newGUID()
	if err := s.repo.InsertAccount(ctx, a); err != nil {
		return domain.Account{}, err
	}
	return a, nil
}

// ListAccounts returns a book's chart of accounts (its root excluded) with each
// account's balance, or ErrBookNotFound if the book does not exist.
func (s *StructureService) ListAccounts(ctx context.Context, bookGUID string) ([]AccountWithBalance, error) {
	root, err := s.repo.BookRootAccount(ctx, bookGUID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListAccountsUnderRoot(ctx, root)
}
