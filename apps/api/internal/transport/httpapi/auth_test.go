package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestProtectedRouteRejectsMissingRemoteUser verifies that /api/v1 routes
// return 401 when the Authelia-set Remote-User header is absent. In production
// Traefik ensures every request reaching the API carries verified headers; in
// tests we confirm the middleware rejects requests that bypass the proxy.
func TestProtectedRouteRejectsMissingRemoteUser(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/x/register", nil)
	registerServer(&ledgerFake{exists: true}).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 without Remote-User header", rec.Code)
	}
}

// TestProtectedRouteAcceptsRemoteUser verifies that a request bearing
// Remote-User (as Traefik sets after Authelia verification) passes requireAuth.
func TestProtectedRouteAcceptsRemoteUser(t *testing.T) {
	repo := &ledgerFake{exists: true}
	rec := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodGet, "/api/v1/accounts/x/register", nil))
	registerServer(repo).ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("status = 401, want auth to pass when Remote-User is set")
	}
}

// TestHealthzIsPublic confirms /healthz is reachable without authentication.
func TestHealthzIsPublic(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	authedServer(Services{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
