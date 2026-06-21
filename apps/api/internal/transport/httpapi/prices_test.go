package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// priceFake satisfies app.PriceRepository. The /prices routes are reference
// data (auth only, no book authz).
type priceFake struct {
	prices []domain.Price
}

func (f *priceFake) InsertPrice(_ context.Context, p domain.Price) error {
	f.prices = append(f.prices, p)
	return nil
}

func (f *priceFake) ListPricesByCommodity(context.Context, string) ([]domain.Price, error) {
	return f.prices, nil
}

func priceServer(f *priceFake) http.Handler {
	return authedServer(Services{Price: app.NewPriceService(f)})
}

func TestCreatePrice(t *testing.T) {
	repo := &priceFake{}
	rec := postTo(priceServer(repo), "/api/v1/prices",
		`{"commodityGuid":"aapl","currencyGuid":"usd","value":{"num":15000,"denom":100}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	if len(repo.prices) != 1 {
		t.Fatalf("got %d prices persisted, want 1", len(repo.prices))
	}
	got := repo.prices[0]
	if got.GUID == "" || got.Source != "user:price" || got.Date.IsZero() {
		t.Errorf("price = %+v, want generated GUID, default source, and a date", got)
	}
	if n, _ := got.Value.AtDenom(100); n != 15000 {
		t.Errorf("value = %s, want 150.00", got.Value)
	}
}

func TestCreatePriceMissingCommodityReturns400(t *testing.T) {
	rec := postTo(priceServer(&priceFake{}), "/api/v1/prices",
		`{"currencyGuid":"usd","value":{"num":15000,"denom":100}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestCreatePriceZeroValueReturns400(t *testing.T) {
	rec := postTo(priceServer(&priceFake{}), "/api/v1/prices",
		`{"commodityGuid":"aapl","currencyGuid":"usd","value":{"num":0,"denom":100}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestListPricesRequiresCommodity(t *testing.T) {
	rec := getRegister(priceServer(&priceFake{}), "/api/v1/prices")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestListPrices(t *testing.T) {
	val, _ := domain.FromNumDenom(15000, 100)
	repo := &priceFake{prices: []domain.Price{
		{GUID: "p1", CommodityGUID: "aapl", CurrencyGUID: "usd", Value: val},
	}}
	rec := getRegister(priceServer(repo), "/api/v1/prices?commodity=aapl")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Prices []struct {
			GUID  string     `json:"guid"`
			Value numericDTO `json:"value"`
		} `json:"prices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Prices) != 1 {
		t.Fatalf("got %d prices, want 1", len(resp.Prices))
	}
	if got := resp.Prices[0].Value; got.Num != 150 || got.Denom != 1 {
		t.Errorf("value = {%d, %d}, want reduced {150, 1}", got.Num, got.Denom)
	}
}
