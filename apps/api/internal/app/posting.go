// Package app holds use-case services that orchestrate the domain kernel and
// persistence. It depends on domain and on repository ports defined here; the
// concrete pgx implementations live in internal/infra.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// ErrTransactionNotFound is returned when an edit or delete references a
// transaction that does not exist. Handlers map it to 404.
var ErrTransactionNotFound = errors.New("transaction not found")

// AuditActor identifies who performed an action, recorded in audit_log. Fields
// may be empty in the current scaffold until auth is wired in.
type AuditActor struct {
	UserID   string
	BookGUID string
}

// TransactionRepository persists validated transactions atomically: each write
// touches the transaction row, its splits, and an audit_log entry in one DB
// transaction.
type TransactionRepository interface {
	InsertTransaction(ctx context.Context, tx domain.Transaction, actor AuditActor) error
	// UpdateTransaction replaces an existing transaction's fields and splits.
	// It returns ErrTransactionNotFound if the GUID is unknown.
	UpdateTransaction(ctx context.Context, tx domain.Transaction, actor AuditActor) error
	// DeleteTransaction removes a transaction and its splits. It returns
	// ErrTransactionNotFound if the GUID is unknown.
	DeleteTransaction(ctx context.Context, guid string, actor AuditActor) error
	// TransactionAccountGUIDs returns the distinct account GUIDs the
	// transaction's splits post to, for authorization. It returns
	// ErrTransactionNotFound if the GUID is unknown.
	TransactionAccountGUIDs(ctx context.Context, guid string) ([]string, error)
}

// PostingService is the single write path for transactions. now and newGUID are
// injectable so the service is deterministic under test.
type PostingService struct {
	repo    TransactionRepository
	now     func() time.Time
	newGUID func() string
	trading TradingBalancer
}

// NewPostingService builds a PostingService backed by repo.
func NewPostingService(repo TransactionRepository) *PostingService {
	return &PostingService{repo: repo, now: time.Now, newGUID: NewGUID}
}

// WithTrading enables GnuCash-style trading-account balancing and returns the
// service, so it can be chained onto the constructor.
func (s *PostingService) WithTrading(b TradingBalancer) *PostingService {
	s.trading = b
	return s
}

// applyTrading adds trading-account splits so the book balances in every
// commodity. It must run only after the value balance is validated, so it never
// masks a real imbalance — it only zeroes out each commodity's net quantity and
// value.
func (s *PostingService) applyTrading(ctx context.Context, tx *domain.Transaction) error {
	if s.trading == nil {
		return nil
	}
	splits, err := s.trading.Balance(ctx, *tx)
	if err != nil {
		return err
	}
	tx.Splits = splits
	return nil
}

// Post fills in missing GUIDs/dates, enforces the double-entry balance
// invariant, and persists the transaction. It returns the completed
// transaction, or a wrapped domain.ErrUnbalanced if the splits do not balance.
func (s *PostingService) Post(ctx context.Context, tx domain.Transaction, actor AuditActor) (domain.Transaction, error) {
	if tx.GUID == "" {
		tx.GUID = s.newGUID()
	}
	if tx.EnterDate.IsZero() {
		tx.EnterDate = s.now().UTC()
	}
	if tx.PostDate.IsZero() {
		tx.PostDate = tx.EnterDate
	}
	for i := range tx.Splits {
		if tx.Splits[i].GUID == "" {
			tx.Splits[i].GUID = s.newGUID()
		}
		if tx.Splits[i].Reconcile == 0 {
			tx.Splits[i].Reconcile = domain.ReconcileNew
		}
	}

	if err := tx.ValidateBalanced(); err != nil {
		return domain.Transaction{}, err
	}
	if err := s.applyTrading(ctx, &tx); err != nil {
		return domain.Transaction{}, err
	}
	if err := s.repo.InsertTransaction(ctx, tx, actor); err != nil {
		return domain.Transaction{}, err
	}
	return tx, nil
}

// Update replaces an existing transaction (identified by tx.GUID) and its
// splits, re-enforcing the balance invariant. The original enter_date is
// preserved by the repository; only the fields and splits change. It returns a
// wrapped domain.ErrUnbalanced if the new splits do not balance, or
// ErrTransactionNotFound if the GUID is unknown.
func (s *PostingService) Update(ctx context.Context, tx domain.Transaction, actor AuditActor) (domain.Transaction, error) {
	if tx.GUID == "" {
		return domain.Transaction{}, ErrTransactionNotFound
	}
	if tx.PostDate.IsZero() {
		tx.PostDate = s.now().UTC()
	}
	for i := range tx.Splits {
		if tx.Splits[i].GUID == "" {
			tx.Splits[i].GUID = s.newGUID()
		}
		if tx.Splits[i].Reconcile == 0 {
			tx.Splits[i].Reconcile = domain.ReconcileNew
		}
	}

	if err := tx.ValidateBalanced(); err != nil {
		return domain.Transaction{}, err
	}
	if err := s.applyTrading(ctx, &tx); err != nil {
		return domain.Transaction{}, err
	}
	if err := s.repo.UpdateTransaction(ctx, tx, actor); err != nil {
		return domain.Transaction{}, err
	}
	return tx, nil
}

// Delete removes a transaction and its splits. It returns ErrTransactionNotFound
// if the GUID is unknown.
func (s *PostingService) Delete(ctx context.Context, guid string, actor AuditActor) error {
	if guid == "" {
		return ErrTransactionNotFound
	}
	return s.repo.DeleteTransaction(ctx, guid, actor)
}

// TransactionAccounts returns the account GUIDs a transaction's splits touch, so
// the transport layer can authorize an edit or delete against those accounts'
// book. It returns ErrTransactionNotFound if the GUID is unknown.
func (s *PostingService) TransactionAccounts(ctx context.Context, guid string) ([]string, error) {
	return s.repo.TransactionAccountGUIDs(ctx, guid)
}

// NewGUID returns a random 32-char hex GUID, matching GnuCash's id format.
func NewGUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("guid: %v", err))
	}
	return hex.EncodeToString(b[:])
}
