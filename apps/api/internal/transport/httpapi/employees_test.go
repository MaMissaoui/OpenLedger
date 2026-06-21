package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func doReq(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(method, path, strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	return rec
}

func TestEmployeeCRUD(t *testing.T) {
	h := newTestServer(&fakeRepo{})

	// Create.
	rec := doReq(h, http.MethodPost, "/api/v1/books/book-1/employees",
		`{"name":"Ada Lovelace","username":"ada","id":"EMP-0001","currencyGuid":"USD","rate":{"num":7500,"denom":100}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	guid, _ := created["guid"].(string)
	if guid == "" {
		t.Fatal("created employee has no guid")
	}
	if created["active"] != true {
		t.Errorf("new employee should default active; got %v", created["active"])
	}
	rate, _ := created["rate"].(map[string]any)
	if rate["num"].(float64) != 7500 || rate["denom"].(float64) != 100 {
		t.Errorf("rate not preserved exactly: %v", rate)
	}

	// List.
	rec = doReq(h, http.MethodGet, "/api/v1/books/book-1/employees", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var list struct {
		Employees []map[string]any `json:"employees"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Employees) != 1 {
		t.Fatalf("list len = %d, want 1", len(list.Employees))
	}

	// Update (name required; empty -> 400).
	rec = doReq(h, http.MethodPatch, "/api/v1/employees/"+guid, `{"name":"","currencyGuid":"USD"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("blank-name update status = %d, want 400", rec.Code)
	}
	rec = doReq(h, http.MethodPatch, "/api/v1/employees/"+guid, `{"name":"Ada L.","currencyGuid":"USD"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// Delete, then 404 on re-get.
	rec = doReq(h, http.MethodDelete, "/api/v1/employees/"+guid, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	rec = doReq(h, http.MethodGet, "/api/v1/employees/"+guid, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("get after delete = %d, want 404", rec.Code)
	}
}

func TestJobCRUD(t *testing.T) {
	h := newTestServer(&fakeRepo{})

	// Missing owner -> 400.
	rec := doReq(h, http.MethodPost, "/api/v1/books/book-1/jobs", `{"name":"Website rebuild"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("ownerless create = %d, want 400", rec.Code)
	}

	// Valid create.
	rec = doReq(h, http.MethodPost, "/api/v1/books/book-1/jobs",
		`{"name":"Website rebuild","id":"JOB-1","reference":"PO-42","ownerType":"customer","ownerGuid":"cust-1"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	guid, _ := created["guid"].(string)
	if guid == "" {
		t.Fatal("created job has no guid")
	}
	if created["ownerType"] != "customer" || created["ownerGuid"] != "cust-1" {
		t.Errorf("owner not preserved: %v / %v", created["ownerType"], created["ownerGuid"])
	}

	// List.
	rec = doReq(h, http.MethodGet, "/api/v1/books/book-1/jobs", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}
	var list struct {
		Jobs []map[string]any `json:"jobs"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Jobs) != 1 {
		t.Fatalf("list len = %d, want 1", len(list.Jobs))
	}
}
