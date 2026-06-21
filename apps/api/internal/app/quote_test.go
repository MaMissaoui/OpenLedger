package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

type stubProvider struct {
	rate              domain.GncNumeric
	date              time.Time
	err               error
	gotBase, gotQuote string
}

func (s *stubProvider) Name() string { return "stub" }

func (s *stubProvider) FetchRate(_ context.Context, base, quote string) (Quote, error) {
	s.gotBase, s.gotQuote = base, quote
	if s.err != nil {
		return Quote{}, s.err
	}
	return Quote{Rate: s.rate, Date: s.date}, nil
}

type stubCommodities map[string]domain.Commodity

func (s stubCommodities) GetCommodity(_ context.Context, guid string) (domain.Commodity, error) {
	c, ok := s[guid]
	if !ok {
		return domain.Commodity{}, ErrCommodityNotFound
	}
	return c, nil
}

func (s stubCommodities) ListDistinctPricePairs(_ context.Context) ([]PricePair, error) {
	return nil, nil
}

type capturingPriceRepo struct{ inserted []domain.Price }

func (r *capturingPriceRepo) InsertPrice(_ context.Context, p domain.Price) error {
	r.inserted = append(r.inserted, p)
	return nil
}

func (r *capturingPriceRepo) ListPricesByCommodity(_ context.Context, _ string) ([]domain.Price, error) {
	return r.inserted, nil
}

func currencies() stubCommodities {
	return stubCommodities{
		"usd": {GUID: "usd", Namespace: domain.NamespaceCurrency, Mnemonic: "USD"},
		"eur": {GUID: "eur", Namespace: domain.NamespaceCurrency, Mnemonic: "EUR"},
	}
}

func TestQuoteFetchAndStore(t *testing.T) {
	rate, _ := domain.FromDecimalString("0.9321")
	provider := &stubProvider{rate: rate, date: time.Date(2024, 6, 19, 0, 0, 0, 0, time.UTC)}
	repo := &capturingPriceRepo{}
	svc := NewQuoteService(provider, currencies(), NewPriceService(repo))

	price, err := svc.FetchAndStore(context.Background(), "usd", "eur")
	if err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}
	if provider.gotBase != "USD" || provider.gotQuote != "EUR" {
		t.Errorf("provider called with base=%q quote=%q, want USD/EUR", provider.gotBase, provider.gotQuote)
	}
	if len(repo.inserted) != 1 {
		t.Fatalf("persisted %d prices, want 1", len(repo.inserted))
	}
	got := repo.inserted[0]
	if got.CommodityGUID != "usd" || got.CurrencyGUID != "eur" {
		t.Errorf("price = %+v, want usd→eur", got)
	}
	if got.Source != "quote:stub" {
		t.Errorf("source = %q, want quote:stub", got.Source)
	}
	if got.Value.DecimalString(4) != "0.9321" {
		t.Errorf("value = %s, want 0.9321", got.Value)
	}
	if !got.Date.Equal(provider.date) {
		t.Errorf("date = %s, want %s", got.Date, provider.date)
	}
	_ = price
}

func TestQuoteFetchAndStoreRejectsNonCurrency(t *testing.T) {
	rate, _ := domain.FromDecimalString("150")
	commodities := currencies()
	commodities["aapl"] = domain.Commodity{GUID: "aapl", Namespace: "STOCK", Mnemonic: "AAPL"}
	svc := NewQuoteService(&stubProvider{rate: rate}, commodities, NewPriceService(&capturingPriceRepo{}))

	_, err := svc.FetchAndStore(context.Background(), "aapl", "usd")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestQuoteFetchAndStoreSameGUID(t *testing.T) {
	svc := NewQuoteService(&stubProvider{}, currencies(), NewPriceService(&capturingPriceRepo{}))
	_, err := svc.FetchAndStore(context.Background(), "usd", "usd")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestQuoteFetchAndStoreUnknownCommodity(t *testing.T) {
	svc := NewQuoteService(&stubProvider{}, currencies(), NewPriceService(&capturingPriceRepo{}))
	_, err := svc.FetchAndStore(context.Background(), "usd", "gbp")
	if !errors.Is(err, ErrCommodityNotFound) {
		t.Fatalf("err = %v, want ErrCommodityNotFound", err)
	}
}

func TestQuoteFetchAndStoreProviderError(t *testing.T) {
	provider := &stubProvider{err: errors.New("network down")}
	svc := NewQuoteService(provider, currencies(), NewPriceService(&capturingPriceRepo{}))
	_, err := svc.FetchAndStore(context.Background(), "usd", "eur")
	if !errors.Is(err, ErrQuoteUnavailable) {
		t.Fatalf("err = %v, want ErrQuoteUnavailable", err)
	}
}
