package app

import (
	"context"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// PortfolioRepository reads the security holdings and latest price quotes the
// portfolio report aggregates.
type PortfolioRepository interface {
	// BookRootAccount returns a book's root_account_guid, or ErrBookNotFound.
	BookRootAccount(ctx context.Context, bookGUID string) (string, error)
	// SecurityHoldings returns every STOCK/MUTUAL descendant of rootGUID with its
	// share quantity (in the account's own commodity) and cost basis (the exact
	// sum of its splits' values, in the transaction currency). Cost basis is
	// summed across differing value denominators exactly, so a book that bought a
	// security in more than one currency still totals correctly.
	SecurityHoldings(ctx context.Context, rootGUID string) ([]HoldingBalance, error)
	// LatestPrice returns a commodity's most recent quote; ok is false when the
	// commodity has no quotes yet.
	LatestPrice(ctx context.Context, commodityGUID string) (domain.Price, bool, error)
}

// HoldingBalance is a security account's raw position before valuation: its
// share quantity (rendered at ShareScale, the commodity fraction) and the cost
// basis paid for it.
type HoldingBalance struct {
	Account    domain.Account
	Shares     domain.GncNumeric
	ShareScale int64
	CostBasis  domain.GncNumeric
}

// Holding is one security position in the portfolio report: its shares and cost
// basis, plus a market valuation derived from the latest price quote when one
// exists. When HasPrice is false the position has no quote and MarketValue /
// UnrealizedGain are zero and should not be shown.
type Holding struct {
	Account        domain.Account
	Shares         domain.GncNumeric
	ShareScale     int64
	CostBasis      domain.GncNumeric
	HasPrice       bool
	Price          domain.GncNumeric
	PriceCurrency  string
	MarketValue    domain.GncNumeric
	UnrealizedGain domain.GncNumeric
}

// Portfolio is a book's security-holdings report. Market values are not
// converted across currencies (there is no FX conversion in reports yet), so a
// holding is only valued in its own quote's currency.
type Portfolio struct {
	Holdings []Holding
}

// PortfolioService produces the portfolio (holdings) report by valuing each
// security position at its latest price quote.
type PortfolioService struct {
	repo PortfolioRepository
}

// NewPortfolioService builds a PortfolioService backed by repo.
func NewPortfolioService(repo PortfolioRepository) *PortfolioService {
	return &PortfolioService{repo: repo}
}

// Portfolio computes the holdings report for a book. It returns ErrBookNotFound
// for an unknown book. Security accounts that have never been traded (zero
// shares and zero cost) are omitted.
func (s *PortfolioService) Portfolio(ctx context.Context, bookGUID string) (Portfolio, error) {
	root, err := s.repo.BookRootAccount(ctx, bookGUID)
	if err != nil {
		return Portfolio{}, err
	}
	balances, err := s.repo.SecurityHoldings(ctx, root)
	if err != nil {
		return Portfolio{}, err
	}

	out := Portfolio{Holdings: make([]Holding, 0, len(balances))}
	for _, b := range balances {
		if b.Shares.IsZero() && b.CostBasis.IsZero() {
			continue
		}
		h := Holding{
			Account:    b.Account,
			Shares:     b.Shares,
			ShareScale: b.ShareScale,
			CostBasis:  b.CostBasis,
		}
		price, ok, err := s.repo.LatestPrice(ctx, b.Account.CommodityGUID)
		if err != nil {
			return Portfolio{}, err
		}
		if ok {
			h.HasPrice = true
			h.Price = price.Value
			h.PriceCurrency = price.CurrencyGUID
			h.MarketValue = b.Shares.Mul(price.Value)
			h.UnrealizedGain = h.MarketValue.Sub(b.CostBasis)
		}
		out.Holdings = append(out.Holdings, h)
	}
	return out, nil
}
