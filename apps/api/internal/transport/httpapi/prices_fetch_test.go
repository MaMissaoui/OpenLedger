package httpapi

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

type stubQuoteProvider struct {
	rate domain.GncNumeric
	date time.Time
	err  error
}

func (s stubQuoteProvider) Name() string { return "stub" }

func (s stubQuoteProvider) FetchRate(_ context.Context, _, _ string) (app.Quote, error) {
	if s.err != nil {
		return app.Quote{}, s.err
	}
	return app.Quote{Rate: s.rate, Date: s.date}, nil
}

func currencyRepo(provider app.QuoteProvider) *fakeRepo {
	return &fakeRepo{
		commodities: []domain.Commodity{
			{GUID: "usd", Namespace: domain.NamespaceCurrency, Mnemonic: "USD"},
			{GUID: "eur", Namespace: domain.NamespaceCurrency, Mnemonic: "EUR"},
		},
		quoteProvider: provider,
	}
}

func TestFetchPrice(t *testing.T) {
	rate, _ := domain.FromDecimalString("0.92")
	repo := currencyRepo(stubQuoteProvider{rate: rate, date: time.Now()})
	rec := postTo(newTestServer(repo), "/api/v1/prices/fetch",
		`{"commodityGuid":"usd","currencyGuid":"eur"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	if len(repo.prices) != 1 {
		t.Fatalf("persisted %d prices, want 1", len(repo.prices))
	}
	if got := repo.prices[0]; got.Source != "quote:stub" || got.Value.DecimalString(2) != "0.92" {
		t.Errorf("price = %+v, want source quote:stub and value 0.92", got)
	}
}

func TestFetchPriceProviderErrorReturns502(t *testing.T) {
	repo := currencyRepo(stubQuoteProvider{err: errors.New("upstream down")})
	rec := postTo(newTestServer(repo), "/api/v1/prices/fetch",
		`{"commodityGuid":"usd","currencyGuid":"eur"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body = %s", rec.Code, rec.Body.String())
	}
}

func TestFetchPriceUnknownCommodityReturns404(t *testing.T) {
	rate, _ := domain.FromDecimalString("0.92")
	repo := currencyRepo(stubQuoteProvider{rate: rate})
	rec := postTo(newTestServer(repo), "/api/v1/prices/fetch",
		`{"commodityGuid":"usd","currencyGuid":"gbp"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
}

func TestFetchPriceNonCurrencyReturns400(t *testing.T) {
	rate, _ := domain.FromDecimalString("150")
	repo := currencyRepo(stubQuoteProvider{rate: rate})
	repo.commodities = append(repo.commodities,
		domain.Commodity{GUID: "aapl", Namespace: "STOCK", Mnemonic: "AAPL"})
	rec := postTo(newTestServer(repo), "/api/v1/prices/fetch",
		`{"commodityGuid":"aapl","currencyGuid":"usd"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestFetchPriceNotConfiguredReturns503(t *testing.T) {
	// No quoteProvider set, so newTestServer leaves the quote service nil.
	repo := &fakeRepo{commodities: []domain.Commodity{
		{GUID: "usd", Namespace: domain.NamespaceCurrency, Mnemonic: "USD"},
		{GUID: "eur", Namespace: domain.NamespaceCurrency, Mnemonic: "EUR"},
	}}
	rec := postTo(newTestServer(repo), "/api/v1/prices/fetch",
		`{"commodityGuid":"usd","currencyGuid":"eur"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body = %s", rec.Code, rec.Body.String())
	}
}
