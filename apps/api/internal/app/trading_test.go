package app

import (
	"context"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// fakeTradingRepo resolves account commodities from a fixed map and hands out a
// deterministic trading account GUID per commodity.
type fakeTradingRepo struct {
	byAccount map[string]AccountCommodityInfo
}

func (f *fakeTradingRepo) AccountCommodity(_ context.Context, guid string) (AccountCommodityInfo, error) {
	info, ok := f.byAccount[guid]
	if !ok {
		return AccountCommodityInfo{}, ErrAccountNotFound
	}
	return info, nil
}

func (f *fakeTradingRepo) FindOrCreateTradingAccount(_ context.Context, _ string, c domain.Commodity) (string, error) {
	return "trading-" + c.GUID, nil
}

func usd() AccountCommodityInfo {
	return AccountCommodityInfo{Commodity: domain.Commodity{GUID: "USD", Namespace: "CURRENCY", Mnemonic: "USD", Fraction: 100}}
}

func eur() AccountCommodityInfo {
	return AccountCommodityInfo{Commodity: domain.Commodity{GUID: "EUR", Namespace: "CURRENCY", Mnemonic: "EUR", Fraction: 100}}
}

func TestTradingServiceForeignExchange(t *testing.T) {
	repo := &fakeTradingRepo{byAccount: map[string]AccountCommodityInfo{
		"eur": eur(),
		"usd": usd(),
	}}
	svc := NewTradingService(repo)

	// Buy €100 for $110; tx currency USD.
	tx := domain.Transaction{CurrencyGUID: "USD", Splits: []domain.Split{
		{GUID: "a", AccountGUID: "eur", Value: domain.MustFromNumDenom(11000, 100), Quantity: domain.MustFromNumDenom(10000, 100)},
		{GUID: "b", AccountGUID: "usd", Value: domain.MustFromNumDenom(-11000, 100), Quantity: domain.MustFromNumDenom(-11000, 100)},
	}}

	splits, err := svc.Balance(context.Background(), tx)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if len(splits) != 4 {
		t.Fatalf("splits = %d, want 4 (2 user + 2 trading)", len(splits))
	}

	// Every commodity must net to zero in both value and quantity.
	commodityOf := map[string]string{"eur": "EUR", "usd": "USD", "trading-EUR": "EUR", "trading-USD": "USD"}
	netValue := map[string]domain.GncNumeric{}
	netQty := map[string]domain.GncNumeric{}
	for _, s := range splits {
		c := commodityOf[s.AccountGUID]
		if _, ok := netValue[c]; !ok {
			netValue[c] = domain.Zero()
			netQty[c] = domain.Zero()
		}
		netValue[c] = netValue[c].Add(s.Value)
		netQty[c] = netQty[c].Add(s.Quantity)
	}
	for c := range netValue {
		if !netValue[c].IsZero() {
			t.Errorf("commodity %s value net = %s, want 0", c, netValue[c])
		}
		if !netQty[c].IsZero() {
			t.Errorf("commodity %s quantity net = %s, want 0", c, netQty[c])
		}
	}
}

func TestTradingServiceSingleCurrencyUnchanged(t *testing.T) {
	repo := &fakeTradingRepo{byAccount: map[string]AccountCommodityInfo{
		"chk": usd(),
		"gro": usd(),
	}}
	svc := NewTradingService(repo)

	tx := domain.Transaction{CurrencyGUID: "USD", Splits: []domain.Split{
		{GUID: "a", AccountGUID: "chk", Value: domain.MustFromNumDenom(-5000, 100), Quantity: domain.MustFromNumDenom(-5000, 100)},
		{GUID: "b", AccountGUID: "gro", Value: domain.MustFromNumDenom(5000, 100), Quantity: domain.MustFromNumDenom(5000, 100)},
	}}

	splits, err := svc.Balance(context.Background(), tx)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if len(splits) != 2 {
		t.Errorf("single-currency tx should be unchanged, got %d splits", len(splits))
	}
}

// captureTxRepo records the transaction InsertTransaction/UpdateTransaction was
// asked to persist.
type captureTxRepo struct {
	inserted *domain.Transaction
}

func (r *captureTxRepo) InsertTransaction(_ context.Context, tx domain.Transaction, _ AuditActor) error {
	cp := tx
	r.inserted = &cp
	return nil
}
func (r *captureTxRepo) UpdateTransaction(_ context.Context, _ domain.Transaction, _ AuditActor) error {
	return nil
}
func (r *captureTxRepo) DeleteTransaction(_ context.Context, _ string, _ AuditActor) error {
	return nil
}
func (r *captureTxRepo) TransactionAccountGUIDs(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func TestPostingAppliesTradingSplits(t *testing.T) {
	txRepo := &captureTxRepo{}
	trading := NewTradingService(&fakeTradingRepo{byAccount: map[string]AccountCommodityInfo{
		"eur": eur(),
		"usd": usd(),
	}})
	svc := NewPostingService(txRepo).WithTrading(trading)

	tx := domain.Transaction{CurrencyGUID: "USD", Splits: []domain.Split{
		{AccountGUID: "eur", Value: domain.MustFromNumDenom(11000, 100), Quantity: domain.MustFromNumDenom(10000, 100)},
		{AccountGUID: "usd", Value: domain.MustFromNumDenom(-11000, 100), Quantity: domain.MustFromNumDenom(-11000, 100)},
	}}

	posted, err := svc.Post(context.Background(), tx, AuditActor{})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if txRepo.inserted == nil {
		t.Fatal("transaction was not persisted")
	}
	if len(posted.Splits) != 4 {
		t.Errorf("posted splits = %d, want 4 (trading splits added)", len(posted.Splits))
	}
	if err := posted.ValidateBalanced(); err != nil {
		t.Errorf("posted transaction does not balance in value: %v", err)
	}
}

func TestPostingRejectsImbalanceBeforeTrading(t *testing.T) {
	txRepo := &captureTxRepo{}
	trading := NewTradingService(&fakeTradingRepo{byAccount: map[string]AccountCommodityInfo{
		"eur": eur(),
		"usd": usd(),
	}})
	svc := NewPostingService(txRepo).WithTrading(trading)

	// Value does not balance (110 vs -100): trading accounts must not paper over
	// a real imbalance.
	tx := domain.Transaction{CurrencyGUID: "USD", Splits: []domain.Split{
		{AccountGUID: "eur", Value: domain.MustFromNumDenom(11000, 100), Quantity: domain.MustFromNumDenom(10000, 100)},
		{AccountGUID: "usd", Value: domain.MustFromNumDenom(-10000, 100), Quantity: domain.MustFromNumDenom(-10000, 100)},
	}}

	if _, err := svc.Post(context.Background(), tx, AuditActor{}); err == nil {
		t.Fatal("expected an imbalance error")
	}
	if txRepo.inserted != nil {
		t.Error("an unbalanced transaction must not be persisted")
	}
}
