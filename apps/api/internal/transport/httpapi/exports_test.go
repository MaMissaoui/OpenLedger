package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/app"
)

// exportFake satisfies app.ExportRepository; the GnuCashWriter is the shared
// fakeWriter stub.
type exportFake struct {
	loadBookData app.GnuCashData
	loadBookErr  error
}

func (f *exportFake) LoadBook(context.Context, string) (app.GnuCashData, error) {
	return f.loadBookData, f.loadBookErr
}

func exportServer(f *exportFake, authz *app.AuthzService) http.Handler {
	return authedServer(Services{Exporter: app.NewExportService(f, &fakeWriter{}), Authz: authz})
}

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
	repo := &exportFake{loadBookData: importableData()}
	rec := exportGnuCash(exportServer(repo, nil), "book1")

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
	repo := &exportFake{loadBookData: importableData()}
	rec := exportGnuCashFormat(exportServer(repo, nil), "book1", "xml")

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
	repo := &exportFake{loadBookData: importableData()}
	rec := exportGnuCashFormat(exportServer(repo, nil), "book1", "csv")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestExportGnuCashBookNotFoundReturns404(t *testing.T) {
	repo := &exportFake{loadBookErr: app.ErrBookNotFound}
	rec := exportGnuCash(exportServer(repo, nil), "missing")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
}

func TestExportGnuCashForbiddenReturns403(t *testing.T) {
	repo := &exportFake{loadBookData: importableData()}
	rec := exportGnuCash(exportServer(repo, app.NewAuthzService(&authStub{noMembership: true})), "book1")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}
