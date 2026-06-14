package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sendJSON(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(method, path, strings.NewReader(body)))
	h.ServeHTTP(rec, req)
	return rec
}

const balancedBody = `{
	"currencyGuid":"USD","description":"corrected","splits":[
		{"accountGuid":"checking","value":{"num":7500,"denom":100},"quantity":{"num":7500,"denom":100}},
		{"accountGuid":"groceries","value":{"num":-7500,"denom":100},"quantity":{"num":-7500,"denom":100}}
	]}`

func TestUpdateTransaction(t *testing.T) {
	repo := &fakeRepo{}
	rec := sendJSON(newTestServer(repo), http.MethodPatch, "/api/v1/transactions/tx-1", balancedBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if repo.updated == nil {
		t.Fatal("transaction was not updated")
	}
	if repo.updated.GUID != "tx-1" {
		t.Errorf("updated GUID = %q, want the path id tx-1", repo.updated.GUID)
	}
	for _, s := range repo.updated.Splits {
		if s.GUID == "" {
			t.Error("expected regenerated split GUIDs on update")
		}
	}
}

func TestUpdateUnbalancedReturns422(t *testing.T) {
	rec := sendJSON(newTestServer(&fakeRepo{}), http.MethodPatch, "/api/v1/transactions/tx-1", `{
		"currencyGuid":"USD","splits":[
			{"accountGuid":"checking","value":{"num":7500,"denom":100},"quantity":{"num":7500,"denom":100}},
			{"accountGuid":"groceries","value":{"num":-7400,"denom":100},"quantity":{"num":-7400,"denom":100}}
		]}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateNotFoundReturns404(t *testing.T) {
	repo := &fakeRepo{txNotFound: true}
	rec := sendJSON(newTestServer(repo), http.MethodPatch, "/api/v1/transactions/missing", balancedBody)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
	if repo.updated != nil {
		t.Error("must not update an unknown transaction")
	}
}

func TestUpdateForbiddenWithoutMembership(t *testing.T) {
	repo := &fakeRepo{noMembership: true}
	rec := sendJSON(newTestServer(repo), http.MethodPatch, "/api/v1/transactions/tx-1", balancedBody)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
	if repo.updated != nil {
		t.Error("must not update without write access")
	}
}

func TestDeleteTransaction(t *testing.T) {
	repo := &fakeRepo{}
	rec := sendJSON(newTestServer(repo), http.MethodDelete, "/api/v1/transactions/tx-1", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body = %s", rec.Code, rec.Body.String())
	}
	if repo.deletedGUID != "tx-1" {
		t.Errorf("deleted GUID = %q, want tx-1", repo.deletedGUID)
	}
}

func TestDeleteNotFoundReturns404(t *testing.T) {
	repo := &fakeRepo{txNotFound: true}
	rec := sendJSON(newTestServer(repo), http.MethodDelete, "/api/v1/transactions/missing", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
	if repo.deletedGUID != "" {
		t.Error("must not delete an unknown transaction")
	}
}
