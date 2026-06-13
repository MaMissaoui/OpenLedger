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

// fakeRepo records the last inserted transaction without a database.
type fakeRepo struct {
	inserted *domain.Transaction
}

func (f *fakeRepo) InsertTransaction(_ context.Context, tx domain.Transaction, _ app.AuditActor) error {
	cp := tx
	f.inserted = &cp
	return nil
}

func newTestServer(repo app.TransactionRepository) http.Handler {
	return NewServer(app.NewPostingService(repo)).Routes()
}

func post(h http.Handler, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", strings.NewReader(body))
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
