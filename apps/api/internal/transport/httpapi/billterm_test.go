package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// fakeBillTermRepoH satisfies app.BillTermRepository for HTTP-layer tests.
type fakeBillTermRepoH struct {
	*fakeRepo
	terms map[string]domain.BillTerm
}

func newFakeBillTermRepoH() *fakeBillTermRepoH {
	return &fakeBillTermRepoH{fakeRepo: &fakeRepo{}, terms: make(map[string]domain.BillTerm)}
}

func (f *fakeBillTermRepoH) CreateBillTerm(_ context.Context, t domain.BillTerm) (domain.BillTerm, error) {
	f.terms[t.GUID] = t
	return t, nil
}

func (f *fakeBillTermRepoH) ListBillTerms(_ context.Context, bookGUID string) ([]domain.BillTerm, error) {
	var out []domain.BillTerm
	for _, t := range f.terms {
		if t.BookGUID == bookGUID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeBillTermRepoH) GetBillTerm(_ context.Context, guid string) (domain.BillTerm, error) {
	t, ok := f.terms[guid]
	if !ok {
		return domain.BillTerm{}, domain.ErrBillTermNotFound
	}
	return t, nil
}

func (f *fakeBillTermRepoH) UpdateBillTerm(_ context.Context, t domain.BillTerm) (domain.BillTerm, error) {
	if _, ok := f.terms[t.GUID]; !ok {
		return domain.BillTerm{}, domain.ErrBillTermNotFound
	}
	f.terms[t.GUID] = t
	return t, nil
}

func (f *fakeBillTermRepoH) DeleteBillTerm(_ context.Context, guid string) error {
	if _, ok := f.terms[guid]; !ok {
		return domain.ErrBillTermNotFound
	}
	delete(f.terms, guid)
	return nil
}

func (f *fakeBillTermRepoH) BookGUIDForBillTerm(_ context.Context, guid string) (string, error) {
	t, ok := f.terms[guid]
	if !ok {
		return "", domain.ErrBillTermNotFound
	}
	return t.BookGUID, nil
}

func newBillTermTestServer(fr *fakeBillTermRepoH) http.Handler {
	posting := app.NewPostingService(fr.fakeRepo)
	return NewServer(
		posting,
		app.NewLedgerService(fr.fakeRepo),
		app.NewStructureService(fr.fakeRepo),
		app.NewPriceService(fr.fakeRepo),
		app.NewReportService(fr.fakeRepo),
		app.NewForecastService(fr.fakeRepo),
		app.NewProvisionService(fr.fakeRepo),
		app.NewAuthzService(fr.fakeRepo),
		app.NewImportService(fr.fakeRepo, fr.fakeRepo),
		app.NewExportService(fr.fakeRepo, &fakeWriter{}),
		app.NewReconcileService(fr.fakeRepo),
		app.NewPortfolioService(fr.fakeRepo),
		app.NewTradeService(fr.fakeRepo, posting),
		app.NewCapitalGainsService(fr.fakeRepo),
		nil,
		nil,
		nil,
		nil,
		nil,
		app.NewBillTermService(fr),
		nil,
		nil,
		nil,
		nil,
		nil,
	).Routes()
}

func billTermReq(h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req = withAuth(req)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestBillTermCRUD(t *testing.T) {
	const book = "book-1"

	t.Run("create then get a net-days term round-trips", func(t *testing.T) {
		h := newBillTermTestServer(newFakeBillTermRepoH())
		rr := billTermReq(h, http.MethodPost, "/api/v1/books/"+book+"/bill-terms", map[string]any{
			"name":    "Net 30",
			"type":    "days",
			"dueDays": 30,
		})
		if rr.Code != http.StatusCreated {
			t.Fatalf("create: got %d, want 201 (%s)", rr.Code, rr.Body.String())
		}
		var created map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &created)
		guid, _ := created["guid"].(string)
		if guid == "" {
			t.Fatal("create: expected a generated guid")
		}

		rr = billTermReq(h, http.MethodGet, "/api/v1/bill-terms/"+guid, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("get: got %d, want 200 (%s)", rr.Code, rr.Body.String())
		}
		var got map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &got)
		if got["name"] != "Net 30" || got["type"] != "days" {
			t.Errorf("get: unexpected body %v", got)
		}
	})

	t.Run("an invalid term type is rejected with 400", func(t *testing.T) {
		h := newBillTermTestServer(newFakeBillTermRepoH())
		rr := billTermReq(h, http.MethodPost, "/api/v1/books/"+book+"/bill-terms", map[string]any{
			"name":    "Bad",
			"type":    "fortnightly",
			"dueDays": 14,
		})
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400 (%s)", rr.Code, rr.Body.String())
		}
	})

	t.Run("getting an unknown term returns 404", func(t *testing.T) {
		h := newBillTermTestServer(newFakeBillTermRepoH())
		rr := billTermReq(h, http.MethodGet, "/api/v1/bill-terms/nope", nil)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("got %d, want 404 (%s)", rr.Code, rr.Body.String())
		}
	})

	t.Run("deleting a term returns 204 and removes it", func(t *testing.T) {
		fr := newFakeBillTermRepoH()
		fr.terms["t-1"] = domain.BillTerm{GUID: "t-1", BookGUID: book, Name: "Net 15", Type: domain.BillTermDays, DueDays: 15}
		h := newBillTermTestServer(fr)
		rr := billTermReq(h, http.MethodDelete, "/api/v1/bill-terms/t-1", nil)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("delete: got %d, want 204 (%s)", rr.Code, rr.Body.String())
		}
		rr = billTermReq(h, http.MethodGet, "/api/v1/bill-terms/t-1", nil)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("get after delete: got %d, want 404", rr.Code)
		}
	})
}
