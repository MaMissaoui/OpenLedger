package app

import (
	"context"
	"fmt"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// PriceRepository persists and reads commodity price quotes.
type PriceRepository interface {
	InsertPrice(ctx context.Context, p domain.Price) error
	// ListPricesByCommodity returns the quotes for a commodity, most recent first.
	ListPricesByCommodity(ctx context.Context, commodityGUID string) ([]domain.Price, error)
}

// PriceService records and reads exchange-rate quotes. Prices are shared
// reference data (not book-scoped); currency conversion built on them comes later.
type PriceService struct {
	repo    PriceRepository
	newGUID func() string
	now     func() time.Time
}

// NewPriceService builds a PriceService backed by repo.
func NewPriceService(repo PriceRepository) *PriceService {
	return &PriceService{repo: repo, newGUID: NewGUID, now: time.Now}
}

// CreatePrice records a quote: one unit of CommodityGUID is worth Value units of
// CurrencyGUID. CommodityGUID, CurrencyGUID and a non-zero Value are required;
// Date defaults to now and Source to "user:price" when unset.
func (s *PriceService) CreatePrice(ctx context.Context, p domain.Price) (domain.Price, error) {
	if p.CommodityGUID == "" {
		return domain.Price{}, fmt.Errorf("%w: commodityGuid is required", ErrInvalidInput)
	}
	if p.CurrencyGUID == "" {
		return domain.Price{}, fmt.Errorf("%w: currencyGuid is required", ErrInvalidInput)
	}
	if p.Value.IsZero() {
		return domain.Price{}, fmt.Errorf("%w: value must be non-zero", ErrInvalidInput)
	}
	if p.Date.IsZero() {
		p.Date = s.now()
	}
	if p.Source == "" {
		p.Source = "user:price"
	}
	p.GUID = s.newGUID()
	if err := s.repo.InsertPrice(ctx, p); err != nil {
		return domain.Price{}, err
	}
	return p, nil
}

// ListPrices returns a commodity's quotes, most recent first. commodityGUID is
// required.
func (s *PriceService) ListPrices(ctx context.Context, commodityGUID string) ([]domain.Price, error) {
	if commodityGUID == "" {
		return nil, fmt.Errorf("%w: commodity is required", ErrInvalidInput)
	}
	return s.repo.ListPricesByCommodity(ctx, commodityGUID)
}
