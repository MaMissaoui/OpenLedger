package app

import (
	"context"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// TradingBalancer rewrites a transaction's splits so the book stays balanced in
// every commodity, adding GnuCash-style trading-account splits. The posting
// service consults it after the transaction's value balance is validated.
type TradingBalancer interface {
	// Balance returns the full split set for tx: the non-trading splits plus
	// freshly-computed trading splits. Any trading splits already present are
	// dropped and recomputed, so repeated calls are idempotent. Returned trading
	// splits carry a GUID and reconcile state.
	Balance(ctx context.Context, tx domain.Transaction) ([]domain.Split, error)
}

// AccountCommodityInfo is the commodity an account is denominated in and whether
// the account is itself a trading account.
type AccountCommodityInfo struct {
	Commodity domain.Commodity
	IsTrading bool
}

// TradingRepository resolves the data the trading engine needs and creates the
// trading accounts it posts to.
type TradingRepository interface {
	// AccountCommodity returns the commodity an account is denominated in and
	// whether it is a trading account, or ErrAccountNotFound if unknown.
	AccountCommodity(ctx context.Context, accountGUID string) (AccountCommodityInfo, error)
	// FindOrCreateTradingAccount returns the GUID of the
	// Trading:NAMESPACE:MNEMONIC account for commodity c in the book that
	// anchorAccountGUID belongs to, creating the Trading hierarchy as needed.
	FindOrCreateTradingAccount(ctx context.Context, anchorAccountGUID string, c domain.Commodity) (string, error)
}

// TradingService implements TradingBalancer over a TradingRepository.
type TradingService struct {
	repo    TradingRepository
	newGUID func() string
}

// NewTradingService builds a TradingService backed by repo.
func NewTradingService(repo TradingRepository) *TradingService {
	return &TradingService{repo: repo, newGUID: NewGUID}
}

// Balance resolves each posting account's commodity, drops any pre-existing
// trading splits, computes the trading splits needed to balance every commodity,
// and materialises them against find-or-created Trading:NS:MNEMONIC accounts.
func (s *TradingService) Balance(ctx context.Context, tx domain.Transaction) ([]domain.Split, error) {
	commodityOf := make(map[string]string, len(tx.Splits))
	commodityByGUID := make(map[string]domain.Commodity)
	var (
		base   = make([]domain.Split, 0, len(tx.Splits)) // non-trading splits
		anchor string                                    // a non-trading account, to locate the book
	)
	for _, sp := range tx.Splits {
		info, err := s.repo.AccountCommodity(ctx, sp.AccountGUID)
		if err != nil {
			return nil, err
		}
		commodityOf[sp.AccountGUID] = info.Commodity.GUID
		commodityByGUID[info.Commodity.GUID] = info.Commodity
		if info.IsTrading {
			continue // drop existing trading splits; they are recomputed below
		}
		base = append(base, sp)
		if anchor == "" {
			anchor = sp.AccountGUID
		}
	}

	trading := domain.ComputeTradingSplits(base, commodityOf, nil)
	if len(trading) == 0 {
		return base, nil
	}

	full := base
	for _, ts := range trading {
		acct, err := s.repo.FindOrCreateTradingAccount(ctx, anchor, commodityByGUID[ts.CommodityGUID])
		if err != nil {
			return nil, err
		}
		full = append(full, domain.Split{
			GUID:        s.newGUID(),
			AccountGUID: acct,
			Reconcile:   domain.ReconcileNew,
			Value:       ts.Value,
			Quantity:    ts.Quantity,
		})
	}
	return full, nil
}
