package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// reconcileFake satisfies app.ReconcileRepository. The authz floor (split
// lookup, role) is supplied separately via an authStub.
type reconcileFake struct {
	reconciledSplit string
	reconciledState domain.ReconcileState
	reconciledDate  *time.Time
}

func (f *reconcileFake) SetSplitReconcile(_ context.Context, splitGUID string, state domain.ReconcileState, date *time.Time) error {
	f.reconciledSplit = splitGUID
	f.reconciledState = state
	f.reconciledDate = date
	return nil
}

func reconcileServer(f *reconcileFake, authz *app.AuthzService) http.Handler {
	return authedServer(Services{Reconciler: app.NewReconcileService(f), Authz: authz})
}

func reconcile(h http.Handler, splitGUID, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodPatch,
		"/api/v1/splits/"+splitGUID+"/reconcile", strings.NewReader(body)))
	h.ServeHTTP(rec, req)
	return rec
}

func TestReconcileSplit(t *testing.T) {
	repo := &reconcileFake{}
	rec := reconcile(reconcileServer(repo, nil), "s1", `{"state":"y"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if repo.reconciledSplit != "s1" || repo.reconciledState != domain.ReconcileYes {
		t.Errorf("wrote split=%q state=%q", repo.reconciledSplit, repo.reconciledState)
	}
	if repo.reconciledDate == nil {
		t.Error("reconciled state should stamp a date")
	}
}

func TestReconcileSplitInvalidStateReturns400(t *testing.T) {
	repo := &reconcileFake{}
	rec := reconcile(reconcileServer(repo, nil), "s1", `{"state":"z"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
	if repo.reconciledSplit != "" {
		t.Error("invalid state must not be persisted")
	}
}

func TestReconcileSplitMultiCharStateReturns400(t *testing.T) {
	rec := reconcile(reconcileServer(&reconcileFake{}, nil), "s1", `{"state":"yes"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestReconcileSplitUnknownReturns404(t *testing.T) {
	repo := &reconcileFake{}
	rec := reconcile(reconcileServer(repo, app.NewAuthzService(&authStub{splitUnknown: true})), "missing", `{"state":"c"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
}

func TestReconcileSplitViewerReturns403(t *testing.T) {
	repo := &reconcileFake{}
	rec := reconcile(reconcileServer(repo, app.NewAuthzService(&authStub{role: "viewer"})), "s1", `{"state":"c"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
	if repo.reconciledSplit != "" {
		t.Error("a viewer must not reconcile")
	}
}
