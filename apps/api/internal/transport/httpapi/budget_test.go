package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// fakeBudgetRepoH satisfies app.BudgetReportRepository for HTTP-layer tests.
type fakeBudgetRepoH struct {
	*fakeRepo
	budgets  map[string]domain.Budget
	actuals  []app.AccountWithBalance
	bookRoot string
}

func newFakeBudgetRepoH() *fakeBudgetRepoH {
	return &fakeBudgetRepoH{
		fakeRepo: &fakeRepo{},
		budgets:  make(map[string]domain.Budget),
		bookRoot: "root-1",
	}
}

func (f *fakeBudgetRepoH) CreateBudget(_ context.Context, b domain.Budget) (domain.Budget, error) {
	f.budgets[b.GUID] = b
	return b, nil
}

func (f *fakeBudgetRepoH) ListBudgets(_ context.Context, bookGUID string) ([]domain.Budget, error) {
	var out []domain.Budget
	for _, b := range f.budgets {
		if b.BookGUID == bookGUID {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *fakeBudgetRepoH) GetBudget(_ context.Context, guid string) (domain.Budget, error) {
	b, ok := f.budgets[guid]
	if !ok {
		return domain.Budget{}, domain.ErrBudgetNotFound
	}
	return b, nil
}

func (f *fakeBudgetRepoH) UpdateBudget(_ context.Context, b domain.Budget) (domain.Budget, error) {
	if _, ok := f.budgets[b.GUID]; !ok {
		return domain.Budget{}, domain.ErrBudgetNotFound
	}
	f.budgets[b.GUID] = b
	return b, nil
}

func (f *fakeBudgetRepoH) DeleteBudget(_ context.Context, guid string) error {
	delete(f.budgets, guid)
	return nil
}

func (f *fakeBudgetRepoH) BookGUIDForBudget(_ context.Context, guid string) (string, error) {
	b, ok := f.budgets[guid]
	if !ok {
		return "", domain.ErrBudgetNotFound
	}
	return b.BookGUID, nil
}

func (f *fakeBudgetRepoH) BookRootAccount(_ context.Context, _ string) (string, error) {
	return f.bookRoot, nil
}

func (f *fakeBudgetRepoH) AccountBalances(_ context.Context, _ string, _, _ *time.Time) ([]app.AccountWithBalance, error) {
	return f.actuals, nil
}

func newBudgetTestServer(fr *fakeBudgetRepoH) http.Handler {
	posting := app.NewPostingService(fr.fakeRepo)
	budgetSvc := app.NewBudgetService(fr)
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
		budgetSvc,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	).Routes()
}

func budgetReq(h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
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

func TestBudgetList200(t *testing.T) {
	fr := newFakeBudgetRepoH()
	fr.budgets["bgt-1"] = domain.Budget{
		GUID: "bgt-1", BookGUID: "book-1", Name: "2024",
		PeriodType: domain.BudgetMonthly, NumPeriods: 12,
		StartDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	h := newBudgetTestServer(fr)
	rr := budgetReq(h, "GET", "/api/v1/books/book-1/budgets", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body)
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if budgets, ok := resp["budgets"].([]any); !ok || len(budgets) != 1 {
		t.Errorf("expected 1 budget in response, got %v", resp)
	}
}

func TestBudgetCreate201(t *testing.T) {
	fr := newFakeBudgetRepoH()
	h := newBudgetTestServer(fr)
	body := map[string]any{
		"name": "2025 Budget", "periodType": "monthly",
		"numPeriods": 12, "startDate": "2025-01-01", "amounts": []any{},
	}
	rr := budgetReq(h, "POST", "/api/v1/books/book-1/budgets", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rr.Code, rr.Body)
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["name"] != "2025 Budget" {
		t.Errorf("name = %v, want '2025 Budget'", resp["name"])
	}
}

func TestBudgetCreateInvalid400(t *testing.T) {
	fr := newFakeBudgetRepoH()
	h := newBudgetTestServer(fr)
	body := map[string]any{"name": "", "periodType": "monthly", "numPeriods": 12, "startDate": "2025-01-01"}
	rr := budgetReq(h, "POST", "/api/v1/books/book-1/budgets", body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rr.Code, rr.Body)
	}
}

func TestBudgetDelete204(t *testing.T) {
	fr := newFakeBudgetRepoH()
	fr.budgets["bgt-1"] = domain.Budget{
		GUID: "bgt-1", BookGUID: "book-1", Name: "test",
		PeriodType: domain.BudgetMonthly, NumPeriods: 1,
		StartDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	h := newBudgetTestServer(fr)
	rr := budgetReq(h, "DELETE", "/api/v1/budgets/bgt-1", nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body = %s", rr.Code, rr.Body)
	}
}

func TestBudgetReport200(t *testing.T) {
	fr := newFakeBudgetRepoH()
	fr.budgets["bgt-1"] = domain.Budget{
		GUID: "bgt-1", BookGUID: "book-1", Name: "test",
		PeriodType: domain.BudgetMonthly, NumPeriods: 12,
		StartDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amounts: []domain.BudgetAmount{
			{AccountGUID: "groc", PeriodNum: 0, Value: domain.MustFromNumDenom(50000, 100)},
		},
	}
	fr.actuals = []app.AccountWithBalance{{
		Account: domain.Account{GUID: "groc", Name: "Groceries", Type: domain.AccountExpense},
		Balance: domain.MustFromNumDenom(45000, 100),
	}}
	h := newBudgetTestServer(fr)
	rr := budgetReq(h, "GET", "/api/v1/budgets/bgt-1/report?asOf=2024-01-15T00:00:00Z", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body)
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["periodLabel"] != "Jan 2024" {
		t.Errorf("periodLabel = %v, want 'Jan 2024'", resp["periodLabel"])
	}
}
