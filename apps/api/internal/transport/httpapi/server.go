// Package httpapi exposes the OpenLedger HTTP API over the app services.
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/openledger/openledger/apps/api/internal/app"
)

// Server holds the dependencies the HTTP handlers need.
type Server struct {
	posting   *app.PostingService
	ledger    *app.LedgerService
	structure *app.StructureService
	price     *app.PriceService
	report    *app.ReportService
	provision *app.ProvisionService
	authz     *app.AuthzService
	importer  *app.ImportService
	exporter  *app.ExportService
}

// NewServer builds a Server from its service dependencies.
func NewServer(posting *app.PostingService, ledger *app.LedgerService, structure *app.StructureService, price *app.PriceService, report *app.ReportService, provision *app.ProvisionService, authz *app.AuthzService, importer *app.ImportService, exporter *app.ExportService) *Server {
	return &Server{posting: posting, ledger: ledger, structure: structure, price: price, report: report, provision: provision, authz: authz, importer: importer, exporter: exporter}
}

// Routes returns the configured HTTP handler. /healthz is public; every
// /api/v1/* route requires a verified Authelia session (Remote-User header).
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)

	mux.HandleFunc("GET /api/v1/commodities", s.requireAuth(s.handleListCommodities))
	mux.HandleFunc("POST /api/v1/commodities", s.requireAuth(s.handleCreateCommodity))
	mux.HandleFunc("GET /api/v1/books", s.requireAuth(s.handleListBooks))
	mux.HandleFunc("POST /api/v1/books", s.requireAuth(s.handleCreateBook))
	mux.HandleFunc("GET /api/v1/books/{id}/accounts", s.requireAuth(s.handleListAccounts))
	mux.HandleFunc("GET /api/v1/books/{id}/reports/balance-sheet", s.requireAuth(s.handleBalanceSheet))
	mux.HandleFunc("GET /api/v1/books/{id}/reports/income-statement", s.requireAuth(s.handleIncomeStatement))
	mux.HandleFunc("POST /api/v1/accounts", s.requireAuth(s.handleCreateAccount))
	mux.HandleFunc("POST /api/v1/transactions", s.requireAuth(s.handlePostTransaction))
	mux.HandleFunc("GET /api/v1/transactions/{id}", s.requireAuth(s.handleGetTransaction))
	mux.HandleFunc("PATCH /api/v1/transactions/{id}", s.requireAuth(s.handleUpdateTransaction))
	mux.HandleFunc("DELETE /api/v1/transactions/{id}", s.requireAuth(s.handleDeleteTransaction))
	mux.HandleFunc("GET /api/v1/accounts/{id}/register", s.requireAuth(s.handleAccountRegister))
	mux.HandleFunc("GET /api/v1/prices", s.requireAuth(s.handleListPrices))
	mux.HandleFunc("POST /api/v1/prices", s.requireAuth(s.handleCreatePrice))
	mux.HandleFunc("POST /api/v1/imports/gnucash", s.requireAuth(s.handleImportGnuCash))
	mux.HandleFunc("GET /api/v1/books/{id}/export/gnucash", s.requireAuth(s.handleExportGnuCash))
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
