package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

type fakeReconcileRepo struct {
	gotGUID  string
	gotState domain.ReconcileState
	gotDate  *time.Time
	err      error
}

func (r *fakeReconcileRepo) SetSplitReconcile(_ context.Context, guid string, state domain.ReconcileState, date *time.Time) error {
	if r.err != nil {
		return r.err
	}
	r.gotGUID, r.gotState, r.gotDate = guid, state, date
	return nil
}

func TestSetReconcileStampsDate(t *testing.T) {
	repo := &fakeReconcileRepo{}
	svc := NewReconcileService(repo)

	if err := svc.SetReconcile(context.Background(), "s1", domain.ReconcileYes); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if repo.gotGUID != "s1" || repo.gotState != domain.ReconcileYes {
		t.Errorf("wrote guid=%q state=%q", repo.gotGUID, repo.gotState)
	}
	if repo.gotDate == nil {
		t.Error("reconciled state should stamp a date")
	}
}

func TestSetReconcileClearsDateForNew(t *testing.T) {
	repo := &fakeReconcileRepo{}
	svc := NewReconcileService(repo)

	if err := svc.SetReconcile(context.Background(), "s1", domain.ReconcileNew); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if repo.gotDate != nil {
		t.Errorf("unmarked state should not stamp a date, got %v", repo.gotDate)
	}
}

func TestSetReconcileRejectsInvalidState(t *testing.T) {
	repo := &fakeReconcileRepo{}
	svc := NewReconcileService(repo)

	err := svc.SetReconcile(context.Background(), "s1", domain.ReconcileState('z'))
	if !errors.Is(err, ErrInvalidReconcileState) {
		t.Fatalf("err = %v, want ErrInvalidReconcileState", err)
	}
	if repo.gotGUID != "" {
		t.Error("invalid state must not reach the repository")
	}
}

func TestSetReconcilePropagatesNotFound(t *testing.T) {
	repo := &fakeReconcileRepo{err: ErrSplitNotFound}
	svc := NewReconcileService(repo)

	err := svc.SetReconcile(context.Background(), "missing", domain.ReconcileCleared)
	if !errors.Is(err, ErrSplitNotFound) {
		t.Fatalf("err = %v, want ErrSplitNotFound", err)
	}
}
