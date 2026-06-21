package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// structureFake satisfies app.StructureRepository for the commodity, book and
// account routes.
type structureFake struct {
	commodities  []domain.Commodity
	books        []domain.Book
	accounts     []domain.Account
	bookRoot     string
	bookNotFound bool
	listAccounts []app.AccountWithBalance
}

func (f *structureFake) InsertCommodity(_ context.Context, c domain.Commodity) error {
	f.commodities = append(f.commodities, c)
	return nil
}

func (f *structureFake) ListCommodities(context.Context) ([]domain.Commodity, error) {
	return f.commodities, nil
}

func (f *structureFake) InsertBook(_ context.Context, b domain.Book, _, _ domain.Account, _ string) error {
	f.books = append(f.books, b)
	return nil
}

func (f *structureFake) InsertAccount(_ context.Context, a domain.Account) error {
	f.accounts = append(f.accounts, a)
	return nil
}

func (f *structureFake) ListBooksForUser(context.Context, string) ([]domain.Book, error) {
	return f.books, nil
}

func (f *structureFake) UpdateBook(_ context.Context, b domain.Book) error {
	for i, existing := range f.books {
		if existing.GUID == b.GUID {
			f.books[i] = b
			return nil
		}
	}
	return app.ErrBookNotFound
}

func (f *structureFake) BookRootAccount(context.Context, string) (string, error) {
	if f.bookNotFound {
		return "", app.ErrBookNotFound
	}
	return f.bookRoot, nil
}

func (f *structureFake) ListAccountsUnderRoot(context.Context, string) ([]app.AccountWithBalance, error) {
	return f.listAccounts, nil
}

func structureServer(f *structureFake) http.Handler {
	return authedServer(Services{Structure: app.NewStructureService(f)})
}

func postTo(h http.Handler, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateCommodity(t *testing.T) {
	repo := &structureFake{}
	rec := postTo(structureServer(repo), "/api/v1/commodities",
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
	rec := postTo(structureServer(&structureFake{}), "/api/v1/commodities", `{"fraction":100}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestListCommodities(t *testing.T) {
	repo := &structureFake{commodities: []domain.Commodity{
		{GUID: "usd", Namespace: "CURRENCY", Mnemonic: "USD", Fraction: 100},
		{GUID: "aapl", Namespace: "NASDAQ", Mnemonic: "AAPL", Fraction: 10000},
	}}
	rec := getRegister(structureServer(repo), "/api/v1/commodities")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Commodities []struct {
			GUID     string `json:"guid"`
			Mnemonic string `json:"mnemonic"`
			Fraction int64  `json:"fraction"`
		} `json:"commodities"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Commodities) != 2 {
		t.Fatalf("got %d commodities, want 2", len(resp.Commodities))
	}
	if got := resp.Commodities[0]; got.Mnemonic != "USD" || got.Fraction != 100 {
		t.Errorf("first commodity = %+v, want USD/100", got)
	}
}

func TestCreateBookReturnsRoot(t *testing.T) {
	repo := &structureFake{}
	rec := postTo(structureServer(repo), "/api/v1/books", `{"name":"Test Co","currencyGuid":""}`)
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
	repo := &structureFake{books: []domain.Book{
		{GUID: "book-1", RootAccountGUID: "root-1"},
		{GUID: "book-2", RootAccountGUID: "root-2"},
	}}
	rec := getRegister(structureServer(repo), "/api/v1/books")
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
	repo := &structureFake{bookRoot: "root-guid"}
	rec := postTo(structureServer(repo), "/api/v1/accounts",
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
	rec := postTo(structureServer(&structureFake{bookRoot: "root-guid"}), "/api/v1/accounts",
		`{"bookGuid":"book-1","name":"Weird","type":"NONSENSE","commodityGuid":"usd"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestCreateAccountUnknownBookReturns404(t *testing.T) {
	rec := postTo(structureServer(&structureFake{bookNotFound: true}), "/api/v1/accounts",
		`{"bookGuid":"missing","name":"Checking","type":"BANK","commodityGuid":"usd"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
}

func TestListAccounts(t *testing.T) {
	checking, _ := domain.FromNumDenom(12345, 100) // $123.45
	repo := &structureFake{
		bookRoot: "root-guid",
		listAccounts: []app.AccountWithBalance{
			{Account: domain.Account{GUID: "a1", Name: "Checking", Type: domain.AccountBank}, Balance: checking, BalanceScale: 100},
			{Account: domain.Account{GUID: "a2", Name: "Groceries", Type: domain.AccountExpense}, Balance: domain.Zero(), BalanceScale: 100},
		},
	}
	rec := getRegister(structureServer(repo), "/api/v1/books/book-1/accounts")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		BookGUID string `json:"bookGuid"`
		Accounts []struct {
			GUID    string `json:"guid"`
			Balance struct {
				Num   int64 `json:"num"`
				Denom int64 `json:"denom"`
			} `json:"balance"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Accounts) != 2 {
		t.Fatalf("got %d accounts, want 2", len(resp.Accounts))
	}
	if got := resp.Accounts[0].Balance; got.Num != 12345 || got.Denom != 100 {
		t.Errorf("checking balance = {%d, %d}, want {12345, 100}", got.Num, got.Denom)
	}
}

func TestListAccountsUnknownBookReturns404(t *testing.T) {
	rec := getRegister(structureServer(&structureFake{bookNotFound: true}), "/api/v1/books/missing/accounts")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
