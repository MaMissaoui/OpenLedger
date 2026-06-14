package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

func getPortfolio(h http.Handler, bookGUID string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodGet, "/api/v1/books/"+bookGUID+"/reports/portfolio", nil))
	h.ServeHTTP(rec, req)
	return rec
}

func TestPortfolioReport(t *testing.T) {
	repo := &fakeRepo{
		bookRoot: "root",
		holdings: []app.HoldingBalance{{
			Account:    domain.Account{GUID: "aapl-acct", Type: domain.AccountStock, CommodityGUID: "aapl", Name: "AAPL"},
			Shares:     domain.MustFromNumDenom(100, 1),
			ShareScale: 1,
			CostBasis:  domain.MustFromNumDenom(150000, 100),
		}},
		latestPrices: map[string]domain.Price{
			"aapl": {CommodityGUID: "aapl", CurrencyGUID: "usd", Value: domain.MustFromNumDenom(1800, 100)},
		},
	}
	rec := getPortfolio(newTestServer(repo), "book1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Holdings []struct {
			HasPrice       bool       `json:"hasPrice"`
			Shares         numericDTO `json:"shares"`
			MarketValue    numericDTO `json:"marketValue"`
			UnrealizedGain numericDTO `json:"unrealizedGain"`
			Account        struct {
				GUID string `json:"guid"`
			} `json:"account"`
		} `json:"holdings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
	}
	if len(body.Holdings) != 1 {
		t.Fatalf("holdings = %d, want 1", len(body.Holdings))
	}
	h := body.Holdings[0]
	if !h.HasPrice {
		t.Error("holding should be priced")
	}
	// 100 × $18.00 = $1,800 market value; gain $300.
	if h.MarketValue.Num != 1800 || h.MarketValue.Denom != 1 {
		t.Errorf("marketValue = %+v, want 1800/1", h.MarketValue)
	}
	if h.UnrealizedGain.Num != 300 || h.UnrealizedGain.Denom != 1 {
		t.Errorf("unrealizedGain = %+v, want 300/1", h.UnrealizedGain)
	}
}

func TestPortfolioForbiddenReturns403(t *testing.T) {
	repo := &fakeRepo{noMembership: true, bookRoot: "root"}
	rec := getPortfolio(newTestServer(repo), "book1")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}

func TestPortfolioBookNotFoundReturns404(t *testing.T) {
	// Membership is fine (default owner), but the book's root lookup fails.
	repo := &fakeRepo{bookNotFound: true}
	rec := getPortfolio(newTestServer(repo), "missing")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
}
