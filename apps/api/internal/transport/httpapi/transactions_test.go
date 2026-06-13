package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// fakeRepo implements all repository ports in memory so the HTTP layer can be
// tested without a database.
type fakeRepo struct {
	inserted     *domain.Transaction
	exists       bool
	registerRows []app.RegisterEntry
	registerTot  int64

	// Structure side.
	commodities  []domain.Commodity
	books        []domain.Book
	accounts     []domain.Account
	bookRoot     string // root returned by BookRootAccount
	bookNotFound bool   // make BookRootAccount return ErrBookNotFound
	listAccounts []app.AccountWithBalance

	// Provision side.
	provisionedUserID string // returned by FindOrCreateLDAPUser (default "user-1")

	// Authz side. The zero value grants owner access so most tests don't set it
	// up; set role to test a specific permission level, or noMembership for 403.
	noMembership    bool     // UserBookRole reports no membership row
	role            app.Role // membership role (defaults to owner when empty)
	accountUnknown  bool     // BookGUIDForAccount returns ErrAccountNotFound
	accountBookGUID string   // book returned by BookGUIDForAccount (default "book-1")
}

func (f *fakeRepo) FindOrCreateLDAPUser(_ context.Context, _, _ string) (string, error) {
	if f.provisionedUserID != "" {
		return f.provisionedUserID, nil
	}
	return "user-1", nil
}

func (f *fakeRepo) UserBookRole(_ context.Context, _, _ string) (app.Role, bool, error) {
	if f.noMembership {
		return "", false, nil
	}
	if f.role != "" {
		return f.role, true, nil
	}
	return app.RoleOwner, true, nil
}

func (f *fakeRepo) BookGUIDForAccount(_ context.Context, _ string) (string, error) {
	if f.accountUnknown {
		return "", app.ErrAccountNotFound
	}
	if f.accountBookGUID != "" {
		return f.accountBookGUID, nil
	}
	return "book-1", nil
}

func (f *fakeRepo) InsertTransaction(_ context.Context, tx domain.Transaction, _ app.AuditActor) error {
	cp := tx
	f.inserted = &cp
	return nil
}

func (f *fakeRepo) AccountExists(_ context.Context, _ string) (bool, error) {
	return f.exists, nil
}

func (f *fakeRepo) ListAccountRegister(_ context.Context, _ string, _, _ int) ([]app.RegisterEntry, int64, error) {
	return f.registerRows, f.registerTot, nil
}

func (f *fakeRepo) InsertCommodity(_ context.Context, c domain.Commodity) error {
	f.commodities = append(f.commodities, c)
	return nil
}

func (f *fakeRepo) ListCommodities(_ context.Context) ([]domain.Commodity, error) {
	return f.commodities, nil
}

func (f *fakeRepo) InsertBook(_ context.Context, b domain.Book, _, _ domain.Account, _ string) error {
	f.books = append(f.books, b)
	return nil
}

func (f *fakeRepo) ListBooksForUser(_ context.Context, _ string) ([]domain.Book, error) {
	return f.books, nil
}

func (f *fakeRepo) InsertAccount(_ context.Context, a domain.Account) error {
	f.accounts = append(f.accounts, a)
	return nil
}

func (f *fakeRepo) BookRootAccount(_ context.Context, _ string) (string, error) {
	if f.bookNotFound {
		return "", app.ErrBookNotFound
	}
	return f.bookRoot, nil
}

func (f *fakeRepo) ListAccountsUnderRoot(_ context.Context, _ string) ([]app.AccountWithBalance, error) {
	return f.listAccounts, nil
}

func newTestServer(repo *fakeRepo) http.Handler {
	return NewServer(
		app.NewPostingService(repo),
		app.NewLedgerService(repo),
		app.NewStructureService(repo),
		app.NewProvisionService(repo),
		app.NewAuthzService(repo),
	).Routes()
}

// withAuth sets the Authelia-forwarded identity headers so requests reach the
// protected /api/v1 handlers. In production Traefik adds these after Authelia
// verifies the session; in tests we set them directly.
func withAuth(req *http.Request) *http.Request {
	req.Header.Set("Remote-User", "test-user")
	req.Header.Set("Remote-Email", "test@example.com")
	return req
}

func post(h http.Handler, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodPost, "/api/v1/transactions", strings.NewReader(body)))
	h.ServeHTTP(rec, req)
	return rec
}

func TestPostBalancedTransaction(t *testing.T) {
	repo := &fakeRepo{}
	rec := post(newTestServer(repo), `{
		"currencyGuid":"USD","description":"groceries","splits":[
			{"accountGuid":"checking","value":{"num":5000,"denom":100},"quantity":{"num":5000,"denom":100}},
			{"accountGuid":"groceries","value":{"num":-5000,"denom":100},"quantity":{"num":-5000,"denom":100}}
		]}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	if repo.inserted == nil {
		t.Fatal("transaction was not persisted")
	}
	if repo.inserted.GUID == "" {
		t.Error("expected a generated transaction GUID")
	}
	for _, s := range repo.inserted.Splits {
		if s.GUID == "" {
			t.Error("expected a generated split GUID")
		}
	}
}

func TestPostUnbalancedTransactionReturns422(t *testing.T) {
	repo := &fakeRepo{}
	rec := post(newTestServer(repo), `{
		"currencyGuid":"USD","splits":[
			{"accountGuid":"checking","value":{"num":5000,"denom":100},"quantity":{"num":5000,"denom":100}},
			{"accountGuid":"groceries","value":{"num":-4900,"denom":100},"quantity":{"num":-4900,"denom":100}}
		]}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", rec.Code, rec.Body.String())
	}
	if repo.inserted != nil {
		t.Error("unbalanced transaction must not be persisted")
	}
}

func TestPostInvalidJSONReturns400(t *testing.T) {
	rec := post(newTestServer(&fakeRepo{}), `{ not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPostSingleSplitReturns400(t *testing.T) {
	rec := post(newTestServer(&fakeRepo{}), `{
		"currencyGuid":"USD","splits":[
			{"accountGuid":"checking","value":{"num":5000,"denom":100},"quantity":{"num":5000,"denom":100}}
		]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (request shape); body = %s", rec.Code, rec.Body.String())
	}
}
