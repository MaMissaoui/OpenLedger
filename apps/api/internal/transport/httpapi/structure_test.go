package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

func postTo(h http.Handler, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateCommodity(t *testing.T) {
	repo := &fakeRepo{}
	rec := postTo(newTestServer(repo), "/api/v1/commodities",
		`{"mnemonic":"USD","fullname":"US Dollar","fraction":100}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	if len(repo.commodities) != 1 {
		t.Fatalf("got %d commodities persisted, want 1", len(repo.commodities))
	}
	if got := repo.commodities[0]; got.GUID == "" || got.Namespace != domain.NamespaceCurrency {
		t.Errorf("commodity = %+v, want generated GUID and CURRENCY namespace", got)
	}
}

func TestCreateCommodityMissingMnemonicReturns400(t *testing.T) {
	rec := postTo(newTestServer(&fakeRepo{}), "/api/v1/commodities", `{"fraction":100}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestCreateBookReturnsRoot(t *testing.T) {
	repo := &fakeRepo{}
	rec := postTo(newTestServer(repo), "/api/v1/books", ``)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		GUID            string `json:"guid"`
		RootAccountGUID string `json:"rootAccountGuid"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.GUID == "" || resp.RootAccountGUID == "" {
		t.Errorf("book = %+v, want both guids populated", resp)
	}
}

func TestListBooks(t *testing.T) {
	repo := &fakeRepo{books: []domain.Book{
		{GUID: "book-1", RootAccountGUID: "root-1"},
		{GUID: "book-2", RootAccountGUID: "root-2"},
	}}
	rec := getRegister(newTestServer(repo), "/api/v1/books")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Books []map[string]any `json:"books"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Books) != 2 {
		t.Fatalf("got %d books, want 2", len(resp.Books))
	}
}

func TestCreateAccountDefaultsParentToRoot(t *testing.T) {
	repo := &fakeRepo{bookRoot: "root-guid"}
	rec := postTo(newTestServer(repo), "/api/v1/accounts",
		`{"bookGuid":"book-1","name":"Checking","type":"BANK","commodityGuid":"usd"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	if len(repo.accounts) != 1 {
		t.Fatalf("got %d accounts persisted, want 1", len(repo.accounts))
	}
	if got := repo.accounts[0].ParentGUID; got != "root-guid" {
		t.Errorf("parent = %q, want defaulted to book root %q", got, "root-guid")
	}
}

func TestCreateAccountUnknownTypeReturns400(t *testing.T) {
	rec := postTo(newTestServer(&fakeRepo{bookRoot: "root-guid"}), "/api/v1/accounts",
		`{"bookGuid":"book-1","name":"Weird","type":"NONSENSE","commodityGuid":"usd"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestCreateAccountUnknownBookReturns404(t *testing.T) {
	rec := postTo(newTestServer(&fakeRepo{bookNotFound: true}), "/api/v1/accounts",
		`{"bookGuid":"missing","name":"Checking","type":"BANK","commodityGuid":"usd"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
}

func TestListAccounts(t *testing.T) {
	repo := &fakeRepo{
		bookRoot: "root-guid",
		listAccounts: []domain.Account{
			{GUID: "a1", Name: "Checking", Type: domain.AccountBank},
			{GUID: "a2", Name: "Groceries", Type: domain.AccountExpense},
		},
	}
	rec := getRegister(newTestServer(repo), "/api/v1/books/book-1/accounts")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		BookGUID string           `json:"bookGuid"`
		Accounts []map[string]any `json:"accounts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Accounts) != 2 {
		t.Fatalf("got %d accounts, want 2", len(resp.Accounts))
	}
}

func TestListAccountsUnknownBookReturns404(t *testing.T) {
	rec := getRegister(newTestServer(&fakeRepo{bookNotFound: true}), "/api/v1/books/missing/accounts")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
