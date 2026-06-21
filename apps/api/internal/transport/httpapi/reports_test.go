package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// reportFake satisfies app.ReportRepository for the report endpoints.
type reportFake struct {
	bookRoot   string
	reportRows []app.AccountWithBalance
}

func (f *reportFake) BookRootAccount(context.Context, string) (string, error) {
	return f.bookRoot, nil
}

func (f *reportFake) AccountBalances(context.Context, string, *time.Time, *time.Time) ([]app.AccountWithBalance, error) {
	return f.reportRows, nil
}

func reportServer(f *reportFake, authz *app.AuthzService) http.Handler {
	return authedServer(Services{Report: app.NewReportService(f), Authz: authz})
}

func getReport(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodGet, path, nil))
	h.ServeHTTP(rec, req)
	return rec
}

func reportRow(typ domain.AccountType, rawNum int64) app.AccountWithBalance {
	return app.AccountWithBalance{
		Account:      domain.Account{GUID: string(typ), Name: string(typ), Type: typ},
		Balance:      domain.MustFromNumDenom(rawNum, 100),
		BalanceScale: 100,
	}
}

func TestBalanceSheet(t *testing.T) {
	repo := &reportFake{bookRoot: "root", reportRows: []app.AccountWithBalance{
		reportRow(domain.AccountBank, 150000),
		reportRow(domain.AccountEquity, -100000),
		reportRow(domain.AccountIncome, -70000),
		reportRow(domain.AccountExpense, 20000),
	}}
	rec := getReport(reportServer(repo, nil), "/api/v1/books/book-1/reports/balance-sheet")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Assets struct {
			Total numericDTO `json:"total"`
		} `json:"assets"`
		TotalLiabilitiesAndEquity numericDTO `json:"totalLiabilitiesAndEquity"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Assets.Total.Num != 150000 || resp.Assets.Total.Denom != 100 {
		t.Errorf("assets total = {%d,%d}, want {150000,100}", resp.Assets.Total.Num, resp.Assets.Total.Denom)
	}
	// The statement must balance.
	if resp.TotalLiabilitiesAndEquity.Num != 150000 {
		t.Errorf("L+E total = {%d,%d}, want {150000,100}",
			resp.TotalLiabilitiesAndEquity.Num, resp.TotalLiabilitiesAndEquity.Denom)
	}
}

func TestIncomeStatementEndpoint(t *testing.T) {
	repo := &reportFake{bookRoot: "root", reportRows: []app.AccountWithBalance{
		reportRow(domain.AccountIncome, -70000),
		reportRow(domain.AccountExpense, 20000),
		reportRow(domain.AccountBank, 150000),
	}}
	rec := getReport(reportServer(repo, nil),
		"/api/v1/books/book-1/reports/income-statement?from=2026-01-01T00:00:00Z&to=2026-06-30T00:00:00Z")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		NetIncome numericDTO `json:"netIncome"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.NetIncome.Num != 50000 || resp.NetIncome.Denom != 100 {
		t.Errorf("net income = {%d,%d}, want {50000,100}", resp.NetIncome.Num, resp.NetIncome.Denom)
	}
}

func TestBalanceSheetBadDateReturns400(t *testing.T) {
	rec := getReport(reportServer(&reportFake{bookRoot: "root"}, nil),
		"/api/v1/books/book-1/reports/balance-sheet?asOf=not-a-date")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestBalanceSheetForbiddenWithoutMembership(t *testing.T) {
	rec := getReport(reportServer(&reportFake{bookRoot: "root"}, app.NewAuthzService(&authStub{noMembership: true})),
		"/api/v1/books/book-1/reports/balance-sheet")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}
