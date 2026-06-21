package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// txFake satisfies app.TransactionRepository (Posting) and app.LedgerRepository
// (register + single-transaction read) — the ports the transaction routes touch.
type txFake struct {
	inserted     *domain.Transaction
	updated      *domain.Transaction
	deletedGUID  string
	txAccounts   []string            // returned by TransactionAccountGUIDs
	txNotFound   bool                // make TransactionAccountGUIDs / GetTransaction return ErrTransactionNotFound
	gotTx        *domain.Transaction // returned by GetTransaction
	exists       bool
	registerRows []app.RegisterEntry
	registerTot  int64
}

func (f *txFake) InsertTransaction(_ context.Context, tx domain.Transaction, _ app.AuditActor) error {
	cp := tx
	f.inserted = &cp
	return nil
}

func (f *txFake) UpdateTransaction(_ context.Context, tx domain.Transaction, _ app.AuditActor) error {
	cp := tx
	f.updated = &cp
	return nil
}

func (f *txFake) DeleteTransaction(_ context.Context, guid string, _ app.AuditActor) error {
	f.deletedGUID = guid
	return nil
}

func (f *txFake) TransactionAccountGUIDs(context.Context, string) ([]string, error) {
	if f.txNotFound {
		return nil, app.ErrTransactionNotFound
	}
	if f.txAccounts != nil {
		return f.txAccounts, nil
	}
	return []string{"checking", "groceries"}, nil
}

func (f *txFake) AccountExists(context.Context, string) (bool, error) {
	return f.exists, nil
}

func (f *txFake) ListAccountRegister(context.Context, string, int, int) ([]app.RegisterEntry, int64, error) {
	return f.registerRows, f.registerTot, nil
}

func (f *txFake) GetTransaction(_ context.Context, guid string) (domain.Transaction, error) {
	if f.txNotFound || f.gotTx == nil {
		return domain.Transaction{}, app.ErrTransactionNotFound
	}
	tx := *f.gotTx
	tx.GUID = guid
	return tx, nil
}

func txServer(f *txFake) http.Handler { return txServerAuthz(f, nil) }

// txServerAuthz wires the transaction routes with a specific AuthzService, for
// tests that exercise membership/role gates (nil -> granted-access default).
func txServerAuthz(f *txFake, authz *app.AuthzService) http.Handler {
	posting := app.NewPostingService(f)
	return authedServer(Services{Posting: posting, Ledger: app.NewLedgerService(f), Authz: authz})
}

// fakeWriter is a stub GnuCashWriter that records what it was asked to write and
// creates a placeholder file at the path so the handler can stream it back. The
// file content carries the format token so handler dispatch can be asserted.
// Shared with exports_test.go.
type fakeWriter struct {
	wrote  *app.GnuCashData
	format string
}

func (fw *fakeWriter) WriteGnuCashSQLite(_ context.Context, path string, data app.GnuCashData) error {
	return fw.record("sqlite", path, data)
}

func (fw *fakeWriter) WriteGnuCashXML(_ context.Context, path string, data app.GnuCashData) error {
	return fw.record("xml", path, data)
}

func (fw *fakeWriter) record(format, path string, data app.GnuCashData) error {
	cp := data
	fw.wrote = &cp
	fw.format = format
	return os.WriteFile(path, []byte("gnucash-export:"+format), 0o600)
}

// withAuth sets the Authelia-forwarded identity headers so requests reach the
// protected /api/v1 handlers. In production Traefik adds these after Authelia
// verifies the session; in tests we set them directly. Shared across test files.
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
	repo := &txFake{}
	rec := post(txServer(repo), `{
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
	repo := &txFake{}
	rec := post(txServer(repo), `{
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
	rec := post(txServer(&txFake{}), `{ not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPostSingleSplitReturns400(t *testing.T) {
	rec := post(txServer(&txFake{}), `{
		"currencyGuid":"USD","splits":[
			{"accountGuid":"checking","value":{"num":5000,"denom":100},"quantity":{"num":5000,"denom":100}}
		]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (request shape); body = %s", rec.Code, rec.Body.String())
	}
}
