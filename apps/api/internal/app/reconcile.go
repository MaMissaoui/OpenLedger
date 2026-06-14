package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// ErrSplitNotFound is returned when an operation references a split that does
// not exist. Handlers map it to HTTP 404.
var ErrSplitNotFound = errors.New("split not found")

// ErrInvalidReconcileState is returned when a reconcile request carries a flag
// that is not a known GnuCash reconcile state. Handlers map it to HTTP 400.
var ErrInvalidReconcileState = errors.New("invalid reconcile state")

// ReconcileRepository updates the reconcile status of a single split. Marking a
// split reconciled does not change any amount, so it is a direct annotation
// rather than a transaction rewrite and does not go through PostingService.
type ReconcileRepository interface {
	// SetSplitReconcile sets a split's reconcile state and date (date is nil for
	// the unmarked state). It returns ErrSplitNotFound if the split is unknown.
	SetSplitReconcile(ctx context.Context, splitGUID string, state domain.ReconcileState, date *time.Time) error
}

// ReconcileService changes the reconcile status of splits — the core of
// statement reconciliation, where a user marks splits cleared or reconciled
// against a bank statement.
type ReconcileService struct {
	repo ReconcileRepository
	now  func() time.Time
}

// NewReconcileService builds a ReconcileService backed by repo.
func NewReconcileService(repo ReconcileRepository) *ReconcileService {
	return &ReconcileService{repo: repo, now: time.Now}
}

// SetReconcile moves a split to the given reconcile state, stamping the
// reconcile date for cleared/reconciled states and clearing it otherwise. It
// returns ErrInvalidReconcileState for an unknown flag and ErrSplitNotFound if
// the split does not exist.
func (s *ReconcileService) SetReconcile(ctx context.Context, splitGUID string, state domain.ReconcileState) error {
	if !state.IsValid() {
		return fmt.Errorf("%w: %q", ErrInvalidReconcileState, string(state))
	}
	var date *time.Time
	if state.SetsReconcileDate() {
		t := s.now().UTC()
		date = &t
	}
	return s.repo.SetSplitReconcile(ctx, splitGUID, state, date)
}
