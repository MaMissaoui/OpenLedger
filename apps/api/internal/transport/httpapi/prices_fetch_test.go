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

// quoteFake satisfies app.CommodityReader (currency lookup) and
// app.PriceRepository (the stored quote is written through PriceService).
type quoteFake struct {
	commodities []domain.Commodity
	prices      []domain.Price
}

func (f *quoteFake) GetCommodity(_ context.Context, guid string) (domain.Commodity, error) {
	for _, c := range f.commodities {
		if c.GUID == guid {
			return c, nil
		}
	}
	return domain.Commodity{}, app.ErrCommodityNotFound
}

func (f *quoteFake) InsertPrice(_ context.Context, p domain.Price) error {
	f.prices = append(f.prices, p)
	return nil
}

func (f *quoteFake) ListPricesByCommodity(context.Context, string) ([]domain.Price, error) {
	return f.prices, nil
}

func (f *quoteFake) ListDistinctPricePairs(context.Context) ([]app.PricePair, error) {
	return nil, nil
}

func currencyFake() *quoteFake {
	return &quoteFake{commodities: []domain.Commodity{
		{GUID: "usd", Namespace: domain.NamespaceCurrency, Mnemonic: "USD"},
		{GUID: "eur", Namespace: domain.NamespaceCurrency, Mnemonic: "EUR"},
	}}
}

// fetchPriceServer wires Price (always) and Quote (only when a provider is
// given, mirroring production where the fetch endpoint is 503 without one).
func fetchPriceServer(f *quoteFake, provider app.QuoteProvider) http.Handler {
	priceSvc := app.NewPriceService(f)
	svcs := Services{Price: priceSvc}
	if provider != nil {
		svcs.Quote = app.NewQuoteService(provider, f, priceSvc)
	}
	return authedServer(svcs)
}

func TestFetchPrice(t *testing.T) {
	rate, _ := domain.FromDecimalString("0.92")
	repo := currencyFake()
	rec := postTo(fetchPriceServer(repo, stubQuoteProvider{rate: rate, date: time.Now()}), "/api/v1/prices/fetch",
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
	repo := currencyFake()
	rec := postTo(fetchPriceServer(repo, stubQuoteProvider{err: errors.New("upstream down")}), "/api/v1/prices/fetch",
		`{"commodityGuid":"usd","currencyGuid":"eur"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body = %s", rec.Code, rec.Body.String())
	}
}

func TestFetchPriceUnknownCommodityReturns404(t *testing.T) {
	rate, _ := domain.FromDecimalString("0.92")
	repo := currencyFake()
	rec := postTo(fetchPriceServer(repo, stubQuoteProvider{rate: rate}), "/api/v1/prices/fetch",
		`{"commodityGuid":"usd","currencyGuid":"gbp"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
}

func TestFetchPriceNonCurrencyReturns400(t *testing.T) {
	rate, _ := domain.FromDecimalString("150")
	repo := currencyFake()
	repo.commodities = append(repo.commodities,
		domain.Commodity{GUID: "aapl", Namespace: "STOCK", Mnemonic: "AAPL"})
	rec := postTo(fetchPriceServer(repo, stubQuoteProvider{rate: rate}), "/api/v1/prices/fetch",
		`{"commodityGuid":"aapl","currencyGuid":"usd"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestFetchPriceNotConfiguredReturns503(t *testing.T) {
	// No provider, so fetchPriceServer leaves the quote service nil.
	repo := &quoteFake{commodities: []domain.Commodity{
		{GUID: "usd", Namespace: domain.NamespaceCurrency, Mnemonic: "USD"},
		{GUID: "eur", Namespace: domain.NamespaceCurrency, Mnemonic: "EUR"},
	}}
	rec := postTo(fetchPriceServer(repo, nil), "/api/v1/prices/fetch",
		`{"commodityGuid":"usd","currencyGuid":"eur"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body = %s", rec.Code, rec.Body.String())
	}
}
