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

// fakeTaxTableRepoH satisfies app.TaxTableRepository for HTTP-layer tests.
type fakeTaxTableRepoH struct {
	*fakeRepo
	tables map[string]domain.TaxTable
}

func newFakeTaxTableRepoH() *fakeTaxTableRepoH {
	return &fakeTaxTableRepoH{fakeRepo: &fakeRepo{}, tables: make(map[string]domain.TaxTable)}
}

func (f *fakeTaxTableRepoH) CreateTaxTable(_ context.Context, tt domain.TaxTable) (domain.TaxTable, error) {
	f.tables[tt.GUID] = tt
	return tt, nil
}

func (f *fakeTaxTableRepoH) ListTaxTables(_ context.Context, bookGUID string) ([]domain.TaxTable, error) {
	var out []domain.TaxTable
	for _, tt := range f.tables {
		if tt.BookGUID == bookGUID {
			out = append(out, tt)
		}
	}
	return out, nil
}

func (f *fakeTaxTableRepoH) GetTaxTable(_ context.Context, guid string) (domain.TaxTable, error) {
	tt, ok := f.tables[guid]
	if !ok {
		return domain.TaxTable{}, domain.ErrTaxTableNotFound
	}
	return tt, nil
}

func (f *fakeTaxTableRepoH) UpdateTaxTable(_ context.Context, tt domain.TaxTable) (domain.TaxTable, error) {
	if _, ok := f.tables[tt.GUID]; !ok {
		return domain.TaxTable{}, domain.ErrTaxTableNotFound
	}
	f.tables[tt.GUID] = tt
	return tt, nil
}

func (f *fakeTaxTableRepoH) DeleteTaxTable(_ context.Context, guid string) error {
	if _, ok := f.tables[guid]; !ok {
		return domain.ErrTaxTableNotFound
	}
	delete(f.tables, guid)
	return nil
}

func (f *fakeTaxTableRepoH) BookGUIDForTaxTable(_ context.Context, guid string) (string, error) {
	tt, ok := f.tables[guid]
	if !ok {
		return "", domain.ErrTaxTableNotFound
	}
	return tt.BookGUID, nil
}

func newTaxTableTestServer(fr *fakeTaxTableRepoH) http.Handler {
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
		nil,
		app.NewTaxTableService(fr),
		nil,
		nil,
	).Routes()
}

func taxTableReq(h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
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

func TestTaxTableCRUD(t *testing.T) {
	const book = "book-1"

	t.Run("create then get a VAT table round-trips with its entry", func(t *testing.T) {
		h := newTaxTableTestServer(newFakeTaxTableRepoH())
		rr := taxTableReq(h, http.MethodPost, "/api/v1/books/"+book+"/tax-tables", map[string]any{
			"name": "VAT 20%",
			"entries": []map[string]any{
				{"accountGuid": "vat-acc", "type": "percentage", "amount": map[string]int{"num": 20, "denom": 1}},
			},
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

		rr = taxTableReq(h, http.MethodGet, "/api/v1/tax-tables/"+guid, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("get: got %d, want 200 (%s)", rr.Code, rr.Body.String())
		}
		var got map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &got)
		entries, _ := got["entries"].([]any)
		if got["name"] != "VAT 20%" || len(entries) != 1 {
			t.Errorf("get: unexpected body %v", got)
		}
	})

	t.Run("a table with no entries is rejected with 400", func(t *testing.T) {
		h := newTaxTableTestServer(newFakeTaxTableRepoH())
		rr := taxTableReq(h, http.MethodPost, "/api/v1/books/"+book+"/tax-tables", map[string]any{
			"name":    "Empty",
			"entries": []map[string]any{},
		})
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400 (%s)", rr.Code, rr.Body.String())
		}
	})

	t.Run("getting an unknown table returns 404", func(t *testing.T) {
		h := newTaxTableTestServer(newFakeTaxTableRepoH())
		rr := taxTableReq(h, http.MethodGet, "/api/v1/tax-tables/nope", nil)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("got %d, want 404 (%s)", rr.Code, rr.Body.String())
		}
	})

	t.Run("deleting a table returns 204 and removes it", func(t *testing.T) {
		fr := newFakeTaxTableRepoH()
		fr.tables["tt-1"] = domain.TaxTable{GUID: "tt-1", BookGUID: book, Name: "GST"}
		h := newTaxTableTestServer(fr)
		rr := taxTableReq(h, http.MethodDelete, "/api/v1/tax-tables/tt-1", nil)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("delete: got %d, want 204 (%s)", rr.Code, rr.Body.String())
		}
		rr = taxTableReq(h, http.MethodGet, "/api/v1/tax-tables/tt-1", nil)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("get after delete: got %d, want 404", rr.Code)
		}
	})
}
