package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// tradeFake satisfies the ports the securities routes touch: app.TradeRepository
// and app.TransactionRepository (Trade posts buys/sells via PostingService), and
// app.CapitalGainsRepository for the realized-gains report.
type tradeFake struct {
	accountCommodities map[string]app.AccountCommodityInfo
	openLots           []domain.OpenLot
	createdLots        []string
	closedLots         []string
	capGainsAccount    string
	inserted           *domain.Transaction
	bookRoot           string
	realizedGainRows   []app.RealizedGainRow
}

func (f *tradeFake) InsertTransaction(_ context.Context, tx domain.Transaction, _ app.AuditActor) error {
	cp := tx
	f.inserted = &cp
	return nil
}

func (f *tradeFake) UpdateTransaction(context.Context, domain.Transaction, app.AuditActor) error {
	return nil
}

func (f *tradeFake) DeleteTransaction(context.Context, string, app.AuditActor) error { return nil }

func (f *tradeFake) TransactionAccountGUIDs(context.Context, string) ([]string, error) {
	return nil, nil
}

func (f *tradeFake) AccountCommodity(_ context.Context, accountGUID string) (app.AccountCommodityInfo, error) {
	if info, ok := f.accountCommodities[accountGUID]; ok {
		return info, nil
	}
	return app.AccountCommodityInfo{}, nil
}

func (f *tradeFake) CreateLot(_ context.Context, lotGUID, _ string) error {
	f.createdLots = append(f.createdLots, lotGUID)
	return nil
}

func (f *tradeFake) OpenLotsForAccount(context.Context, string) ([]domain.OpenLot, error) {
	return f.openLots, nil
}

func (f *tradeFake) SetLotClosed(_ context.Context, lotGUID string) error {
	f.closedLots = append(f.closedLots, lotGUID)
	return nil
}

func (f *tradeFake) FindOrCreateCapitalGainsAccount(context.Context, string, domain.Commodity) (string, error) {
	if f.capGainsAccount == "" {
		f.capGainsAccount = "capgains"
	}
	return f.capGainsAccount, nil
}

func (f *tradeFake) BookRootAccount(context.Context, string) (string, error) {
	return f.bookRoot, nil
}

func (f *tradeFake) RealizedGainRows(context.Context, string, *time.Time, *time.Time) ([]app.RealizedGainRow, error) {
	return f.realizedGainRows, nil
}

func securitiesServer(f *tradeFake) http.Handler {
	posting := app.NewPostingService(f)
	return authedServer(Services{
		Trade:        app.NewTradeService(f, posting),
		CapitalGains: app.NewCapitalGainsService(f),
	})
}

func postJSON(h http.Handler, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	return rec
}

func TestBuySecurity(t *testing.T) {
	repo := &tradeFake{
		accountCommodities: map[string]app.AccountCommodityInfo{
			"aapl": {Commodity: domain.Commodity{GUID: "aapl", Fraction: 1}},
			"cash": {Commodity: domain.Commodity{GUID: "usd", Fraction: 100}},
		},
	}
	rec := postJSON(securitiesServer(repo), "/api/v1/securities/buy", `{
		"securityAccountGuid":"aapl","cashAccountGuid":"cash",
		"shares":{"num":10,"denom":1},"cash":{"num":150000,"denom":100}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	if repo.inserted == nil {
		t.Fatal("buy did not post a transaction")
	}
	if len(repo.createdLots) != 1 {
		t.Errorf("expected one lot opened, got %d", len(repo.createdLots))
	}
}

func TestSellSecurityRealizesGain(t *testing.T) {
	repo := &tradeFake{
		accountCommodities: map[string]app.AccountCommodityInfo{
			"aapl": {Commodity: domain.Commodity{GUID: "aapl", Fraction: 1}},
			"cash": {Commodity: domain.Commodity{GUID: "usd", Fraction: 100}},
		},
		openLots: []domain.OpenLot{{
			GUID: "L1", Remaining: domain.MustFromNumDenom(10, 1), Cost: domain.MustFromNumDenom(150000, 100),
		}},
	}
	// Sell all 10 for $2,000 → realized gain $500.
	rec := postJSON(securitiesServer(repo), "/api/v1/securities/sell", `{
		"securityAccountGuid":"aapl","cashAccountGuid":"cash",
		"shares":{"num":10,"denom":1},"cash":{"num":200000,"denom":100}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	if repo.inserted == nil {
		t.Fatal("sell did not post a transaction")
	}
	if len(repo.closedLots) != 1 || repo.closedLots[0] != "L1" {
		t.Errorf("the fully-sold lot should be closed, got %v", repo.closedLots)
	}
}

func TestSellSecurityInsufficientSharesReturns422(t *testing.T) {
	repo := &tradeFake{
		accountCommodities: map[string]app.AccountCommodityInfo{
			"aapl": {Commodity: domain.Commodity{GUID: "aapl", Fraction: 1}},
			"cash": {Commodity: domain.Commodity{GUID: "usd", Fraction: 100}},
		},
		openLots: []domain.OpenLot{{
			GUID: "L1", Remaining: domain.MustFromNumDenom(5, 1), Cost: domain.MustFromNumDenom(75000, 100),
		}},
	}
	rec := postJSON(securitiesServer(repo), "/api/v1/securities/sell", `{
		"securityAccountGuid":"aapl","cashAccountGuid":"cash",
		"shares":{"num":6,"denom":1},"cash":{"num":90000,"denom":100}}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", rec.Code, rec.Body.String())
	}
	if repo.inserted != nil {
		t.Error("a failed sale must not post a transaction")
	}
}

func TestBuySecurityInvalidReturns400(t *testing.T) {
	repo := &tradeFake{
		accountCommodities: map[string]app.AccountCommodityInfo{
			"aapl": {Commodity: domain.Commodity{GUID: "aapl", Fraction: 1}},
			"cash": {Commodity: domain.Commodity{GUID: "usd", Fraction: 100}},
		},
	}
	// Zero shares is invalid.
	rec := postJSON(securitiesServer(repo), "/api/v1/securities/buy", `{
		"securityAccountGuid":"aapl","cashAccountGuid":"cash",
		"shares":{"num":0,"denom":1},"cash":{"num":150000,"denom":100}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestCapitalGainsReport(t *testing.T) {
	repo := &tradeFake{
		bookRoot: "root",
		realizedGainRows: []app.RealizedGainRow{{
			Date:        time.Now(),
			Description: "Sell 10 AAPL",
			Account:     domain.Account{Name: "Capital Gains", Type: domain.AccountIncome},
			// Income credit-normal: a $500 gain is stored as −500.
			Value: domain.MustFromNumDenom(-50000, 100),
			Scale: 100,
		}},
	}
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodGet, "/api/v1/books/book1/reports/capital-gains", nil))
	securitiesServer(repo).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	// The $500 gain should surface as a positive total (natural sign).
	if !strings.Contains(rec.Body.String(), `"num":50000`) {
		t.Errorf("expected a +$500 gain in the report, got %s", rec.Body.String())
	}
}
