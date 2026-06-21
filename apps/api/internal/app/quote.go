package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// ErrQuoteUnavailable wraps an upstream/provider failure (network error, unknown
// symbol, malformed response) so the transport layer can map it to 502 rather
// than a generic 500.
var ErrQuoteUnavailable = errors.New("quote unavailable")

// Quote is a fetched exchange rate: one unit of the base commodity is worth Rate
// units of the quote currency, as of Date. Rate is exact (parsed from the
// provider's decimal text, never a float).
type Quote struct {
	Rate domain.GncNumeric
	Date time.Time
}

// QuoteProvider fetches a live exchange rate between two currency mnemonics
// (e.g. base "USD", quote "EUR"). It is the pluggable replacement for GnuCash's
// Finance::Quote; concrete providers live in internal/infra.
type QuoteProvider interface {
	// FetchRate returns how many units of quote one unit of base buys.
	FetchRate(ctx context.Context, base, quote string) (Quote, error)
	// Name identifies the provider; it is recorded in the stored price's Source.
	Name() string
}

// PricePair is a (commodity, currency) pair for which a price has been recorded.
type PricePair struct {
	CommodityGUID string
	CurrencyGUID  string
}

// CommodityReader looks up a single commodity by GUID. Commodities are shared
// reference data (not book-scoped).
type CommodityReader interface {
	GetCommodity(ctx context.Context, guid string) (domain.Commodity, error)
	// ListDistinctPricePairs returns all distinct (commodity, currency) pairs
	// that have at least one recorded price. Used by the auto-refresh worker.
	ListDistinctPricePairs(ctx context.Context) ([]PricePair, error)
}

// QuoteService fetches an online quote for a commodity and records it as a
// price via the normal PriceService write path. It currently supports only
// currency commodities (the Frankfurter provider is FX-only); a non-currency
// commodity is rejected with ErrInvalidInput.
type QuoteService struct {
	provider    QuoteProvider
	commodities CommodityReader
	prices      *PriceService
}

// NewQuoteService wires a provider, a commodity lookup, and the price writer.
func NewQuoteService(provider QuoteProvider, commodities CommodityReader, prices *PriceService) *QuoteService {
	return &QuoteService{provider: provider, commodities: commodities, prices: prices}
}

// RefreshResult summarises a RefreshAll run.
type RefreshResult struct {
	Fetched int // number of pairs successfully updated
	Skipped int // pairs skipped (not a currency, same commodity/currency, etc.)
	Failed  int // pairs where the provider returned an error
}

// RefreshAll fetches the current rate for every (commodity, currency) pair that
// has at least one recorded price and stores each as a new price. Errors for
// individual pairs are counted but do not abort the loop.
func (s *QuoteService) RefreshAll(ctx context.Context) (RefreshResult, error) {
	pairs, err := s.commodities.ListDistinctPricePairs(ctx)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("list price pairs: %w", err)
	}
	var res RefreshResult
	for _, p := range pairs {
		if p.CommodityGUID == p.CurrencyGUID {
			res.Skipped++
			continue
		}
		_, ferr := s.FetchAndStore(ctx, p.CommodityGUID, p.CurrencyGUID)
		if ferr != nil {
			res.Failed++
		} else {
			res.Fetched++
		}
	}
	return res, nil
}

// FetchAndStore fetches the current rate for commodityGUID expressed in
// currencyGUID and records it as a price. Both GUIDs must resolve to currency
// commodities and must differ. Provider failures surface as ErrQuoteUnavailable.
func (s *QuoteService) FetchAndStore(ctx context.Context, commodityGUID, currencyGUID string) (domain.Price, error) {
	if commodityGUID == "" || currencyGUID == "" {
		return domain.Price{}, fmt.Errorf("%w: commodityGuid and currencyGuid are required", ErrInvalidInput)
	}
	if commodityGUID == currencyGUID {
		return domain.Price{}, fmt.Errorf("%w: commodity and currency must differ", ErrInvalidInput)
	}

	commodity, err := s.commodities.GetCommodity(ctx, commodityGUID)
	if err != nil {
		return domain.Price{}, err
	}
	currency, err := s.commodities.GetCommodity(ctx, currencyGUID)
	if err != nil {
		return domain.Price{}, err
	}
	if commodity.Namespace != domain.NamespaceCurrency || currency.Namespace != domain.NamespaceCurrency {
		return domain.Price{}, fmt.Errorf("%w: online quotes currently support only currency commodities", ErrInvalidInput)
	}

	quote, err := s.provider.FetchRate(ctx, commodity.Mnemonic, currency.Mnemonic)
	if err != nil {
		return domain.Price{}, fmt.Errorf("%w: %v", ErrQuoteUnavailable, err)
	}

	// CreatePrice keeps the explicit Source/Date (it only fills blanks) and
	// re-validates the value is non-zero.
	return s.prices.CreatePrice(ctx, domain.Price{
		CommodityGUID: commodityGUID,
		CurrencyGUID:  currencyGUID,
		Date:          quote.Date,
		Source:        "quote:" + s.provider.Name(),
		Type:          "last",
		Value:         quote.Rate,
	})
}
