package app

import (
	"context"
	"errors"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// fakePortfolioRepo drives PortfolioService in isolation.
type fakePortfolioRepo struct {
	root        string
	rootErr     error
	holdings    []HoldingBalance
	prices      map[string]domain.Price
	holdingsErr error
}

func (f *fakePortfolioRepo) BookRootAccount(_ context.Context, _ string) (string, error) {
	return f.root, f.rootErr
}

func (f *fakePortfolioRepo) SecurityHoldings(_ context.Context, _ string) ([]HoldingBalance, error) {
	return f.holdings, f.holdingsErr
}

func (f *fakePortfolioRepo) LatestPrice(_ context.Context, commodityGUID string) (domain.Price, bool, error) {
	p, ok := f.prices[commodityGUID]
	return p, ok, nil
}

func stock(guid, commodity string) domain.Account {
	return domain.Account{GUID: guid, Type: domain.AccountStock, CommodityGUID: commodity}
}

func TestPortfolioValuesHoldingFromLatestPrice(t *testing.T) {
	repo := &fakePortfolioRepo{
		root: "root",
		holdings: []HoldingBalance{{
			Account:    stock("aapl-acct", "aapl"),
			Shares:     domain.MustFromNumDenom(100, 1), // 100 shares
			ShareScale: 1,
			CostBasis:  domain.MustFromNumDenom(150000, 100), // paid $1,500.00
		}},
		prices: map[string]domain.Price{
			// Latest quote: $18.00 per share.
			"aapl": {CommodityGUID: "aapl", CurrencyGUID: "usd", Value: domain.MustFromNumDenom(1800, 100)},
		},
	}
	p, err := NewPortfolioService(repo).Portfolio(context.Background(), "book")
	if err != nil {
		t.Fatalf("portfolio: %v", err)
	}
	if len(p.Holdings) != 1 {
		t.Fatalf("holdings = %d, want 1", len(p.Holdings))
	}
	h := p.Holdings[0]
	if !h.HasPrice || h.PriceCurrency != "usd" {
		t.Fatalf("holding price = %+v", h)
	}
	// 100 shares × $18.00 = $1,800.00 market value.
	if want := domain.MustFromNumDenom(1800, 1); !h.MarketValue.Equal(want) {
		t.Errorf("market value = %s, want %s", h.MarketValue, want)
	}
	// $1,800.00 − $1,500.00 = $300.00 unrealized gain.
	if want := domain.MustFromNumDenom(300, 1); !h.UnrealizedGain.Equal(want) {
		t.Errorf("unrealized gain = %s, want %s", h.UnrealizedGain, want)
	}
}

func TestPortfolioHoldingWithoutQuote(t *testing.T) {
	repo := &fakePortfolioRepo{
		root: "root",
		holdings: []HoldingBalance{{
			Account:    stock("vti-acct", "vti"),
			Shares:     domain.MustFromNumDenom(10, 1),
			ShareScale: 1,
			CostBasis:  domain.MustFromNumDenom(50000, 100),
		}},
		prices: map[string]domain.Price{}, // no quote for vti
	}
	p, err := NewPortfolioService(repo).Portfolio(context.Background(), "book")
	if err != nil {
		t.Fatalf("portfolio: %v", err)
	}
	if len(p.Holdings) != 1 {
		t.Fatalf("holdings = %d, want 1", len(p.Holdings))
	}
	if h := p.Holdings[0]; h.HasPrice || !h.MarketValue.IsZero() || !h.UnrealizedGain.IsZero() {
		t.Errorf("unpriced holding should have no valuation: %+v", h)
	}
}

func TestPortfolioOmitsUntradedAccounts(t *testing.T) {
	repo := &fakePortfolioRepo{
		root: "root",
		holdings: []HoldingBalance{
			{Account: stock("empty", "aapl"), Shares: domain.Zero(), CostBasis: domain.Zero(), ShareScale: 1},
			{Account: stock("held", "aapl"), Shares: domain.MustFromNumDenom(5, 1), CostBasis: domain.MustFromNumDenom(100, 1), ShareScale: 1},
		},
	}
	p, err := NewPortfolioService(repo).Portfolio(context.Background(), "book")
	if err != nil {
		t.Fatalf("portfolio: %v", err)
	}
	if len(p.Holdings) != 1 || p.Holdings[0].Account.GUID != "held" {
		t.Errorf("expected only the traded account, got %+v", p.Holdings)
	}
}

func TestPortfolioBookNotFound(t *testing.T) {
	repo := &fakePortfolioRepo{rootErr: ErrBookNotFound}
	if _, err := NewPortfolioService(repo).Portfolio(context.Background(), "missing"); !errors.Is(err, ErrBookNotFound) {
		t.Fatalf("err = %v, want ErrBookNotFound", err)
	}
}
