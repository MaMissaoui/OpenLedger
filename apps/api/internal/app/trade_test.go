package app

import (
	"context"
	"errors"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// fakeTradeRepo implements both TransactionRepository (so a real PostingService
// can run) and TradeRepository.
type fakeTradeRepo struct {
	posted   *domain.Transaction
	openLots []domain.OpenLot
	created  []string
	closed   []string
}

func (f *fakeTradeRepo) InsertTransaction(_ context.Context, tx domain.Transaction, _ AuditActor) error {
	cp := tx
	f.posted = &cp
	return nil
}
func (f *fakeTradeRepo) UpdateTransaction(context.Context, domain.Transaction, AuditActor) error {
	return nil
}
func (f *fakeTradeRepo) DeleteTransaction(context.Context, string, AuditActor) error { return nil }
func (f *fakeTradeRepo) TransactionAccountGUIDs(context.Context, string) ([]string, error) {
	return nil, nil
}

func (f *fakeTradeRepo) AccountCommodity(_ context.Context, _ string) (AccountCommodityInfo, error) {
	// The cash account carries USD; everything else is irrelevant to these tests.
	return AccountCommodityInfo{Commodity: domain.Commodity{GUID: "usd", Mnemonic: "USD", Fraction: 100}}, nil
}
func (f *fakeTradeRepo) CreateLot(_ context.Context, lotGUID, _ string) error {
	f.created = append(f.created, lotGUID)
	return nil
}
func (f *fakeTradeRepo) OpenLotsForAccount(context.Context, string) ([]domain.OpenLot, error) {
	return f.openLots, nil
}
func (f *fakeTradeRepo) SetLotClosed(_ context.Context, lotGUID string) error {
	f.closed = append(f.closed, lotGUID)
	return nil
}
func (f *fakeTradeRepo) FindOrCreateCapitalGainsAccount(context.Context, string, domain.Commodity) (string, error) {
	return "capgains", nil
}

func newTradeService(repo *fakeTradeRepo) *TradeService {
	return NewTradeService(repo, NewPostingService(repo))
}

func dollars(cents int64) domain.GncNumeric { return domain.MustFromNumDenom(cents, 100) }
func shares(n int64) domain.GncNumeric      { return domain.MustFromNumDenom(n, 1) }

func TestBuyOpensLotAndPosts(t *testing.T) {
	repo := &fakeTradeRepo{}
	res, err := newTradeService(repo).Buy(context.Background(), Trade{
		SecurityAccountGUID: "aapl", CashAccountGUID: "cash",
		Shares: shares(10), Cash: dollars(150000),
	}, AuditActor{})
	if err != nil {
		t.Fatalf("buy: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected one lot opened, got %d", len(repo.created))
	}
	if repo.posted == nil || len(repo.posted.Splits) != 2 {
		t.Fatalf("expected a 2-split post, got %+v", repo.posted)
	}
	// The security split carries the new lot and shares as quantity, cash as value.
	sec := repo.posted.Splits[0]
	if sec.LotGUID != repo.created[0] || !sec.Quantity.Equal(shares(10)) || !sec.Value.Equal(dollars(150000)) {
		t.Errorf("security split = %+v", sec)
	}
	if !res.RealizedGain.IsZero() {
		t.Errorf("a buy realizes no gain, got %s", res.RealizedGain)
	}
}

func TestSellPartialFIFOGain(t *testing.T) {
	repo := &fakeTradeRepo{openLots: []domain.OpenLot{{GUID: "L1", Remaining: shares(10), Cost: dollars(150000)}}}
	// Sell 4 of 10 for $800. Cost basis removed = 4/10 × $1,500 = $600 → gain $200.
	res, err := newTradeService(repo).Sell(context.Background(), Trade{
		SecurityAccountGUID: "aapl", CashAccountGUID: "cash",
		Shares: shares(4), Cash: dollars(80000),
	}, AuditActor{})
	if err != nil {
		t.Fatalf("sell: %v", err)
	}
	if want := dollars(20000); !res.RealizedGain.Equal(want) {
		t.Errorf("realized gain = %s, want %s", res.RealizedGain, want)
	}
	if len(repo.closed) != 0 {
		t.Errorf("a partial sale must not close the lot, closed = %v", repo.closed)
	}
	// security (−$600, −4 sh, lot L1), cash (+$800), gains (−$200). Balances to 0.
	if repo.posted == nil || len(repo.posted.Splits) != 3 {
		t.Fatalf("expected 3 splits, got %+v", repo.posted)
	}
	if err := repo.posted.ValidateBalanced(); err != nil {
		t.Errorf("sale transaction does not balance: %v", err)
	}
}

func TestSellWholeLotClosesIt(t *testing.T) {
	repo := &fakeTradeRepo{openLots: []domain.OpenLot{{GUID: "L1", Remaining: shares(10), Cost: dollars(150000)}}}
	res, err := newTradeService(repo).Sell(context.Background(), Trade{
		SecurityAccountGUID: "aapl", CashAccountGUID: "cash",
		Shares: shares(10), Cash: dollars(200000),
	}, AuditActor{})
	if err != nil {
		t.Fatalf("sell: %v", err)
	}
	if want := dollars(50000); !res.RealizedGain.Equal(want) {
		t.Errorf("realized gain = %s, want %s ($2,000 − $1,500)", res.RealizedGain, want)
	}
	if len(repo.closed) != 1 || repo.closed[0] != "L1" {
		t.Errorf("the fully-sold lot should be closed, closed = %v", repo.closed)
	}
}

func TestSellMoreThanHeld(t *testing.T) {
	repo := &fakeTradeRepo{openLots: []domain.OpenLot{{GUID: "L1", Remaining: shares(5), Cost: dollars(75000)}}}
	_, err := newTradeService(repo).Sell(context.Background(), Trade{
		SecurityAccountGUID: "aapl", CashAccountGUID: "cash",
		Shares: shares(6), Cash: dollars(90000),
	}, AuditActor{})
	if !errors.Is(err, domain.ErrInsufficientShares) {
		t.Fatalf("err = %v, want ErrInsufficientShares", err)
	}
	if repo.posted != nil {
		t.Error("a failed sale must not post a transaction")
	}
}
