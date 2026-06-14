package app

import (
	"context"
	"fmt"
	"time"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// TradeRepository is the persistence the security-trading use-cases need beyond
// posting: lot lifecycle and the find-or-create of the capital-gains account.
type TradeRepository interface {
	// AccountCommodity returns the commodity an account is denominated in (and
	// whether it is a trading account), or ErrAccountNotFound.
	AccountCommodity(ctx context.Context, accountGUID string) (AccountCommodityInfo, error)
	// CreateLot inserts a new open lot for a security account.
	CreateLot(ctx context.Context, lotGUID, accountGUID string) error
	// OpenLotsForAccount returns the account's open lots in FIFO order, each with
	// its remaining shares and attached cost basis.
	OpenLotsForAccount(ctx context.Context, accountGUID string) ([]domain.OpenLot, error)
	// SetLotClosed marks a lot fully sold.
	SetLotClosed(ctx context.Context, lotGUID string) error
	// FindOrCreateCapitalGainsAccount returns the book's Capital Gains INCOME
	// account for the given currency, creating it if needed.
	FindOrCreateCapitalGainsAccount(ctx context.Context, anchorAccountGUID string, currency domain.Commodity) (string, error)
}

// TradeService posts security purchases and sales, maintaining purchase lots so
// that a sale realizes a FIFO capital gain. It builds the transaction and hands
// it to the PostingService (the only transaction write path), which validates
// the balance and adds trading splits to keep the book balanced per commodity.
type TradeService struct {
	repo    TradeRepository
	posting *PostingService
	newGUID func() string
	now     func() time.Time
}

// NewTradeService builds a TradeService backed by repo, posting through posting.
func NewTradeService(repo TradeRepository, posting *PostingService) *TradeService {
	return &TradeService{repo: repo, posting: posting, newGUID: NewGUID, now: time.Now}
}

// Trade is a buy or sell request. Shares is in the security's commodity; Cash is
// the total cash paid (buy) or received (sell), in the cash account's currency.
type Trade struct {
	SecurityAccountGUID string
	CashAccountGUID     string
	Shares              domain.GncNumeric
	Cash                domain.GncNumeric
	Description         string
	PostDate            time.Time
}

// TradeResult reports the posted transaction and, for a sale, the realized gain.
type TradeResult struct {
	TransactionGUID string
	RealizedGain    domain.GncNumeric
}

// Buy records a purchase of Shares for Cash: it opens a lot, tags the security
// split to it, and posts a two-leg transaction (the trading engine balances the
// commodities). Shares and Cash must be positive.
func (s *TradeService) Buy(ctx context.Context, t Trade, actor AuditActor) (TradeResult, error) {
	if err := t.validate(); err != nil {
		return TradeResult{}, err
	}
	currency, err := s.tradeCurrency(ctx, t)
	if err != nil {
		return TradeResult{}, err
	}

	lotGUID := s.newGUID()
	if err := s.repo.CreateLot(ctx, lotGUID, t.SecurityAccountGUID); err != nil {
		return TradeResult{}, err
	}

	tx := domain.Transaction{
		CurrencyGUID: currency.GUID,
		PostDate:     s.postDate(t),
		Description:  t.descriptionOr("Buy security"),
		Splits: []domain.Split{
			{AccountGUID: t.SecurityAccountGUID, Value: t.Cash, Quantity: t.Shares, LotGUID: lotGUID},
			{AccountGUID: t.CashAccountGUID, Value: t.Cash.Neg(), Quantity: t.Cash.Neg()},
		},
	}
	posted, err := s.posting.Post(ctx, tx, actor)
	if err != nil {
		return TradeResult{}, err
	}
	return TradeResult{TransactionGUID: posted.GUID, RealizedGain: domain.Zero()}, nil
}

// Sell records a sale of Shares for Cash proceeds. It matches the shares against
// the account's open lots FIFO, removing each lot's cost basis, posts one
// security split per consumed lot (so each split stays tied to a single lot), a
// cash split for the proceeds, and a capital-gains split for proceeds minus cost
// basis, then closes any fully-consumed lots. It returns ErrInsufficientShares
// (mapped to 422) when the account holds fewer shares than the sale.
func (s *TradeService) Sell(ctx context.Context, t Trade, actor AuditActor) (TradeResult, error) {
	if err := t.validate(); err != nil {
		return TradeResult{}, err
	}
	currency, err := s.tradeCurrency(ctx, t)
	if err != nil {
		return TradeResult{}, err
	}

	openLots, err := s.repo.OpenLotsForAccount(ctx, t.SecurityAccountGUID)
	if err != nil {
		return TradeResult{}, err
	}
	match, err := domain.MatchFIFO(openLots, t.Shares)
	if err != nil {
		return TradeResult{}, err
	}
	realizedGain := t.Cash.Sub(match.TotalCost)

	gainsAccount, err := s.repo.FindOrCreateCapitalGainsAccount(ctx, t.SecurityAccountGUID, currency)
	if err != nil {
		return TradeResult{}, err
	}

	// One security split per consumed lot removes that lot's cost basis (value)
	// and its shares (quantity); the cash leg takes the proceeds; the gains leg
	// books proceeds − cost so the transaction balances in the trade currency.
	splits := make([]domain.Split, 0, len(match.Consumptions)+2)
	for _, c := range match.Consumptions {
		splits = append(splits, domain.Split{
			AccountGUID: t.SecurityAccountGUID,
			Value:       c.Cost.Neg(),
			Quantity:    c.Quantity.Neg(),
			LotGUID:     c.LotGUID,
		})
	}
	splits = append(splits,
		domain.Split{AccountGUID: t.CashAccountGUID, Value: t.Cash, Quantity: t.Cash},
		domain.Split{AccountGUID: gainsAccount, Value: realizedGain.Neg(), Quantity: realizedGain.Neg()},
	)

	tx := domain.Transaction{
		CurrencyGUID: currency.GUID,
		PostDate:     s.postDate(t),
		Description:  t.descriptionOr("Sell security"),
		Splits:       splits,
	}
	posted, err := s.posting.Post(ctx, tx, actor)
	if err != nil {
		return TradeResult{}, err
	}

	for _, c := range match.Consumptions {
		if c.ClosesLot {
			if err := s.repo.SetLotClosed(ctx, c.LotGUID); err != nil {
				return TradeResult{}, err
			}
		}
	}
	return TradeResult{TransactionGUID: posted.GUID, RealizedGain: realizedGain}, nil
}

// tradeCurrency resolves the trade currency from the cash account and confirms
// both accounts exist (AccountCommodity returns ErrAccountNotFound otherwise).
// The cash account's commodity is the currency the trade is denominated in.
func (s *TradeService) tradeCurrency(ctx context.Context, t Trade) (domain.Commodity, error) {
	if _, err := s.repo.AccountCommodity(ctx, t.SecurityAccountGUID); err != nil {
		return domain.Commodity{}, err
	}
	cash, err := s.repo.AccountCommodity(ctx, t.CashAccountGUID)
	if err != nil {
		return domain.Commodity{}, err
	}
	return cash.Commodity, nil
}

func (t Trade) validate() error {
	if t.SecurityAccountGUID == "" || t.CashAccountGUID == "" {
		return fmt.Errorf("%w: securityAccountGuid and cashAccountGuid are required", ErrInvalidInput)
	}
	if t.Shares.Sign() <= 0 {
		return fmt.Errorf("%w: shares must be positive", ErrInvalidInput)
	}
	if t.Cash.Sign() <= 0 {
		return fmt.Errorf("%w: cash amount must be positive", ErrInvalidInput)
	}
	return nil
}

func (t Trade) descriptionOr(def string) string {
	if t.Description == "" {
		return def
	}
	return t.Description
}

func (s *TradeService) postDate(t Trade) time.Time {
	if t.PostDate.IsZero() {
		return s.now().UTC()
	}
	return t.PostDate
}
