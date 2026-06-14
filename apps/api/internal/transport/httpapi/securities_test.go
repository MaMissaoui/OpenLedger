package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

func postJSON(h http.Handler, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	return rec
}

func TestBuySecurity(t *testing.T) {
	repo := &fakeRepo{
		accountCommodities: map[string]app.AccountCommodityInfo{
			"aapl": {Commodity: domain.Commodity{GUID: "aapl", Fraction: 1}},
			"cash": {Commodity: domain.Commodity{GUID: "usd", Fraction: 100}},
		},
	}
	rec := postJSON(newTestServer(repo), "/api/v1/securities/buy", `{
		"securityAccountGuid":"aapl","cashAccountGuid":"cash",
		"shares":{"num":10,"denom":1},"cash":{"num":150000,"denom":100}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	if repo.inserted == nil {
		t.Fatal("buy did not post a transaction")
	}
	if len(repo.createdLots) != 1 {
		t.Errorf("expected one lot opened, got %d", len(repo.createdLots))
	}
}

func TestSellSecurityRealizesGain(t *testing.T) {
	repo := &fakeRepo{
		accountCommodities: map[string]app.AccountCommodityInfo{
			"aapl": {Commodity: domain.Commodity{GUID: "aapl", Fraction: 1}},
			"cash": {Commodity: domain.Commodity{GUID: "usd", Fraction: 100}},
		},
		openLots: []domain.OpenLot{{
			GUID: "L1", Remaining: domain.MustFromNumDenom(10, 1), Cost: domain.MustFromNumDenom(150000, 100),
		}},
	}
	// Sell all 10 for $2,000 → realized gain $500.
	rec := postJSON(newTestServer(repo), "/api/v1/securities/sell", `{
		"securityAccountGuid":"aapl","cashAccountGuid":"cash",
		"shares":{"num":10,"denom":1},"cash":{"num":200000,"denom":100}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	if repo.inserted == nil {
		t.Fatal("sell did not post a transaction")
	}
	if len(repo.closedLots) != 1 || repo.closedLots[0] != "L1" {
		t.Errorf("the fully-sold lot should be closed, got %v", repo.closedLots)
	}
}

func TestSellSecurityInsufficientSharesReturns422(t *testing.T) {
	repo := &fakeRepo{
		accountCommodities: map[string]app.AccountCommodityInfo{
			"aapl": {Commodity: domain.Commodity{GUID: "aapl", Fraction: 1}},
			"cash": {Commodity: domain.Commodity{GUID: "usd", Fraction: 100}},
		},
		openLots: []domain.OpenLot{{
			GUID: "L1", Remaining: domain.MustFromNumDenom(5, 1), Cost: domain.MustFromNumDenom(75000, 100),
		}},
	}
	rec := postJSON(newTestServer(repo), "/api/v1/securities/sell", `{
		"securityAccountGuid":"aapl","cashAccountGuid":"cash",
		"shares":{"num":6,"denom":1},"cash":{"num":90000,"denom":100}}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", rec.Code, rec.Body.String())
	}
	if repo.inserted != nil {
		t.Error("a failed sale must not post a transaction")
	}
}

func TestBuySecurityInvalidReturns400(t *testing.T) {
	repo := &fakeRepo{
		accountCommodities: map[string]app.AccountCommodityInfo{
			"aapl": {Commodity: domain.Commodity{GUID: "aapl", Fraction: 1}},
			"cash": {Commodity: domain.Commodity{GUID: "usd", Fraction: 100}},
		},
	}
	// Zero shares is invalid.
	rec := postJSON(newTestServer(repo), "/api/v1/securities/buy", `{
		"securityAccountGuid":"aapl","cashAccountGuid":"cash",
		"shares":{"num":0,"denom":1},"cash":{"num":150000,"denom":100}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestCapitalGainsReport(t *testing.T) {
	repo := &fakeRepo{
		bookRoot: "root",
		realizedGainRows: []app.RealizedGainRow{{
			Date:        time.Now(),
			Description: "Sell 10 AAPL",
			Account:     domain.Account{Name: "Capital Gains", Type: domain.AccountIncome},
			// Income credit-normal: a $500 gain is stored as −500.
			Value: domain.MustFromNumDenom(-50000, 100),
			Scale: 100,
		}},
	}
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodGet, "/api/v1/books/book1/reports/capital-gains", nil))
	newTestServer(repo).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	// The $500 gain should surface as a positive total (natural sign).
	if !strings.Contains(rec.Body.String(), `"num":50000`) {
		t.Errorf("expected a +$500 gain in the report, got %s", rec.Body.String())
	}
}
