package httpapi

import (
	"net/http"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/app"
)

// A user with no membership on the book must not reach its accounts, register,
// or post transactions to it — each book-scoped route returns 403.

func TestListAccountsForbiddenWithoutMembership(t *testing.T) {
	repo := &fakeRepo{noMembership: true, bookRoot: "root-guid"}
	rec := getRegister(newTestServer(repo), "/api/v1/books/book-1/accounts")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}

func TestCreateAccountForbiddenWithoutMembership(t *testing.T) {
	repo := &fakeRepo{noMembership: true, bookRoot: "root-guid"}
	rec := postTo(newTestServer(repo), "/api/v1/accounts",
		`{"bookGuid":"book-1","name":"Checking","type":"BANK","commodityGuid":"usd"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
	if len(repo.accounts) != 0 {
		t.Errorf("account must not be created without book access; got %d", len(repo.accounts))
	}
}

func TestAccountRegisterForbiddenWithoutMembership(t *testing.T) {
	repo := &fakeRepo{noMembership: true, exists: true}
	rec := getRegister(newTestServer(repo), "/api/v1/accounts/checking/register")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}

func TestAccountRegisterUnknownAccountReturns404(t *testing.T) {
	repo := &fakeRepo{accountUnknown: true}
	rec := getRegister(newTestServer(repo), "/api/v1/accounts/missing/register")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
}

func TestPostTransactionForbiddenWithoutMembership(t *testing.T) {
	repo := &fakeRepo{noMembership: true}
	rec := post(newTestServer(repo), `{
		"currencyGuid":"USD","splits":[
			{"accountGuid":"checking","value":{"num":5000,"denom":100},"quantity":{"num":5000,"denom":100}},
			{"accountGuid":"groceries","value":{"num":-5000,"denom":100},"quantity":{"num":-5000,"denom":100}}
		]}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
	if repo.inserted != nil {
		t.Error("transaction must not be posted to a book the user cannot access")
	}
}

func TestPostTransactionUnknownAccountReturns404(t *testing.T) {
	repo := &fakeRepo{accountUnknown: true}
	rec := post(newTestServer(repo), `{
		"currencyGuid":"USD","splits":[
			{"accountGuid":"ghost","value":{"num":5000,"denom":100},"quantity":{"num":5000,"denom":100}},
			{"accountGuid":"other","value":{"num":-5000,"denom":100},"quantity":{"num":-5000,"denom":100}}
		]}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
	if repo.inserted != nil {
		t.Error("transaction referencing an unknown account must not be posted")
	}
}

// Role-based checks: a viewer may read but not write; an editor may do both.

func TestViewerCanReadRegister(t *testing.T) {
	repo := &fakeRepo{role: app.RoleViewer, exists: true}
	rec := getRegister(newTestServer(repo), "/api/v1/accounts/checking/register")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
}

func TestViewerCanListAccounts(t *testing.T) {
	repo := &fakeRepo{role: app.RoleViewer, bookRoot: "root-guid"}
	rec := getRegister(newTestServer(repo), "/api/v1/books/book-1/accounts")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
}

func TestViewerCannotCreateAccount(t *testing.T) {
	repo := &fakeRepo{role: app.RoleViewer, bookRoot: "root-guid"}
	rec := postTo(newTestServer(repo), "/api/v1/accounts",
		`{"bookGuid":"book-1","name":"Checking","type":"BANK","commodityGuid":"usd"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
	if len(repo.accounts) != 0 {
		t.Errorf("viewer must not create accounts; got %d", len(repo.accounts))
	}
}

func TestViewerCannotPostTransaction(t *testing.T) {
	repo := &fakeRepo{role: app.RoleViewer}
	rec := post(newTestServer(repo), `{
		"currencyGuid":"USD","splits":[
			{"accountGuid":"checking","value":{"num":5000,"denom":100},"quantity":{"num":5000,"denom":100}},
			{"accountGuid":"groceries","value":{"num":-5000,"denom":100},"quantity":{"num":-5000,"denom":100}}
		]}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
	if repo.inserted != nil {
		t.Error("viewer must not post transactions")
	}
}

func TestEditorCanPostTransaction(t *testing.T) {
	repo := &fakeRepo{role: app.RoleEditor}
	rec := post(newTestServer(repo), `{
		"currencyGuid":"USD","splits":[
			{"accountGuid":"checking","value":{"num":5000,"denom":100},"quantity":{"num":5000,"denom":100}},
			{"accountGuid":"groceries","value":{"num":-5000,"denom":100},"quantity":{"num":-5000,"denom":100}}
		]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	if repo.inserted == nil {
		t.Error("editor's transaction should be persisted")
	}
}

func TestEditorCanCreateAccount(t *testing.T) {
	repo := &fakeRepo{role: app.RoleEditor, bookRoot: "root-guid"}
	rec := postTo(newTestServer(repo), "/api/v1/accounts",
		`{"bookGuid":"book-1","name":"Checking","type":"BANK","commodityGuid":"usd"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	if len(repo.accounts) != 1 {
		t.Errorf("editor should create the account; got %d", len(repo.accounts))
	}
}
