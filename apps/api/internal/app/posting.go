// Package app holds use-case services that orchestrate the domain kernel and
// persistence. It depends on domain and on repository ports defined here; the
// concrete pgx implementations live in internal/infra.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// AuditActor identifies who performed an action, recorded in audit_log. Fields
// may be empty in the current scaffold until auth is wired in.
type AuditActor struct {
	UserID   string
	BookGUID string
}

// TransactionRepository persists a validated transaction atomically: the
// transaction row, its splits, and an audit_log entry in one DB transaction.
type TransactionRepository interface {
	InsertTransaction(ctx context.Context, tx domain.Transaction, actor AuditActor) error
}

// PostingService is the single write path for transactions. now and newGUID are
// injectable so the service is deterministic under test.
type PostingService struct {
	repo    TransactionRepository
	now     func() time.Time
	newGUID func() string
}

// NewPostingService builds a PostingService backed by repo.
func NewPostingService(repo TransactionRepository) *PostingService {
	return &PostingService{repo: repo, now: time.Now, newGUID: NewGUID}
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
	if err := s.repo.InsertTransaction(ctx, tx, actor); err != nil {
		return domain.Transaction{}, err
	}
	return tx, nil
}

// NewGUID returns a random 32-char hex GUID, matching GnuCash's id format.
func NewGUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("guid: %v", err))
	}
	return hex.EncodeToString(b[:])
}
