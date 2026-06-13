package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

func getRegister(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodGet, path, nil))
	h.ServeHTTP(rec, req)
	return rec
}

func TestAccountRegisterReturnsEntries(t *testing.T) {
	repo := &fakeRepo{
		exists:      true,
		registerTot: 1,
		registerRows: []app.RegisterEntry{{
			SplitGUID:     "split-1",
			TxGUID:        "tx-1",
			Description:   "Weekly groceries",
			Value:         domain.MustFromNumDenom(-5000, 100),
			Quantity:      domain.MustFromNumDenom(-5000, 100),
			Balance:       domain.MustFromNumDenom(-5000, 100),
			ValueScale:    100,
			QuantityScale: 100,
		}},
	}
	rec := getRegister(newTestServer(repo), "/api/v1/accounts/checking/register")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	var page registerPageDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if page.Total != 1 || len(page.Entries) != 1 {
		t.Fatalf("got total=%d entries=%d, want 1/1", page.Total, len(page.Entries))
	}
	if page.Limit != defaultRegisterLimit {
		t.Errorf("limit = %d, want default %d", page.Limit, defaultRegisterLimit)
	}
	bal := page.Entries[0].Balance
	if bal.Num != -5000 || bal.Denom != 100 {
		t.Errorf("balance = %d/%d, want -5000/100 (natural scale)", bal.Num, bal.Denom)
	}
}

func TestAccountRegisterNotFound(t *testing.T) {
	repo := &fakeRepo{exists: false}
	rec := getRegister(newTestServer(repo), "/api/v1/accounts/missing/register")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAccountRegisterClampsLimit(t *testing.T) {
	repo := &fakeRepo{exists: true}
	rec := getRegister(newTestServer(repo), "/api/v1/accounts/checking/register?limit=99999")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var page registerPageDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if page.Limit != maxRegisterLimit {
		t.Errorf("limit = %d, want clamped to %d", page.Limit, maxRegisterLimit)
	}
}
