package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/app"
)

func exportGnuCash(h http.Handler, bookGUID string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodGet, "/api/v1/books/"+bookGUID+"/export/gnucash", nil))
	h.ServeHTTP(rec, req)
	return rec
}

func TestExportGnuCash(t *testing.T) {
	repo := &fakeRepo{loadBookData: importableData()}
	rec := exportGnuCash(newTestServer(repo), "book1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="book1.gnucash"` {
		t.Errorf("Content-Disposition = %q", got)
	}
	if rec.Body.Len() == 0 {
		t.Error("export body is empty")
	}
}

func TestExportGnuCashBookNotFoundReturns404(t *testing.T) {
	repo := &fakeRepo{loadBookErr: app.ErrBookNotFound}
	rec := exportGnuCash(newTestServer(repo), "missing")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
}

func TestExportGnuCashForbiddenReturns403(t *testing.T) {
	repo := &fakeRepo{noMembership: true, loadBookData: importableData()}
	rec := exportGnuCash(newTestServer(repo), "book1")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}
