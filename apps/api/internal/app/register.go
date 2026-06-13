package app

import (
	"context"
	"errors"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// ErrAccountNotFound is returned when a register is requested for an account
// that does not exist.
var ErrAccountNotFound = errors.New("account not found")

// RegisterEntry is one row of an account register: a split with the running
// balance (in the account's commodity) as of that split. ValueScale and
// QuantityScale carry the stored denominators (the currency / account-commodity
// fractions) so amounts can be rendered at their natural scale rather than as a
// reduced rational. The Balance shares QuantityScale.
type RegisterEntry struct {
	SplitGUID     string
	TxGUID        string
	PostDate      time.Time
	Description   string
	Memo          string
	Reconcile     domain.ReconcileState
	Value         domain.GncNumeric // in the transaction currency
	Quantity      domain.GncNumeric // in the account commodity
	Balance       domain.GncNumeric // running balance in the account commodity
	ValueScale    int64
	QuantityScale int64
}

// RegisterPage is a paginated account register.
type RegisterPage struct {
	AccountGUID string
	Total       int64
	Limit       int
	Offset      int
	Entries     []RegisterEntry
}

// LedgerRepository reads account ledger data.
type LedgerRepository interface {
	AccountExists(ctx context.Context, guid string) (bool, error)
	ListAccountRegister(ctx context.Context, guid string, limit, offset int) ([]RegisterEntry, int64, error)
}

// LedgerService serves read models over the ledger.
type LedgerService struct {
	repo LedgerRepository
}

// NewLedgerService builds a LedgerService backed by repo.
func NewLedgerService(repo LedgerRepository) *LedgerService {
	return &LedgerService{repo: repo}
}

// AccountRegister returns a page of the account's register, or
// ErrAccountNotFound if the account does not exist.
func (s *LedgerService) AccountRegister(ctx context.Context, guid string, limit, offset int) (RegisterPage, error) {
	exists, err := s.repo.AccountExists(ctx, guid)
	if err != nil {
		return RegisterPage{}, err
	}
	if !exists {
		return RegisterPage{}, ErrAccountNotFound
	}

	entries, total, err := s.repo.ListAccountRegister(ctx, guid, limit, offset)
	if err != nil {
		return RegisterPage{}, err
	}
	return RegisterPage{
		AccountGUID: guid,
		Total:       total,
		Limit:       limit,
		Offset:      offset,
		Entries:     entries,
	}, nil
}
