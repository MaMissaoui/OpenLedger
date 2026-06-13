// Package httpapi exposes the OpenLedger HTTP API over the app services.
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/openledger/openledger/apps/api/internal/app"
)

// Server holds the dependencies the HTTP handlers need.
type Server struct {
	posting *app.PostingService
}

// NewServer builds a Server from its service dependencies.
func NewServer(posting *app.PostingService) *Server {
	return &Server{posting: posting}
}

// Routes returns the configured HTTP handler (stdlib mux with method routing).
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("POST /api/v1/transactions", s.handlePostTransaction)
	return mux
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
