package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/app"
)

func exportGnuCash(h http.Handler, bookGUID string) *httptest.ResponseRecorder {
	return exportGnuCashFormat(h, bookGUID, "")
}

func exportGnuCashFormat(h http.Handler, bookGUID, format string) *httptest.ResponseRecorder {
	url := "/api/v1/books/" + bookGUID + "/export/gnucash"
	if format != "" {
		url += "?format=" + format
	}
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodGet, url, nil))
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

func TestExportGnuCashXML(t *testing.T) {
	repo := &fakeRepo{loadBookData: importableData()}
	rec := exportGnuCashFormat(newTestServer(repo), "book1", "xml")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/xml" {
		t.Errorf("Content-Type = %q, want application/xml", got)
	}
	// The fake writer records the format it was asked for in the body, so this
	// confirms the handler dispatched to the XML writer rather than SQLite.
	if got := rec.Body.String(); got != "gnucash-export:xml" {
		t.Errorf("body = %q, want gnucash-export:xml", got)
	}
}

func TestExportGnuCashUnknownFormatReturns400(t *testing.T) {
	repo := &fakeRepo{loadBookData: importableData()}
	rec := exportGnuCashFormat(newTestServer(repo), "book1", "csv")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
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
