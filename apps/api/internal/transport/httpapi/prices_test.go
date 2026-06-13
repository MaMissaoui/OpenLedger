package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

func TestCreatePrice(t *testing.T) {
	repo := &fakeRepo{}
	rec := postTo(newTestServer(repo), "/api/v1/prices",
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
	rec := postTo(newTestServer(&fakeRepo{}), "/api/v1/prices",
		`{"currencyGuid":"usd","value":{"num":15000,"denom":100}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestCreatePriceZeroValueReturns400(t *testing.T) {
	rec := postTo(newTestServer(&fakeRepo{}), "/api/v1/prices",
		`{"commodityGuid":"aapl","currencyGuid":"usd","value":{"num":0,"denom":100}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestListPricesRequiresCommodity(t *testing.T) {
	rec := getRegister(newTestServer(&fakeRepo{}), "/api/v1/prices")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestListPrices(t *testing.T) {
	val, _ := domain.FromNumDenom(15000, 100)
	repo := &fakeRepo{prices: []domain.Price{
		{GUID: "p1", CommodityGUID: "aapl", CurrencyGUID: "usd", Value: val},
	}}
	rec := getRegister(newTestServer(repo), "/api/v1/prices?commodity=aapl")
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
