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

// ErrCommodityNotFound is returned when an operation references a commodity GUID
// that does not exist. Handlers map it to 404.
var ErrCommodityNotFound = errors.New("commodity not found")

// StructureRepository persists and reads the chart-of-accounts structure:
// commodities, books, and accounts.
type StructureRepository interface {
	InsertCommodity(ctx context.Context, c domain.Commodity) error
	// ListCommodities returns all commodities (shared reference data, not
	// book-scoped), ordered by namespace then mnemonic.
	ListCommodities(ctx context.Context) ([]domain.Commodity, error)
	// InsertBook writes a book together with its root and template-root accounts
	// in one DB transaction. When ownerUserID is non-empty it also records an
	// owner membership linking that user to the new book.
	InsertBook(ctx context.Context, b domain.Book, root, templateRoot domain.Account, ownerUserID string) error
	// UpdateBook persists a book's name and currency_guid. Returns ErrBookNotFound
	// when the guid is unknown.
	UpdateBook(ctx context.Context, b domain.Book) error
	InsertAccount(ctx context.Context, a domain.Account) error
	// ListBooksForUser returns the books a user has a membership on.
	ListBooksForUser(ctx context.Context, userID string) ([]domain.Book, error)
	// BookRootAccount returns a book's root_account_guid, or ErrBookNotFound.
	BookRootAccount(ctx context.Context, bookGUID string) (string, error)
	// ListAccountsUnderRoot returns every descendant of rootGUID (the root
	// itself excluded), each with its balance, ordered by code then name.
	ListAccountsUnderRoot(ctx context.Context, rootGUID string) ([]AccountWithBalance, error)
}

// AccountWithBalance pairs an account with its balances, both in the account's
// own commodity. Balance is the sum of the account's own splits; SubtreeBalance
// adds the balances of every descendant sharing this account's commodity (the
// roll-up shown against placeholder parents). BalanceScale is the commodity
// fraction, so the transport layer can render either amount at its natural
// denominator, mirroring the register. An account with no splits has a zero
// Balance. Descendants in a different commodity are not converted (there is no
// price engine yet), so they contribute only to same-commodity ancestors.
type AccountWithBalance struct {
	Account        domain.Account
	Balance        domain.GncNumeric
	SubtreeBalance domain.GncNumeric
	BalanceScale   int64
}

// rollUpSubtreeBalances sets SubtreeBalance on each account to its own balance
// plus the subtree balance of every child that shares its commodity. It walks
// the account forest (the rows exclude the book root, so top-level accounts are
// those whose parent is not itself in the slice) in post-order, so each parent
// sees its children's totals. The input slice is mutated in place.
func rollUpSubtreeBalances(accts []AccountWithBalance) {
	idx := make(map[string]int, len(accts))
	for i := range accts {
		idx[accts[i].Account.GUID] = i
	}
	children := make(map[string][]int, len(accts))
	var roots []int
	for i := range accts {
		if _, ok := idx[accts[i].Account.ParentGUID]; ok {
			children[accts[i].Account.ParentGUID] = append(children[accts[i].Account.ParentGUID], i)
		} else {
			roots = append(roots, i)
		}
	}
	var visit func(i int) domain.GncNumeric
	visit = func(i int) domain.GncNumeric {
		total := accts[i].Balance
		for _, c := range children[accts[i].Account.GUID] {
			sub := visit(c)
			if accts[c].Account.CommodityGUID == accts[i].Account.CommodityGUID {
				total = total.Add(sub)
			}
		}
		accts[i].SubtreeBalance = total
		return total
	}
	for _, r := range roots {
		visit(r)
	}
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

// ListCommodities returns all commodities, shared reference data used when
// creating accounts and posting prices. It is not scoped to a book.
func (s *StructureService) ListCommodities(ctx context.Context) ([]domain.Commodity, error) {
	return s.repo.ListCommodities(ctx)
}

// CreateBook creates a book with a fresh root account and template root. When
// ownerUserID is non-empty the creator is recorded as the book's owner. The
// returned book's RootAccountGUID is the parent for top-level accounts.
func (s *StructureService) CreateBook(ctx context.Context, ownerUserID, name, currencyGUID string) (domain.Book, error) {
	root := domain.Account{GUID: s.newGUID(), Name: "Root Account", Type: domain.AccountRoot}
	templateRoot := domain.Account{GUID: s.newGUID(), Name: "Template Root", Type: domain.AccountRoot}
	book := domain.Book{
		GUID:             s.newGUID(),
		Name:             name,
		CurrencyGUID:     currencyGUID,
		RootAccountGUID:  root.GUID,
		RootTemplateGUID: templateRoot.GUID,
	}
	if err := s.repo.InsertBook(ctx, book, root, templateRoot, ownerUserID); err != nil {
		return domain.Book{}, err
	}
	return book, nil
}

// UpdateBook renames a book or changes its home currency. Authorization is
// enforced by the HTTP layer before this is called.
func (s *StructureService) UpdateBook(ctx context.Context, b domain.Book) (domain.Book, error) {
	if err := s.repo.UpdateBook(ctx, b); err != nil {
		return domain.Book{}, err
	}
	return b, nil
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
	accts, err := s.repo.ListAccountsUnderRoot(ctx, root)
	if err != nil {
		return nil, err
	}
	rollUpSubtreeBalances(accts)
	return accts, nil
}
